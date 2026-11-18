-- +goose Up

-- The work queue shared by the Go and Python workers (decision D2). Workers claim a job with
-- SELECT ... FOR UPDATE SKIP LOCKED, hold it under a lease, and renew the lease while working;
-- a crashed worker's lease runs out and the job is claimed again.
CREATE TABLE jobs (
    id               uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    user_id          uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    project_id       uuid,
    kind             text        NOT NULL,
    payload          jsonb       NOT NULL DEFAULT '{}',
    status           text        NOT NULL DEFAULT 'queued',
    attempts         integer     NOT NULL DEFAULT 0,
    max_attempts     integer     NOT NULL DEFAULT 3,
    run_after        timestamptz NOT NULL DEFAULT now(),
    locked_by        text,
    lease_expires_at timestamptz,
    idempotency_key  text        NOT NULL,
    error_code       text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    finished_at      timestamptz,

    -- The project must belong to the same user as the job. With a NULL project_id this is not checked.
    CONSTRAINT jobs_project_id_user_id_fkey FOREIGN KEY (project_id, user_id)
        REFERENCES projects (id, user_id) ON DELETE CASCADE,

    CONSTRAINT jobs_kind_check CHECK (kind IN ('ingest', 'generate_readme', 'generate_diagram', 'build_context_pack')),
    CONSTRAINT jobs_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    CONSTRAINT jobs_payload_check CHECK (jsonb_typeof(payload) = 'object' AND octet_length(payload::text) <= 65536),
    CONSTRAINT jobs_attempts_check CHECK (attempts >= 0 AND attempts <= max_attempts),
    CONSTRAINT jobs_max_attempts_check CHECK (max_attempts BETWEEN 1 AND 20),
    CONSTRAINT jobs_idempotency_key_check CHECK (char_length(idempotency_key) BETWEEN 8 AND 128),
    -- A short machine code, never a stack trace or text from a provider.
    CONSTRAINT jobs_error_code_check CHECK (error_code ~ '^[a-z][a-z0-9_]{0,63}$'),
    -- Only a running job has a worker and a lease, and a running job always has both.
    CONSTRAINT jobs_lease_check CHECK (
        (status = 'running') = (locked_by IS NOT NULL) AND (locked_by IS NULL) = (lease_expires_at IS NULL)
    ),
    CONSTRAINT jobs_locked_by_check CHECK (locked_by IS NULL OR char_length(locked_by) BETWEEN 1 AND 128),
    -- A finished job says when it finished, and an unfinished one does not.
    CONSTRAINT jobs_finished_at_check CHECK (
        (status IN ('succeeded', 'failed', 'cancelled')) = (finished_at IS NOT NULL)
    ),
    -- A retried request must not create a second job.
    CONSTRAINT jobs_user_id_idempotency_key_key UNIQUE (user_id, idempotency_key)
);

-- The claim query reads only queued jobs, oldest run_after first.
CREATE INDEX jobs_queued_run_after_idx ON jobs (run_after) WHERE status = 'queued';
-- Finding jobs whose worker disappeared.
CREATE INDEX jobs_running_lease_expires_at_idx ON jobs (lease_expires_at) WHERE status = 'running';
-- Serves the composite foreign key: deleting a project finds its jobs without a table scan.
CREATE INDEX jobs_project_id_user_id_idx ON jobs (project_id, user_id);

CREATE TRIGGER jobs_set_updated_at BEFORE UPDATE ON jobs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE jobs;
