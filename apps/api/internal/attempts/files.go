package attempts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxSourceFileBytes = 1024 * 1024

type FileEntry struct {
	Path      string `json:"path"`
	SizeBytes int    `json:"sizeBytes"`
}

type FileTree struct {
	Revision int         `json:"revision"`
	Items    []FileEntry `json:"items"`
}

type File struct {
	Path     string `json:"path"`
	Revision int    `json:"revision"`
	SHA256   string `json:"sha256"`
	Content  string `json:"content"`
}

type Snapshot struct {
	ArtifactURI string
	SourceRoot  string
	Revision    int
	Overlay     map[string]string
}

type SnapshotRepository interface {
	Snapshot(context.Context, string, string, *int) (Snapshot, error)
}

type FileService struct {
	repo         SnapshotRepository
	artifactRoot string
}

func NewFileService(repo SnapshotRepository, artifactRoot string) *FileService {
	return &FileService{repo: repo, artifactRoot: artifactRoot}
}

func (s *FileService) List(ctx context.Context, userID, attemptID string, revision *int) (FileTree, error) {
	snapshot, err := s.repo.Snapshot(ctx, userID, attemptID, revision)
	if err != nil {
		return FileTree{}, err
	}
	files, err := s.load(snapshot)
	if err != nil {
		return FileTree{}, err
	}
	items := make([]FileEntry, 0, len(files))
	for name, content := range files {
		items = append(items, FileEntry{Path: name, SizeBytes: len(content)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return FileTree{Revision: snapshot.Revision, Items: items}, nil
}

func (s *FileService) Read(ctx context.Context, userID, attemptID, name string, revision *int) (File, error) {
	if !validRelativePath(name) {
		return File{}, ErrNotFound
	}
	snapshot, err := s.repo.Snapshot(ctx, userID, attemptID, revision)
	if err != nil {
		return File{}, err
	}
	files, err := s.load(snapshot)
	if err != nil {
		return File{}, err
	}
	content, ok := files[name]
	if !ok {
		return File{}, ErrNotFound
	}
	hash := sha256.Sum256([]byte(content))
	return File{Path: name, Revision: snapshot.Revision, SHA256: hex.EncodeToString(hash[:]), Content: content}, nil
}

func (s *FileService) load(snapshot Snapshot) (map[string]string, error) {
	if !validRelativePath(snapshot.ArtifactURI) || !validRelativePath(snapshot.SourceRoot) {
		return nil, fmt.Errorf("unsafe artifact path")
	}
	root := filepath.Join(s.artifactRoot, filepath.FromSlash(snapshot.ArtifactURI), filepath.FromSlash(snapshot.SourceRoot))
	if err := rejectSymlinkComponents(s.artifactRoot, snapshot.ArtifactURI, snapshot.SourceRoot); err != nil {
		return nil, err
	}
	files := map[string]string{}
	err := filepath.WalkDir(root, func(fullPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact contains symlink: %s", fullPath)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("artifact contains non-regular file: %s", fullPath)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxSourceFileBytes {
			return fmt.Errorf("artifact file too large: %s", fullPath)
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, fullPath)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(content)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load source snapshot: %w", err)
	}
	for name, content := range snapshot.Overlay {
		if !validRelativePath(name) || len(content) > maxSourceFileBytes {
			return nil, fmt.Errorf("unsafe overlay entry")
		}
		if _, ok := files[name]; !ok {
			return nil, fmt.Errorf("overlay refers to unknown file: %q", name)
		}
		files[name] = content
	}
	return files, nil
}

func validRelativePath(name string) bool {
	return name != "" && name != "." && !strings.ContainsAny(name, "\\\x00") &&
		!strings.HasPrefix(name, "/") && !strings.HasPrefix(name, "../") &&
		name != ".." && path.Clean(name) == name && !strings.Contains(name, "//")
}

func rejectSymlinkComponents(root string, segments ...string) error {
	for _, segment := range segments {
		for _, part := range strings.Split(segment, "/") {
			root = filepath.Join(root, part)
			info, err := os.Lstat(root)
			if err != nil {
				return fmt.Errorf("inspect artifact path: %w", err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("artifact path contains symlink")
			}
		}
	}
	return nil
}

type SnapshotPostgres struct{ db *pgxpool.Pool }

func NewSnapshotPostgres(db *pgxpool.Pool) *SnapshotPostgres { return &SnapshotPostgres{db: db} }

func (p *SnapshotPostgres) Snapshot(
	ctx context.Context, userID, attemptID string, requested *int,
) (Snapshot, error) {
	var snapshot Snapshot
	var state State
	var overlay []byte
	err := p.db.QueryRow(ctx, `SELECT a.state, v.artifact_uri,
		v.manifest->>'sourceRoot', r.revision, r.file_overlay
		FROM attempts a
		JOIN challenge_versions v ON v.id = a.challenge_version_id
		JOIN attempt_revisions r ON r.attempt_id = a.id
			AND r.revision = COALESCE($3::integer, a.current_revision)
		WHERE a.id = $1 AND a.user_id = $2`, attemptID, userID, requested).
		Scan(&state, &snapshot.ArtifactURI, &snapshot.SourceRoot, &snapshot.Revision, &overlay)
	if errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, ErrNotFound
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load attempt snapshot: %w", err)
	}
	if !CanReadFiles(state) {
		return Snapshot{}, ErrLocked
	}
	if err := json.Unmarshal(overlay, &snapshot.Overlay); err != nil {
		return Snapshot{}, fmt.Errorf("decode attempt overlay: %w", err)
	}
	return snapshot, nil
}
