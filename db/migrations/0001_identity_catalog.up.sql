CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    google_subject text NOT NULL UNIQUE,
    email text NOT NULL,
    display_name text NOT NULL,
    avatar_url text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_google_subject_nonempty CHECK (length(google_subject) > 0),
    CONSTRAINT users_email_nonempty CHECK (length(email) > 0)
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    csrf_token_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sessions_token_hash_sha256 CHECK (octet_length(token_hash) = 32),
    CONSTRAINT sessions_csrf_hash_sha256 CHECK (octet_length(csrf_token_hash) = 32),
    CONSTRAINT sessions_expiry_after_creation CHECK (expires_at > created_at)
);
CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_expiry_idx ON sessions(expires_at) WHERE revoked_at IS NULL;

CREATE TABLE problems (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    title text NOT NULL,
    summary text NOT NULL,
    difficulty text NOT NULL,
    language text NOT NULL DEFAULT 'go',
    tags text[] NOT NULL DEFAULT '{}',
    is_published boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT problems_slug_format CHECK (slug ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
    CONSTRAINT problems_difficulty CHECK (difficulty IN ('beginner', 'intermediate', 'advanced')),
    CONSTRAINT problems_language CHECK (language = 'go')
);
CREATE INDEX problems_published_idx ON problems(slug) WHERE is_published;

CREATE TABLE challenge_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    problem_id uuid NOT NULL REFERENCES problems(id) ON DELETE RESTRICT,
    version integer NOT NULL CHECK (version > 0),
    manifest_schema_version integer NOT NULL CHECK (manifest_schema_version = 1),
    manifest jsonb NOT NULL,
    artifact_uri text NOT NULL,
    artifact_sha256 text NOT NULL CHECK (artifact_sha256 ~ '^[0-9a-f]{64}$'),
    evaluation_ref text NOT NULL,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (problem_id, version),
    UNIQUE (id, problem_id),
    CONSTRAINT challenge_versions_manifest_object CHECK (jsonb_typeof(manifest) = 'object'),
    CONSTRAINT challenge_versions_artifact_uri_nonempty CHECK (length(artifact_uri) > 0),
    CONSTRAINT challenge_versions_evaluation_ref_nonempty CHECK (length(evaluation_ref) > 0)
);
CREATE INDEX challenge_versions_published_idx ON challenge_versions(problem_id, version DESC) WHERE published_at IS NOT NULL;

CREATE OR REPLACE FUNCTION prevent_published_challenge_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.published_at IS NOT NULL AND (TG_OP = 'DELETE' OR NEW IS DISTINCT FROM OLD) THEN
        RAISE EXCEPTION 'published challenge version is immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER challenge_versions_immutable
BEFORE UPDATE OR DELETE ON challenge_versions
FOR EACH ROW EXECUTE FUNCTION prevent_published_challenge_change();
