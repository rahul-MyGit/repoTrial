package runs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
