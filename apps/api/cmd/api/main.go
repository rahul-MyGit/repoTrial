package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rahul-MyGit/repoTrial/apps/api/internal/attempts"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/auth"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/catalog"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/config"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/db"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/diagnosis"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/httpapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	pool, err := db.Open(context.Background(), cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	authHandler, err := auth.NewHandler(auth.NewPostgresRepository(pool), auth.Config{
		ClientID: cfg.GoogleClientID, ClientSecret: cfg.GoogleClientSecret,
		RedirectURL: cfg.OAuthRedirectURL, AppURL: cfg.AppURL,
		StateSecret: cfg.OAuthStateSecret, CookieSecure: cfg.CookieSecure,
	})
	if err != nil {
		logger.Error("OAuth configuration failed", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr: cfg.Address,
		Handler: httpapi.NewApplicationHandler(httpapi.Services{
			Auth:      authHandler,
			Catalog:   catalog.NewHandler(catalog.NewPostgresRepository(pool)),
			Attempts:  attempts.NewService(attempts.NewPostgres(pool)),
			Files:     attempts.NewFileService(attempts.NewSnapshotPostgres(pool), cfg.ArtifactRoot),
			Diagnoses: diagnosis.NewService(diagnosis.NewPostgres(pool)),
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown failed", "error", err)
		}
	}()

	logger.Info("api listening", "address", cfg.Address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
