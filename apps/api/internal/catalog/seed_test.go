package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactDigestChangesWithContentsAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "source"), 0700); err != nil {
		t.Fatal(err)
	}
	modPath := filepath.Join(root, "source", "go.mod")
	if err := os.WriteFile(modPath, []byte("module example.com/challenge\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := artifactDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := artifactDigest(root)
	if err != nil || first != second {
		t.Fatalf("digest changed without source changes: %q, %q, %v", first, second, err)
	}
	if err := os.WriteFile(modPath, []byte("module example.com/changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := artifactDigest(root)
	if err != nil || changed == first {
		t.Fatalf("source change did not alter digest: %q, %q, %v", first, changed, err)
	}
	if err := os.Symlink(modPath, filepath.Join(root, "linked.mod")); err != nil {
		t.Fatal(err)
	}
	if _, err := artifactDigest(root); err == nil {
		t.Fatal("digest accepted a symlink")
	}
}
