package config

import (
	"errors"
	"fmt"
	"net"
	"os"
)

type Config struct {
	Address     string
	DatabaseURL string
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

	return Config{
		Address:     net.JoinHostPort("", port),
		DatabaseURL: databaseURL,
	}, nil
}
