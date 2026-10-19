CREATE TABLE ai_turns (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id uuid NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
    base_revision integer NOT NULL,
    idempotency_key text NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    prompt text NOT NULL,
    selected_path text,
    response text,
    provider text,
    model text,
    input_tokens integer CHECK (input_tokens >= 0),
    output_tokens integer CHECK (output_tokens >= 0),
    failure_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    FOREIGN KEY (attempt_id, base_revision) REFERENCES attempt_revisions(attempt_id, revision),
    UNIQUE (attempt_id, idempotency_key),
    UNIQUE (id, attempt_id),
    CONSTRAINT ai_turns_status CHECK (status IN ('queued', 'running', 'completed', 'failed')),
    CONSTRAINT ai_turns_prompt_nonempty CHECK (length(btrim(prompt)) > 0),
    CONSTRAINT ai_turns_idempotency_nonempty CHECK (length(idempotency_key) BETWEEN 1 AND 128)
);
CREATE UNIQUE INDEX ai_turns_one_active_idx ON ai_turns(attempt_id) WHERE status IN ('queued', 'running');
CREATE INDEX ai_turns_attempt_created_idx ON ai_turns(attempt_id, created_at);

CREATE TABLE patch_proposals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id uuid NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
    ai_turn_id uuid NOT NULL,
    base_revision integer NOT NULL,
    status text NOT NULL DEFAULT 'proposed',
    diff text NOT NULL,
    explanation text NOT NULL DEFAULT '',
    file_hashes jsonb NOT NULL,
    result_revision integer,
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    FOREIGN KEY (attempt_id, base_revision) REFERENCES attempt_revisions(attempt_id, revision),
    FOREIGN KEY (ai_turn_id, attempt_id) REFERENCES ai_turns(id, attempt_id) ON DELETE RESTRICT,
    UNIQUE (ai_turn_id),
    UNIQUE (id, attempt_id),
    CONSTRAINT patch_proposals_status CHECK (status IN ('proposed', 'accepted', 'rejected', 'stale')),
    CONSTRAINT patch_proposals_file_hashes_object CHECK (jsonb_typeof(file_hashes) = 'object'),
    CONSTRAINT patch_proposals_diff_nonempty CHECK (length(diff) > 0),
    CONSTRAINT patch_proposals_result_revision CHECK (
        (status = 'accepted' AND result_revision = base_revision + 1)
        OR (status <> 'accepted' AND result_revision IS NULL)
    ),
    CONSTRAINT patch_proposals_decision_time CHECK ((status = 'proposed') = (decided_at IS NULL))
);
CREATE INDEX patch_proposals_attempt_created_idx ON patch_proposals(attempt_id, created_at);
ALTER TABLE attempt_revisions
    ADD CONSTRAINT attempt_revisions_source_patch_fk
    FOREIGN KEY (source_patch_id, attempt_id) REFERENCES patch_proposals(id, attempt_id) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id uuid NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
    revision integer NOT NULL,
    kind text NOT NULL,
    idempotency_key text NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    worker_id text,
    lease_expires_at timestamptz,
    dispatch_attempts integer NOT NULL DEFAULT 0,
    stdout text,
    stderr text,
    exit_code integer,
    failure_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    FOREIGN KEY (attempt_id, revision) REFERENCES attempt_revisions(attempt_id, revision),
    UNIQUE (attempt_id, idempotency_key),
    UNIQUE (id, attempt_id),
    CONSTRAINT runs_kind CHECK (kind IN ('baseline', 'visible', 'private')),
    CONSTRAINT runs_status CHECK (status IN ('queued', 'running', 'passed', 'failed', 'infra_failed', 'timed_out')),
    CONSTRAINT runs_dispatch_attempts_nonnegative CHECK (dispatch_attempts >= 0),
    CONSTRAINT runs_idempotency_nonempty CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    CONSTRAINT runs_baseline_revision CHECK (kind <> 'baseline' OR revision = 0)
);
CREATE UNIQUE INDEX runs_one_active_idx ON runs(attempt_id) WHERE status IN ('queued', 'running');
CREATE INDEX runs_dispatch_idx ON runs(created_at) WHERE status = 'queued';
CREATE INDEX runs_lease_idx ON runs(lease_expires_at) WHERE status = 'running';
CREATE INDEX runs_attempt_created_idx ON runs(attempt_id, created_at DESC);

CREATE TABLE submissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id uuid NOT NULL UNIQUE REFERENCES attempts(id) ON DELETE RESTRICT,
    revision integer NOT NULL,
    run_id uuid UNIQUE,
    idempotency_key text NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    feedback jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    FOREIGN KEY (attempt_id, revision) REFERENCES attempt_revisions(attempt_id, revision),
    FOREIGN KEY (run_id, attempt_id) REFERENCES runs(id, attempt_id) ON DELETE RESTRICT,
    CONSTRAINT submissions_status CHECK (status IN ('queued', 'running', 'passed', 'failed', 'infra_failed')),
    CONSTRAINT submissions_idempotency_nonempty CHECK (length(idempotency_key) BETWEEN 1 AND 128)
);
