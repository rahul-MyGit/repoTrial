package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ db *pgxpool.Pool }

func NewPostgres(db *pgxpool.Pool) *Postgres { return &Postgres{db: db} }

const runColumns = `id, attempt_id, revision, kind, status, stdout, stderr, exit_code, created_at, finished_at`
const joinedRunColumns = `r.id, r.attempt_id, r.revision, r.kind, r.status, r.stdout, r.stderr, r.exit_code, r.created_at, r.finished_at`

func scanRun(row pgx.Row) (Run, error) {
	var r Run
	err := row.Scan(&r.ID, &r.AttemptID, &r.Revision, &r.Kind, &r.Status,
		&r.Stdout, &r.Stderr, &r.ExitCode, &r.CreatedAt, &r.FinishedAt)
	return r, err
}

func (p *Postgres) Enqueue(ctx context.Context, req Request) (Run, error) {
	if req.UserID == "" || req.AttemptID == "" || len(req.IdempotencyKey) < 1 || len(req.IdempotencyKey) > 128 || req.Revision < 0 || (req.Kind != Baseline && req.Kind != Visible) {
		return Run{}, ErrInvalid
	}
	tx, err := p.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	var current int
	err = tx.QueryRow(ctx, `SELECT state, current_revision FROM attempts WHERE id=$1 AND user_id=$2 FOR UPDATE`, req.AttemptID, req.UserID).Scan(&state, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	// Idempotent retries return the original run, including after state changes.
	r, err := scanRun(tx.QueryRow(ctx, `SELECT `+runColumns+` FROM runs WHERE attempt_id=$1 AND idempotency_key=$2`, req.AttemptID, req.IdempotencyKey))
	if err == nil {
		if r.Kind != req.Kind || r.Revision != req.Revision {
			return Run{}, ErrConflict
		}
		return r, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Run{}, err
	}
	if req.Kind == Baseline {
		if req.Revision != 0 || (state != "created" && state != "baseline_ready") {
			return Run{}, ErrConflict
		}
	} else if state != "investigating" || current != req.Revision {
		return Run{}, ErrConflict
	}
	err = tx.QueryRow(ctx, `INSERT INTO runs (attempt_id,revision,kind,idempotency_key)
		VALUES ($1,$2,$3,$4) RETURNING `+runColumns, req.AttemptID, req.Revision, req.Kind, req.IdempotencyKey).Scan(
		&r.ID, &r.AttemptID, &r.Revision, &r.Kind, &r.Status, &r.Stdout, &r.Stderr, &r.ExitCode, &r.CreatedAt, &r.FinishedAt)
	if err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.Code == "23505" {
			return Run{}, ErrConflict
		}
		return Run{}, err
	}
	if req.Kind == Baseline {
		_, err = tx.Exec(ctx, `UPDATE attempts SET state='baseline_queued', updated_at=now() WHERE id=$1`, req.AttemptID)
		if err != nil {
			return Run{}, err
		}
	}
	return r, tx.Commit(ctx)
}

