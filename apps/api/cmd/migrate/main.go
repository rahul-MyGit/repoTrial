package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	directory := flag.String("dir", "db/migrations", "migration directory")
	flag.Parse()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := migrate(ctx, databaseURL, *directory, logger); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

func migrate(ctx context.Context, databaseURL, directory string, logger *slog.Logger) error {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(hashtext('repotrial_migrations'))"); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext('repotrial_migrations'))")

	_, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY,
		checksum bytea NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	paths, err := filepath.Glob(filepath.Join(directory, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	if len(paths) == 0 {
		return errors.New("no migration files found")
	}
	sort.Strings(paths)
	for _, path := range paths {
		version := filepath.Base(path)
		if !strings.HasSuffix(version, ".up.sql") {
			continue
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", version, err)
		}
		checksum := sha256.Sum256(contents)
		var prior []byte
		err = conn.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE version = $1", version).Scan(&prior)
		if err == nil {
			if !bytes.Equal(prior, checksum[:]) {
				return fmt.Errorf("migration %s changed after application", version)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check %s: %w", version, err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin %s: %w", version, err)
		}
		_, execErr := tx.Exec(ctx, string(contents), pgx.QueryExecModeSimpleProtocol)
		if execErr != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", version, execErr)
		}
		_, execErr = tx.Exec(ctx, "INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)", version, checksum[:])
		if execErr != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", version, execErr)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", version, err)
		}
		logger.Info("migration applied", "version", version)
	}
	return nil
}
