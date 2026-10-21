package runs

import (
	"context"
	"strings"
	"time"
)

type Queue interface {
	Claim(context.Context, string, time.Duration) (Snapshot, bool, error)
	Finish(context.Context, Snapshot, string, Result) error
	RequeueExpired(context.Context, int) error
}

type Worker struct {
	Queue         Queue
	Executor      Executor
	ID            string
	Lease         time.Duration
	MaxDispatches int
}

func containsExpected(stdout, stderr, marker string) bool {
	return marker != "" && (strings.Contains(stdout, marker) || strings.Contains(stderr, marker))
}

// Once claims and executes one immutable revision. A worker lease must exceed
// the challenge's execution timeout plus the executor's startup allowance.
func (w Worker) Once(ctx context.Context) (bool, error) {
	if w.Queue == nil || w.Executor == nil || w.ID == "" || w.Lease <= 0 || w.MaxDispatches < 1 {
		return false, ErrInvalid
	}
	if err := w.Queue.RequeueExpired(ctx, w.MaxDispatches); err != nil {
		return false, err
	}
	s, ok, err := w.Queue.Claim(ctx, w.ID, w.Lease)
	if err != nil || !ok {
		return false, err
	}
	result, execErr := w.Executor.Execute(ctx, s)
	if execErr != nil {
		result = Result{Status: InfraFailed, Stderr: execErr.Error()}
	}
	if result.Status == "" {
		result.Status = InfraFailed
	}
	return true, w.Queue.Finish(ctx, s, w.ID, result)
}