func (p *Postgres) Get(ctx context.Context, userID, attemptID, runID string) (Run, error) {
	r, err := scanRun(p.db.QueryRow(ctx, `SELECT `+joinedRunColumns+` FROM runs r JOIN attempts a ON a.id=r.attempt_id
		WHERE r.id=$1 AND r.attempt_id=$2 AND a.user_id=$3`, runID, attemptID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return r, err
}

func (p *Postgres) List(ctx context.Context, userID, attemptID string) ([]Run, error) {
	rows, err := p.db.Query(ctx, `SELECT `+joinedRunColumns+`
		FROM runs r JOIN attempts a ON a.id=r.attempt_id WHERE a.user_id=$1 AND a.id=$2 ORDER BY r.created_at DESC`, userID, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

// Claim uses row locks so multiple dispatchers cannot claim the same run.
func (p *Postgres) Claim(ctx context.Context, workerID string, lease time.Duration) (Snapshot, bool, error) {
	if workerID == "" || lease <= 0 {
		return Snapshot{}, false, ErrInvalid
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return Snapshot{}, false, err
	}
	defer tx.Rollback(ctx)
	var s Snapshot
	var manifest, overlay []byte
	err = tx.QueryRow(ctx, `SELECT r.id,r.attempt_id,r.revision,r.kind,v.artifact_uri,v.manifest,ar.file_overlay
		FROM runs r JOIN attempts a ON a.id=r.attempt_id
		JOIN challenge_versions v ON v.id=a.challenge_version_id
		JOIN attempt_revisions ar ON ar.attempt_id=r.attempt_id AND ar.revision=r.revision
		WHERE r.status='queued' ORDER BY r.created_at FOR UPDATE OF r SKIP LOCKED LIMIT 1`).Scan(
		&s.RunID, &s.AttemptID, &s.Revision, &s.Kind, &s.ArtifactURI, &manifest, &overlay)
	if errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, err
	}
	if err := json.Unmarshal(manifest, &s.Manifest); err != nil {
		return Snapshot{}, false, fmt.Errorf("decode manifest: %w", err)
	}
	if err := json.Unmarshal(overlay, &s.Overlay); err != nil {
		return Snapshot{}, false, fmt.Errorf("decode revision: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE runs SET status='running',worker_id=$2,lease_expires_at=now()+$3::interval,
		dispatch_attempts=dispatch_attempts+1,started_at=now() WHERE id=$1`, s.RunID, workerID, lease.String())
	if err != nil {
		return Snapshot{}, false, err
	}
	return s, true, tx.Commit(ctx)
}

func (p *Postgres) Finish(ctx context.Context, s Snapshot, workerID string, result Result) error {
	if result.Status != Passed && result.Status != Failed && result.Status != InfraFailed && result.Status != TimedOut {
		return ErrInvalid
	}
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var active bool
	err = tx.QueryRow(ctx, `UPDATE runs SET status=$3,stdout=$4,stderr=$5,exit_code=$6,finished_at=now(),
		lease_expires_at=NULL WHERE id=$1 AND worker_id=$2 AND status='running' RETURNING true`, s.RunID, workerID,
		result.Status, result.Stdout, result.Stderr, result.ExitCode).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if s.Kind == Baseline {
		if result.Status == Failed && result.ExitCode != nil && *result.ExitCode == s.Manifest.BaselineExpected.ExitCode &&
			containsExpected(result.Stdout, result.Stderr, s.Manifest.BaselineExpected.OutputContains) {
			_, err = tx.Exec(ctx, `UPDATE attempts SET state='baseline_ready',updated_at=now() WHERE id=$1 AND state='baseline_queued'`, s.AttemptID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE attempts SET state='created',updated_at=now() WHERE id=$1 AND state='baseline_queued'`, s.AttemptID)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) RequeueExpired(ctx context.Context, maxDispatches int) error {
	if maxDispatches < 1 {
		return ErrInvalid
	}
	_, err := p.db.Exec(ctx, `UPDATE runs SET status='queued',worker_id=NULL,lease_expires_at=NULL
		WHERE status='running' AND lease_expires_at < now() AND dispatch_attempts < $1`, maxDispatches)
	if err != nil {
		return err
	}
	_, err = p.db.Exec(ctx, `WITH expired AS (
		UPDATE runs SET status='infra_failed',failure_code='lease_expired',finished_at=now(),lease_expires_at=NULL
		WHERE status='running' AND lease_expires_at < now() AND dispatch_attempts >= $1 RETURNING attempt_id,kind
	) UPDATE attempts a SET state='created',updated_at=now() FROM expired e
	WHERE a.id=e.attempt_id AND e.kind='baseline' AND a.state='baseline_queued'`, maxDispatches)
	return err
}
