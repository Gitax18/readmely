-- +goose Up

-- A summary job runs after ingestion and writes what a project does in plain English.
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('ingest', 'summarise', 'generate_readme', 'generate_diagram', 'build_context_pack'));

-- Every call to a model provider, with what it cost. This is also how the daily spend cap is
-- enforced, so a row is written for failures too. It is kept when the person or job is deleted
-- (the references are cleared) because the provider bills us either way.
CREATE TABLE llm_calls (
    id              uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    user_id         uuid        REFERENCES users (id) ON DELETE SET NULL,
    job_id          uuid        REFERENCES jobs (id) ON DELETE SET NULL,
    provider        text        NOT NULL,
    model           text        NOT NULL,
    tier            text        NOT NULL,
    purpose         text        NOT NULL,
    input_tokens    integer     NOT NULL DEFAULT 0,
    output_tokens   integer     NOT NULL DEFAULT 0,
    -- Millionths of a US dollar, so no rounding hides small calls.
    cost_micro_usd  bigint      NOT NULL DEFAULT 0,
    status          text        NOT NULL,
    error_code      text,
    duration_ms     integer     NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT llm_calls_provider_check CHECK (provider IN ('openai')),
    CONSTRAINT llm_calls_model_check CHECK (char_length(model) BETWEEN 1 AND 100),
    CONSTRAINT llm_calls_tier_check CHECK (tier IN ('fast', 'quality', 'embedding')),
    CONSTRAINT llm_calls_purpose_check CHECK (purpose ~ '^[a-z][a-z0-9_]{0,63}$'),
    CONSTRAINT llm_calls_tokens_check CHECK (input_tokens >= 0 AND output_tokens >= 0),
    CONSTRAINT llm_calls_cost_check CHECK (cost_micro_usd >= 0),
    CONSTRAINT llm_calls_status_check CHECK (status IN ('ok', 'error')),
    CONSTRAINT llm_calls_error_code_check CHECK (error_code ~ '^[a-z][a-z0-9_]{0,63}$'),
    CONSTRAINT llm_calls_duration_check CHECK (duration_ms >= 0)
);
CREATE INDEX llm_calls_created_at_idx ON llm_calls (created_at);
CREATE INDEX llm_calls_user_id_idx ON llm_calls (user_id);
CREATE INDEX llm_calls_job_id_idx ON llm_calls (job_id);

-- One summary document per project, replaced on every analysis. Its shape belongs to
-- contracts/core-public.yaml (ProjectSummary).
CREATE TABLE project_summaries (
    id          uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    project_id  uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    commit_sha  text        NOT NULL,
    model       text        NOT NULL,
    summary     jsonb       NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT project_summaries_project_id_key UNIQUE (project_id),
    CONSTRAINT project_summaries_commit_sha_check CHECK (commit_sha ~ '^[0-9a-f]{40}$'),
    CONSTRAINT project_summaries_model_check CHECK (char_length(model) BETWEEN 1 AND 100),
    CONSTRAINT project_summaries_summary_check CHECK (jsonb_typeof(summary) = 'object' AND octet_length(summary::text) <= 32768)
);
CREATE TRIGGER project_summaries_set_updated_at BEFORE UPDATE ON project_summaries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The AI service logs in as readmely_ai, which can read only what summarising needs and write only
-- its own results. It cannot read users, sessions or anything else, and it cannot touch the credit
-- ledger directly: it may only settle or refund a hold through the two functions below.
GRANT SELECT, UPDATE ON jobs TO readmely_ai;
GRANT SELECT ON projects, files, project_facts, graph_nodes, graph_edges TO readmely_ai;
GRANT SELECT, INSERT, UPDATE ON project_summaries TO readmely_ai;
GRANT SELECT, INSERT ON llm_calls TO readmely_ai;

-- Settles the hold placed for a job: charges p_charge (never more than was held) and returns the rest. Calling it again
-- returns the first answer and changes nothing.
-- +goose StatementBegin
CREATE FUNCTION credit_settle(p_job_id uuid, p_charge bigint)
RETURNS credit_ledger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
    held credit_ledger;
    existing credit_ledger;
    entry credit_ledger;
BEGIN
    IF p_charge IS NULL OR p_charge < 0 THEN
        RAISE EXCEPTION 'the charge must not be negative' USING ERRCODE = 'invalid_parameter_value';
    END IF;
    SELECT * INTO held FROM credit_ledger WHERE job_id = p_job_id AND kind = 'reserve' ORDER BY created_at LIMIT 1;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'no hold for this job' USING ERRCODE = 'invalid_parameter_value';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended('credit:' || held.user_id::text, 0));
    SELECT * INTO existing FROM credit_ledger WHERE reservation_id = held.id;
    IF FOUND THEN
        RETURN existing;
    END IF;
    INSERT INTO credit_ledger (user_id, delta, kind, job_id, reservation_id, idempotency_key, note)
    VALUES (held.user_id, -held.delta - LEAST(p_charge, -held.delta), 'settle', held.job_id, held.id,
            'settle:' || held.id::text, 'Unused part of the hold returned')
    RETURNING * INTO entry;
    RETURN entry;
END
$$;
-- +goose StatementEnd

-- Gives the whole hold of a job back, for a job that did not deliver.
-- +goose StatementBegin
CREATE FUNCTION credit_refund(p_job_id uuid)
RETURNS credit_ledger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
    held credit_ledger;
    existing credit_ledger;
    entry credit_ledger;
BEGIN
    SELECT * INTO held FROM credit_ledger WHERE job_id = p_job_id AND kind = 'reserve' ORDER BY created_at LIMIT 1;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'no hold for this job' USING ERRCODE = 'invalid_parameter_value';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended('credit:' || held.user_id::text, 0));
    SELECT * INTO existing FROM credit_ledger WHERE reservation_id = held.id;
    IF FOUND THEN
        RETURN existing;
    END IF;
    INSERT INTO credit_ledger (user_id, delta, kind, job_id, reservation_id, idempotency_key, note)
    VALUES (held.user_id, -held.delta, 'refund', held.job_id, held.id, 'refund:' || held.id::text, 'Refunded: the job did not finish')
    RETURNING * INTO entry;
    RETURN entry;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION credit_settle(uuid, bigint), credit_refund(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION credit_settle(uuid, bigint), credit_refund(uuid) TO readmely_ai;

-- +goose Down
DROP FUNCTION credit_refund(uuid);
DROP FUNCTION credit_settle(uuid, bigint);
REVOKE SELECT, UPDATE ON jobs FROM readmely_ai;
REVOKE SELECT ON projects, files, project_facts, graph_nodes, graph_edges FROM readmely_ai;
DROP TABLE project_summaries;
DROP TABLE llm_calls;
DELETE FROM jobs WHERE kind = 'summarise';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('ingest', 'generate_readme', 'generate_diagram', 'build_context_pack'));
