package runs

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("run not found")
	ErrConflict = errors.New("run conflicts with attempt state or active run")
	ErrInvalid  = errors.New("invalid run request")
)

type Kind string

const (
	Baseline Kind = "baseline"
	Visible  Kind = "visible"
)

type Status string

const (
	Queued      Status = "queued"
	Running     Status = "running"
	Passed      Status = "passed"
	Failed      Status = "failed"
	InfraFailed Status = "infra_failed"
	TimedOut    Status = "timed_out"
)

type Run struct {
	ID         string     `json:"id"`
	AttemptID  string     `json:"attemptId"`
	Revision   int        `json:"revision"`
	Kind       Kind       `json:"kind"`
	Status     Status     `json:"status"`
	Stdout     *string    `json:"stdout"`
	Stderr     *string    `json:"stderr"`
	ExitCode   *int       `json:"exitCode"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

type Request struct {
	UserID, AttemptID, IdempotencyKey string
	Revision                          int
	Kind                              Kind
}

type Limits struct {
	CPUMillis      int `json:"cpuMillis"`
	MemoryMiB      int `json:"memoryMiB"`
	TimeoutSeconds int `json:"timeoutSeconds"`
	OutputKiB      int `json:"outputKiB"`
}

type Expected struct {
	ExitCode       int    `json:"exitCode"`
	OutputContains string `json:"outputContains"`
}

type Manifest struct {
	SourceRoot         string   `json:"sourceRoot"`
	AllowedPatchPaths  []string `json:"allowedPatchPaths"`
	BaselineCommand    []string `json:"baselineCommand"`
	VisibleTestCommand []string `json:"visibleTestCommand"`
	BaselineExpected   Expected `json:"baselineExpected"`
	Limits             Limits   `json:"limits"`
}

type Snapshot struct {
	RunID, AttemptID, ArtifactURI string
	Revision                      int
	DispatchAttempt               int
	Kind                          Kind
	Overlay                       map[string]string
	Manifest                      Manifest
}

type Result struct {
	Status   Status
	Stdout   string
	Stderr   string
	ExitCode *int
}

type Executor interface {
	Execute(context.Context, Snapshot) (Result, error)
}
