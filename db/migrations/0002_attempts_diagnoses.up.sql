CREATE TABLE attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    problem_id uuid NOT NULL REFERENCES problems(id) ON DELETE RESTRICT,
    challenge_version_id uuid NOT NULL,
    state text NOT NULL DEFAULT 'created',
    current_revision integer NOT NULL DEFAULT 0,
    create_idempotency_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    submitted_at timestamptz,
    FOREIGN KEY (challenge_version_id, problem_id) REFERENCES challenge_versions(id, problem_id) ON DELETE RESTRICT,
    UNIQUE (user_id, create_idempotency_key),
    CONSTRAINT attempts_state CHECK (state IN ('created', 'baseline_queued', 'baseline_ready', 'investigating', 'submitted')),
    CONSTRAINT attempts_revision_nonnegative CHECK (current_revision >= 0),
    CONSTRAINT attempts_submitted_at_state CHECK ((state = 'submitted') = (submitted_at IS NOT NULL)),
    CONSTRAINT attempts_idempotency_nonempty CHECK (length(create_idempotency_key) BETWEEN 1 AND 128)
);
CREATE INDEX attempts_user_created_idx ON attempts(user_id, created_at DESC);
CREATE INDEX attempts_challenge_version_idx ON attempts(challenge_version_id);

CREATE TABLE attempt_revisions (
    attempt_id uuid NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
    revision integer NOT NULL CHECK (revision >= 0),
    parent_revision integer,
    source_kind text NOT NULL,
    source_patch_id uuid,
    file_overlay jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (attempt_id, revision),
    FOREIGN KEY (attempt_id, parent_revision) REFERENCES attempt_revisions(attempt_id, revision),
    CONSTRAINT attempt_revisions_source_kind CHECK (source_kind IN ('baseline', 'accepted_patch', 'revert')),
    CONSTRAINT attempt_revisions_parent_chain CHECK (
        (revision = 0 AND parent_revision IS NULL AND source_kind = 'baseline' AND source_patch_id IS NULL)
        OR (revision > 0 AND parent_revision = revision - 1 AND source_kind <> 'baseline')
    ),
    CONSTRAINT attempt_revisions_overlay_object CHECK (jsonb_typeof(file_overlay) = 'object')
);

CREATE TABLE diagnoses (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id uuid NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
    sequence integer NOT NULL CHECK (sequence >= 1),
    suspected_cause text NOT NULL,
    error_evidence text NOT NULL,
    first_inspection text NOT NULL,
    confidence integer CHECK (confidence BETWEEN 0 AND 100),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (attempt_id, sequence),
    CONSTRAINT diagnoses_fields_nonempty CHECK (
        length(btrim(suspected_cause)) > 0 AND
        length(btrim(error_evidence)) > 0 AND
        length(btrim(first_inspection)) > 0
    )
);
CREATE INDEX diagnoses_attempt_created_idx ON diagnoses(attempt_id, created_at);

CREATE OR REPLACE FUNCTION reject_immutable_row_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'immutable record cannot be changed';
END;
$$;
CREATE TRIGGER attempt_revisions_immutable
BEFORE UPDATE OR DELETE ON attempt_revisions
FOR EACH ROW EXECUTE FUNCTION reject_immutable_row_change();
CREATE TRIGGER diagnoses_immutable
BEFORE UPDATE OR DELETE ON diagnoses
FOR EACH ROW EXECUTE FUNCTION reject_immutable_row_change();
