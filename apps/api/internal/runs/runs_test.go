package runs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rahul-MyGit/repoTrial/apps/api/internal/artifact"
)

func TestBaselineMarker(t *testing.T) {
	if !containsExpected("FAIL TestCheckoutRetryReturnsOriginalOrder", "", "TestCheckoutRetryReturnsOriginalOrder") {
		t.Fatal("intended failure should match")
	}
	if containsExpected("FAIL OtherTest", "", "TestCheckoutRetryReturnsOriginalOrder") || containsExpected("anything", "", "") {
		t.Fatal("unexpected baseline result should stay locked")
	}
}

func TestSnapshotRejectsSymlinkAndUnknownOverlay(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "source.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copySnapshot(source, t.TempDir(), map[string]string{"missing.go": "injected"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown overlay: %v", err)
	}
	if err := os.Symlink(filepath.Join(source, "source.go"), filepath.Join(source, "link.go")); err != nil {
		t.Fatal(err)
	}
	if err := copySnapshot(source, t.TempDir(), nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("symlink: %v", err)
	}
}

func TestSnapshotAppliesExactOverlay(t *testing.T) {
	source, dest := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "source.go"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copySnapshot(source, dest, map[string]string{"source.go": "new"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "source.go"))
	if err != nil || string(got) != "new" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestBoundedWriter(t *testing.T) {
	w := &boundedWriter{limit: 4}
	if n, err := w.Write([]byte("abcdefgh")); n != 8 || err != nil {
		t.Fatalf("write %d, %v", n, err)
	}
	if !strings.HasPrefix(w.String(), "abcd") || !strings.Contains(w.String(), "truncated") {
		t.Fatal(w.String())
	}
}

func TestContainerNameFencesDispatchAttempt(t *testing.T) {
	s := Snapshot{RunID: "01234567-89ab-cdef-0123-456789abcdef", DispatchAttempt: 1}
	first, err := containerNameFor(s)
	if err != nil {
		t.Fatal(err)
	}
	s.DispatchAttempt = 2
	second, err := containerNameFor(s)
	if err != nil || first == second {
		t.Fatalf("first=%q second=%q err=%v", first, second, err)
	}
	s.DispatchAttempt = 0
	if _, err := containerNameFor(s); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid dispatch: %v", err)
	}
}

func TestDockerRejectsChangedPublishedArtifactBeforeExecution(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "challenge", "source")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "main.go")
	if err := os.WriteFile(file, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	digest, err := artifact.DigestDirectory(filepath.Join(root, "challenge"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("package changed"), 0644); err != nil {
		t.Fatal(err)
	}
	s := Snapshot{RunID: "01234567-89ab-cdef-0123-456789abcdef", DispatchAttempt: 1,
		ArtifactURI: "challenge", ArtifactSHA256: digest, Kind: Baseline,
		Manifest: Manifest{SourceRoot: "source", BaselineCommand: []string{"go", "test", "./..."},
			Limits: Limits{CPUMillis: 1000, MemoryMiB: 512, TimeoutSeconds: 60, OutputKiB: 128}}}
	_, err = (Docker{ArtifactRoot: root, Image: "fixture", Binary: "missing-docker"}).Execute(context.Background(), s)
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
}

type fakeQueue struct {
	s        Snapshot
	finished Result
}

func (q *fakeQueue) Claim(context.Context, string, time.Duration) (Snapshot, bool, error) {
	return q.s, true, nil
}
func (q *fakeQueue) Finish(_ context.Context, _ Snapshot, _ string, r Result) error {
	q.finished = r
	return nil
}
func (q *fakeQueue) RequeueExpired(context.Context, int) error { return nil }

type failingExecutor struct{}

func (failingExecutor) Execute(context.Context, Snapshot) (Result, error) {
	return Result{}, errors.New("runtime unavailable")
}

func TestWorkerStoresInfrastructureFailure(t *testing.T) {
	q := &fakeQueue{}
	w := Worker{Queue: q, Executor: failingExecutor{}, ID: "worker", Lease: time.Minute, MaxDispatches: 2}
	ok, err := w.Once(context.Background())
	if !ok || err != nil || q.finished.Status != InfraFailed {
		t.Fatalf("ok=%v err=%v result=%+v", ok, err, q.finished)
	}
}
