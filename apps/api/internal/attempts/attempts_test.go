package attempts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeRepository struct{ gotKey string }

func (f *fakeRepository) Create(_ context.Context, _, _, key string) (Attempt, error) {
	f.gotKey = key
	return Attempt{ID: "attempt", State: Created}, nil
}

func (f *fakeRepository) Get(_ context.Context, _, _ string) (Attempt, error) {
	return Attempt{ID: "attempt", State: Investigating}, nil
}

func TestCreateValidatesIdempotencyKey(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)
	if _, err := service.Create(context.Background(), "user", "problem", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("empty idempotency key: got %v", err)
	}
	if _, err := service.Create(context.Background(), "user", "problem", "retry-1"); err != nil {
		t.Fatal(err)
	}
	if repo.gotKey != "retry-1" {
		t.Fatalf("key = %q", repo.gotKey)
	}
}

type fakeSnapshot struct{ snapshot Snapshot }

func (f fakeSnapshot) Snapshot(_ context.Context, _, _ string, _ *int) (Snapshot, error) {
	return f.snapshot, nil
}

func TestFileServiceAppliesRevisionOverlayAndRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "challenge", "source")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "main.go"), []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	service := NewFileService(fakeSnapshot{Snapshot{
		ArtifactURI: "challenge", SourceRoot: "source", Revision: 1,
		Overlay: map[string]string{"main.go": "after"},
	}}, root)

	file, err := service.Read(context.Background(), "user", "attempt", "main.go", nil)
	if err != nil {
		t.Fatal(err)
	}
	if file.Content != "after" || file.Revision != 1 {
		t.Fatalf("unexpected file: %+v", file)
	}
	if _, err := service.Read(context.Background(), "user", "attempt", "../secret", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("traversal: got %v", err)
	}
	if _, err := service.Read(context.Background(), "user", "attempt", "missing.go", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing file: got %v", err)
	}
}

func TestFileServiceRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "challenge", "source")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(source, "link")); err != nil {
		t.Fatal(err)
	}
	service := NewFileService(fakeSnapshot{Snapshot{
		ArtifactURI: "challenge", SourceRoot: "source", Overlay: map[string]string{},
	}}, root)
	if _, err := service.List(context.Background(), "user", "attempt", nil); err == nil {
		t.Fatal("symlink was exposed")
	}
}
