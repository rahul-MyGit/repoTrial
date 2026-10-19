package diagnosis

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ db *pgxpool.Pool }

func NewPostgres(db *pgxpool.Pool) *Postgres { return &Postgres{db: db} }

func (p *Postgres) Record(ctx context.Context, userID, attemptID string, in Input) (Diagnosis, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return Diagnosis{}, fmt.Errorf("begin diagnosis: %w", err)
	}
	defer tx.Rollback(ctx)

	var state string
	err = tx.QueryRow(ctx, `SELECT state FROM attempts
		WHERE id = $1 AND user_id = $2 FOR UPDATE`, attemptID, userID).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return Diagnosis{}, ErrNotFound
	}
	if err != nil {
		return Diagnosis{}, fmt.Errorf("lock attempt: %w", err)
	}

	var sequence int
	err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1
		FROM diagnoses WHERE attempt_id = $1`, attemptID).Scan(&sequence)
	if err != nil {
		return Diagnosis{}, fmt.Errorf("number diagnosis: %w", err)
	}
	if sequence == 1 {
		if state != "baseline_ready" {
			return Diagnosis{}, ErrConflict
		}
		var completed bool
		err = tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM runs WHERE attempt_id = $1 AND kind = 'baseline'
			AND revision = 0 AND status = 'failed')`, attemptID).Scan(&completed)
		if err != nil {
			return Diagnosis{}, fmt.Errorf("check baseline run: %w", err)
		}
		if !completed {
			return Diagnosis{}, ErrConflict
		}
	} else if state != "investigating" {
		return Diagnosis{}, ErrConflict
	}

	var d Diagnosis
	err = tx.QueryRow(ctx, `INSERT INTO diagnoses
		(attempt_id, sequence, suspected_cause, error_evidence, first_inspection, confidence)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, attempt_id, sequence, suspected_cause, error_evidence,
		first_inspection, confidence, created_at`, attemptID, sequence,
		in.SuspectedCause, in.ErrorEvidence, in.FirstInspection, in.Confidence).
		Scan(&d.ID, &d.AttemptID, &d.Sequence, &d.SuspectedCause,
			&d.ErrorEvidence, &d.FirstInspection, &d.Confidence, &d.CreatedAt)
	if err != nil {
		return Diagnosis{}, fmt.Errorf("insert diagnosis: %w", err)
	}
	if sequence == 1 {
		_, err = tx.Exec(ctx, `UPDATE attempts SET state = 'investigating', updated_at = now()
			WHERE id = $1`, attemptID)
		if err != nil {
			return Diagnosis{}, fmt.Errorf("unlock attempt: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Diagnosis{}, fmt.Errorf("commit diagnosis: %w", err)
	}
	return d, nil
}

func (p *Postgres) List(ctx context.Context, userID, attemptID string) ([]Diagnosis, error) {
	var exists bool
	err := p.db.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM attempts WHERE id = $1 AND user_id = $2)`, attemptID, userID).
		Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("authorize diagnosis list: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := p.db.Query(ctx, `SELECT id, attempt_id, sequence, suspected_cause,
		error_evidence, first_inspection, confidence, created_at
		FROM diagnoses WHERE attempt_id = $1 ORDER BY sequence`, attemptID)
	if err != nil {
		return nil, fmt.Errorf("list diagnoses: %w", err)
	}
	defer rows.Close()

	items := []Diagnosis{}
	for rows.Next() {
		var d Diagnosis
		if err := rows.Scan(&d.ID, &d.AttemptID, &d.Sequence, &d.SuspectedCause,
			&d.ErrorEvidence, &d.FirstInspection, &d.Confidence, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan diagnosis: %w", err)
		}
		items = append(items, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate diagnoses: %w", err)
	}
	return items, nil
}
