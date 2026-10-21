package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/runs"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	dsn, artifactRoot, image := os.Getenv("DATABASE_URL"), os.Getenv("ARTIFACT_ROOT"), os.Getenv("RUNNER_IMAGE")
	if dsn == "" || artifactRoot == "" || image == "" {
		log.Fatal("DATABASE_URL, ARTIFACT_ROOT and RUNNER_IMAGE are required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatal(err)
	}
	id := os.Getenv("RUNNER_ID")
	if id == "" {
		id, err = os.Hostname()
		if err != nil {
			log.Fatal(err)
		}
	}
	maxRuns := 2
	if value := os.Getenv("RUNNER_MAX_DISPATCHES"); value != "" {
		maxRuns, err = strconv.Atoi(value)
		if err != nil || maxRuns < 1 {
			log.Fatal("invalid RUNNER_MAX_DISPATCHES")
		}
	}
	w := runs.Worker{Queue: runs.NewPostgres(pool), Executor: runs.Docker{ArtifactRoot: artifactRoot, Image: image},
		ID: id, Lease: 4 * time.Minute, MaxDispatches: maxRuns}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		worked, err := w.Once(ctx)
		if err != nil {
			log.Printf("runner cycle: %v", err)
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
