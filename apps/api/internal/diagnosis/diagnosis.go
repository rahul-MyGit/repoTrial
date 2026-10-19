package diagnosis

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("attempt not found")
	ErrConflict = errors.New("diagnosis is not allowed in this state")
	ErrInvalid  = errors.New("invalid diagnosis")
)

type Input struct {
	SuspectedCause  string `json:"suspectedCause"`
	ErrorEvidence   string `json:"errorEvidence"`
	FirstInspection string `json:"firstInspection"`
	Confidence      *int   `json:"confidence,omitempty"`
}

type Diagnosis struct {
	ID              string    `json:"id"`
	AttemptID       string    `json:"attemptId"`
	Sequence        int       `json:"sequence"`
	SuspectedCause  string    `json:"suspectedCause"`
	ErrorEvidence   string    `json:"errorEvidence"`
	FirstInspection string    `json:"firstInspection"`
	Confidence      *int      `json:"confidence,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

type Repository interface {
	Record(context.Context, string, string, Input) (Diagnosis, error)
	List(context.Context, string, string) ([]Diagnosis, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Record(ctx context.Context, userID, attemptID string, in Input) (Diagnosis, error) {
	if userID == "" || attemptID == "" || !valid(in) {
		return Diagnosis{}, ErrInvalid
	}
	return s.repo.Record(ctx, userID, attemptID, in)
}

func (s *Service) List(ctx context.Context, userID, attemptID string) ([]Diagnosis, error) {
	return s.repo.List(ctx, userID, attemptID)
}

func valid(in Input) bool {
	for _, field := range []string{in.SuspectedCause, in.ErrorEvidence, in.FirstInspection} {
		if strings.TrimSpace(field) == "" || len(field) > 4000 {
			return false
		}
	}
	return in.Confidence == nil || (*in.Confidence >= 0 && *in.Confidence <= 100)
}
