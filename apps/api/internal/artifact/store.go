package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	slugPattern   = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// DigestDirectory hashes every regular file by relative name and content.
// It rejects links and special files so the digest describes the exact tree.
func DigestDirectory(root string) (string, error) {
	var paths []string
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
		rel, err := filepath.Rel(root, fullPath)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk artifact: %w", err)
	}
	if len(paths) == 0 {
		return "", errors.New("artifact is empty")
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, name := range paths {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return "", fmt.Errorf("read artifact %s: %w", name, err)
		}
		_, _ = hash.Write([]byte(name + "\x00"))
		_, _ = hash.Write(content)
		_, _ = hash.Write([]byte("\x00"))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// PublishDirectory copies a source artifact to a version and digest named path.
// A changed source always produces a new URI, never mutating an existing version.
func PublishDirectory(sourceRoot, publishedRoot, slug string, version int) (string, string, error) {
	if !slugPattern.MatchString(slug) || version < 1 {
		return "", "", errors.New("invalid artifact identity")
	}
	digest, err := DigestDirectory(sourceRoot)
	if err != nil {
		return "", "", err
	}
	uri := fmt.Sprintf("%s/v%d-%s", slug, version, digest)
	parent := filepath.Join(publishedRoot, slug)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", "", fmt.Errorf("create artifact parent: %w", err)
	}
	if err := rejectSymlinkPath(publishedRoot, parent); err != nil {
		return "", "", err
	}
	target := filepath.Join(publishedRoot, filepath.FromSlash(uri))
	if _, err := os.Lstat(target); err == nil {
		return verifyExisting(publishedRoot, uri, digest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	temporary, err := os.MkdirTemp(parent, ".publishing-")
	if err != nil {
		return "", "", fmt.Errorf("create artifact staging directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	if err := copyTree(sourceRoot, temporary); err != nil {
		return "", "", err
	}
	stagedDigest, err := DigestDirectory(temporary)
	if err != nil {
		return "", "", err
	}
	if stagedDigest != digest {
		return "", "", errors.New("source artifact changed during publication")
	}
	if err := os.Rename(temporary, target); err != nil {
		if _, statErr := os.Lstat(target); statErr == nil {
			return verifyExisting(publishedRoot, uri, digest)
		}
		return "", "", fmt.Errorf("publish artifact: %w", err)
	}
	return uri, digest, nil
}

func verifyExisting(root, uri, digest string) (string, string, error) {
	if err := Verify(root, uri, digest); err != nil {
		return "", "", err
	}
	return uri, digest, nil
}

// Verify catches accidental or malicious edits to a published artifact.
func Verify(root, uri, expectedDigest string) error {
	if !digestPattern.MatchString(expectedDigest) || uri == "" || strings.ContainsAny(uri, "\\\x00") ||
		strings.HasPrefix(uri, "/") || path.Clean(uri) != uri || strings.HasPrefix(uri, "../") {
		return errors.New("invalid artifact reference")
	}
	target := filepath.Join(root, filepath.FromSlash(uri))
	if err := rejectSymlinkPath(root, target); err != nil {
		return err
	}
	actual, err := DigestDirectory(target)
	if err != nil {
		return err
	}
	if actual != expectedDigest {
		return fmt.Errorf("artifact digest mismatch for %s", uri)
	}
	return nil
}

func rejectSymlinkPath(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("artifact path escapes root")
	}
	current := root
	if info, err := os.Lstat(current); err != nil || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("artifact root is missing or a symlink")
	}
	for _, segment := range strings.Split(rel, string(filepath.Separator)) {
		if segment == "." {
			continue
		}
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("artifact path is missing or contains a symlink")
		}
	}
	return nil
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(fullPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, fullPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact contains symlink: %s", fullPath)
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.Mkdir(target, 0755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("artifact contains non-regular file: %s", fullPath)
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0644)
	})
}
