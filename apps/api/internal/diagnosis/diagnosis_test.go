package diagnosis

import (
	"context"
	"errors"
	"testing"
)

type fakeRepository struct{ calls int }

func (f *fakeRepository) Record(_ context.Context, _, _ string, _ Input) (Diagnosis, error) {
	f.calls++
	return Diagnosis{Sequence: 1}, nil
}

func (f *fakeRepository) List(_ context.Context, _, _ string) ([]Diagnosis, error) {
	return []Diagnosis{}, nil
}

func TestRecordValidatesBeforePersistence(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)
	in := Input{SuspectedCause: "retry handling", ErrorEvidence: "duplicate key", FirstInspection: "repository"}
	if _, err := service.Record(context.Background(), "user", "attempt", in); err != nil {
		t.Fatal(err)
	}
	in.ErrorEvidence = "  "
	if _, err := service.Record(context.Background(), "user", "attempt", in); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank evidence: got %v", err)
	}
	if repo.calls != 1 {
		t.Fatalf("persistence calls = %d", repo.calls)
	}
}
