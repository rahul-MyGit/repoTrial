package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Address            string
	DatabaseURL        string
	DBMaxConns         int32
	AppURL             string
	GoogleClientID     string
	GoogleClientSecret string
	OAuthStateSecret   string
	OAuthRedirectURL   string
	ArtifactRoot       string
	CookieSecure       bool
}

func Load() (Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}

	if _, err := net.LookupPort("tcp", port); err != nil {
		return Config{}, fmt.Errorf("invalid PORT: %w", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	maxConns := int32(10)
	if configured := os.Getenv("DB_MAX_CONNS"); configured != "" {
		parsed, err := strconv.ParseInt(configured, 10, 32)
		if err != nil || parsed < 1 || parsed > 100 {
			return Config{}, errors.New("DB_MAX_CONNS must be between 1 and 100")
		}
		maxConns = int32(parsed)
	}
	appURL := os.Getenv("APP_URL")
	parsedAppURL, err := url.Parse(appURL)
	if err != nil || parsedAppURL.Host == "" || (parsedAppURL.Scheme != "http" && parsedAppURL.Scheme != "https") || parsedAppURL.Path != "" {
		return Config{}, errors.New("APP_URL must be an HTTP(S) origin")
	}
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	stateSecret := os.Getenv("OAUTH_STATE_SECRET")
	if clientID == "" || clientSecret == "" || len(stateSecret) < 32 {
		return Config{}, errors.New("Google OAuth credentials and an OAUTH_STATE_SECRET of at least 32 bytes are required")
	}
	artifactRoot := os.Getenv("ARTIFACT_ROOT")
	if strings.TrimSpace(artifactRoot) == "" {
		return Config{}, errors.New("ARTIFACT_ROOT is required")
	}
	redirectURL := strings.TrimRight(appURL, "/") + "/api/v1/auth/google/callback"

	return Config{
		Address:            net.JoinHostPort("", port),
		DatabaseURL:        databaseURL,
		DBMaxConns:         maxConns,
		AppURL:             appURL,
		GoogleClientID:     clientID,
		GoogleClientSecret: clientSecret,
		OAuthStateSecret:   stateSecret,
		OAuthRedirectURL:   redirectURL,
		ArtifactRoot:       artifactRoot,
		CookieSecure:       parsedAppURL.Scheme == "https",
	}, nil
}
