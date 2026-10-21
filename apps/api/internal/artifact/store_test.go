package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishPinsContentAndDetectsTampering(t *testing.T) {
	source := t.TempDir()
	published := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "source"), 0700); err != nil {
		t.Fatal(err)
	}
	modPath := filepath.Join(source, "source", "go.mod")
	if err := os.WriteFile(modPath, []byte("module example.com/challenge\n"), 0600); err != nil {
		t.Fatal(err)
	}
	uri, digest, err := PublishDirectory(source, published, "checkout-idempotency", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := PublishDirectory(source, published, "checkout-idempotency", 1); err != nil {
		t.Fatalf("repeat publication failed: %v", err)
	}
	if err := Verify(published, uri, digest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modPath, []byte("module example.com/changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(published, uri, digest); err != nil {
		t.Fatalf("source edit changed published copy: %v", err)
	}
	publishedMod := filepath.Join(published, filepath.FromSlash(uri), "source", "go.mod")
	if err := os.WriteFile(publishedMod, []byte("tampered\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(published, uri, digest); err == nil {
		t.Fatal("tampered published copy passed verification")
	}
}

func TestPublishRejectsSymlinks(t *testing.T) {
	source := t.TempDir()
	if err := os.Symlink("missing", filepath.Join(source, "link")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := PublishDirectory(source, t.TempDir(), "example", 1); err == nil {
		t.Fatal("symlink artifact was accepted")
	}
}
