package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/artifact"
)

type SeedManifest struct {
	SchemaVersion int      `json:"schemaVersion"`
	Slug          string   `json:"slug"`
	Version       int      `json:"version"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	Language      string   `json:"language"`
	Difficulty    string   `json:"difficulty"`
	Tags          []string `json:"tags"`
	SourceRoot    string   `json:"sourceRoot"`
}

// SeedChallenge publishes an immutable challenge version from a local artifact.
// Repeating the same seed is safe; changing a published artifact requires a new version.
func SeedChallenge(ctx context.Context, pool *pgxpool.Pool, sourceRoot, artifactRoot, slug string) error {
	if !slugPattern.MatchString(slug) {
		return errors.New("invalid challenge slug")
	}
	root := filepath.Join(sourceRoot, slug)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read challenge manifest: %w", err)
	}
	var manifest SeedManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("decode challenge manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 || manifest.Slug != slug || manifest.Version < 1 ||
		manifest.Title == "" || manifest.Summary == "" || manifest.Language != "go" ||
		!validDifficulty(manifest.Difficulty) || manifest.SourceRoot != "source" {
		return errors.New("invalid public challenge metadata")
	}
	if _, err := os.Stat(filepath.Join(root, manifest.SourceRoot, "go.mod")); err != nil {
		return fmt.Errorf("challenge source is incomplete: %w", err)
	}
	artifactURI, digest, err := artifact.PublishDirectory(root, artifactRoot, slug, manifest.Version)
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin challenge seed: %w", err)
	}
	defer tx.Rollback(ctx)
	var problemID string
	err = tx.QueryRow(ctx, `INSERT INTO problems
		(slug, title, summary, difficulty, language, tags, is_published)
		VALUES ($1, $2, $3, $4, $5, $6, true)
		ON CONFLICT (slug) DO UPDATE SET is_published = true
		RETURNING id`, manifest.Slug, manifest.Title, manifest.Summary,
		manifest.Difficulty, manifest.Language, manifest.Tags).Scan(&problemID)
	if err != nil {
		return fmt.Errorf("upsert challenge problem: %w", err)
	}
	var existingDigest string
	var publishedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT artifact_sha256, published_at FROM challenge_versions
		WHERE problem_id = $1 AND version = $2`, problemID, manifest.Version).Scan(&existingDigest, &publishedAt)
	if err == nil {
		if existingDigest != digest {
			return fmt.Errorf("published challenge %s v%d differs from local artifact", slug, manifest.Version)
		}
		if publishedAt == nil {
			if _, err := tx.Exec(ctx, `UPDATE challenge_versions SET published_at = now()
				WHERE problem_id = $1 AND version = $2`, problemID, manifest.Version); err != nil {
				return fmt.Errorf("publish existing challenge version: %w", err)
			}
		}
		return tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check challenge version: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO challenge_versions
		(problem_id, version, manifest_schema_version, manifest, artifact_uri,
		 artifact_sha256, evaluation_ref, published_at)
		VALUES ($1, $2, 1, $3, $4, $5, $6, now())`,
		problemID, manifest.Version, string(manifestBytes), artifactURI, digest,
		fmt.Sprintf("%s/v%d", slug, manifest.Version))
	if err != nil {
		return fmt.Errorf("publish challenge version: %w", err)
	}
	return tx.Commit(ctx)
}

func validDifficulty(value string) bool {
	return value == "beginner" || value == "intermediate" || value == "advanced"
}
