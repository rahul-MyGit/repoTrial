package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
)

type Config struct {
	Address     string
	DatabaseURL string
	DBMaxConns  int32
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

	return Config{
		Address:     net.JoinHostPort("", port),
		DatabaseURL: databaseURL,
		DBMaxConns:  maxConns,
	}, nil
}
