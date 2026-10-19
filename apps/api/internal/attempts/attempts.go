package attempts

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("attempt not found")
	ErrConflict = errors.New("attempt state conflict")
	ErrLocked   = errors.New("repository locked until initial diagnosis")
)

type State string

const (
	Created        State = "created"
	BaselineQueued State = "baseline_queued"
	BaselineReady  State = "baseline_ready"
	Investigating  State = "investigating"
	Submitted      State = "submitted"
)

type Attempt struct {
	ID               string     `json:"id"`
	ProblemSlug      string     `json:"problemSlug"`
	ChallengeVersion int        `json:"challengeVersion"`
	State            State      `json:"state"`
	Revision         int        `json:"revision"`
	CreatedAt        time.Time  `json:"createdAt"`
	SubmittedAt      *time.Time `json:"submittedAt,omitempty"`
}

type Repository interface {
	Create(context.Context, string, string, string) (Attempt, error)
	Get(context.Context, string, string) (Attempt, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Create(ctx context.Context, userID, slug, key string) (Attempt, error) {
	if userID == "" || slug == "" || len(key) == 0 || len(key) > 128 {
		return Attempt{}, ErrConflict
	}
	return s.repo.Create(ctx, userID, slug, key)
}

func (s *Service) Get(ctx context.Context, userID, attemptID string) (Attempt, error) {
	return s.repo.Get(ctx, userID, attemptID)
}

func CanReadFiles(state State) bool {
	return state == Investigating || state == Submitted
}
