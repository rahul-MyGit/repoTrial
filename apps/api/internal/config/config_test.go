package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example/test")
	t.Setenv("PORT", "4080")
	t.Setenv("DB_MAX_CONNS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Address != ":4080" {
		t.Fatalf("Address = %q, want :4080", cfg.Address)
	}
	if cfg.DBMaxConns != 10 {
		t.Fatalf("DBMaxConns = %d, want 10", cfg.DBMaxConns)
	}
}

func TestLoadRejectsInvalidPoolLimit(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example/test")
	t.Setenv("DB_MAX_CONNS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted invalid DB_MAX_CONNS")
	}
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted missing DATABASE_URL")
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example/test")
	t.Setenv("PORT", "not-a-port")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted invalid PORT")
	}
}
