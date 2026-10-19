package attempts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ db *pgxpool.Pool }

func NewPostgres(db *pgxpool.Pool) *Postgres { return &Postgres{db: db} }

const attemptColumns = `a.id, p.slug, v.version, a.state, a.current_revision, a.created_at, a.submitted_at`

func (p *Postgres) Create(ctx context.Context, userID, slug, key string) (Attempt, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return Attempt{}, fmt.Errorf("begin attempt creation: %w", err)
	}
	defer tx.Rollback(ctx)
	var existingID, existingSlug string
	err = tx.QueryRow(ctx, `SELECT a.id, p.slug FROM attempts a
		JOIN problems p ON p.id = a.problem_id
		WHERE a.user_id = $1 AND a.create_idempotency_key = $2`, userID, key).
		Scan(&existingID, &existingSlug)
	if err == nil {
		if existingSlug != slug {
			return Attempt{}, ErrConflict
		}
		return get(ctx, tx, userID, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Attempt{}, fmt.Errorf("find existing attempt: %w", err)
	}

	var problemID, versionID string
	err = tx.QueryRow(ctx, `SELECT p.id, v.id FROM problems p
		JOIN LATERAL (SELECT id FROM challenge_versions
			WHERE problem_id = p.id AND published_at IS NOT NULL
			ORDER BY version DESC LIMIT 1) v ON true
		WHERE p.slug = $1 AND p.is_published`, slug).Scan(&problemID, &versionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attempt{}, ErrNotFound
	}
	if err != nil {
		return Attempt{}, fmt.Errorf("find published challenge: %w", err)
	}

	var id string
	err = tx.QueryRow(ctx, `INSERT INTO attempts
		(user_id, problem_id, challenge_version_id, create_idempotency_key)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, create_idempotency_key) DO NOTHING RETURNING id`,
		userID, problemID, versionID, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingProblem string
		err = tx.QueryRow(ctx, `SELECT id, problem_id FROM attempts
			WHERE user_id = $1 AND create_idempotency_key = $2`, userID, key).
			Scan(&id, &existingProblem)
		if err != nil {
			return Attempt{}, fmt.Errorf("find idempotent attempt: %w", err)
		}
		if existingProblem != problemID {
			return Attempt{}, ErrConflict
		}
	} else if err != nil {
		return Attempt{}, fmt.Errorf("insert attempt: %w", err)
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO attempt_revisions
			(attempt_id, revision, source_kind, file_overlay)
			VALUES ($1, 0, 'baseline', '{}'::jsonb)`, id)
		if err != nil {
			return Attempt{}, fmt.Errorf("insert baseline revision: %w", err)
		}
	}

	a, err := get(ctx, tx, userID, id)
	if err != nil {
		return Attempt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Attempt{}, fmt.Errorf("commit attempt: %w", err)
	}
	return a, nil
}

func (p *Postgres) Get(ctx context.Context, userID, attemptID string) (Attempt, error) {
	return get(ctx, p.db, userID, attemptID)
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func get(ctx context.Context, q queryRower, userID, attemptID string) (Attempt, error) {
	var a Attempt
	err := q.QueryRow(ctx, `SELECT `+attemptColumns+` FROM attempts a
		JOIN problems p ON p.id = a.problem_id
		JOIN challenge_versions v ON v.id = a.challenge_version_id
		WHERE a.id = $1 AND a.user_id = $2`, attemptID, userID).
		Scan(&a.ID, &a.ProblemSlug, &a.ChallengeVersion, &a.State,
			&a.Revision, &a.CreatedAt, &a.SubmittedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attempt{}, ErrNotFound
	}
	if err != nil {
		return Attempt{}, fmt.Errorf("get attempt: %w", err)
	}
	return a, nil
}
