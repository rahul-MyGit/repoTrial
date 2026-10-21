package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/rahul-MyGit/repoTrial/apps/api/internal/catalog"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	databaseURL := os.Getenv("DATABASE_URL")
	sourceRoot := os.Getenv("CHALLENGE_SOURCE_ROOT")
	artifactRoot := os.Getenv("ARTIFACT_ROOT")
	if databaseURL == "" || sourceRoot == "" || artifactRoot == "" {
		logger.Error("DATABASE_URL, CHALLENGE_SOURCE_ROOT, and ARTIFACT_ROOT are required")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, databaseURL, 2)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := catalog.SeedChallenge(ctx, pool, sourceRoot, artifactRoot, "checkout-idempotency"); err != nil {
		logger.Error("challenge seed failed", "error", err)
		os.Exit(1)
	}
	logger.Info("challenge published", "slug", "checkout-idempotency", "version", 1)
}
