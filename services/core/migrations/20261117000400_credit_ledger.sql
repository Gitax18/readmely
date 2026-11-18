-- +goose Up

-- Append-only record of every credit movement (threat T11). The balance is the sum of delta.
--   grant       credits given to the user                 delta > 0
--   reserve     credits held before a job starts          delta < 0
--   settle      the unused part of a reserve is returned  delta >= 0, at most what was reserved
--   refund      a failed job gives the whole reserve back delta = the reserve's size
--   adjustment  a manual correction                       delta <> 0
CREATE TABLE credit_ledger (
    id              uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    -- Cleared (not deleted) when the account is deleted, so history stays without the person.
    user_id         uuid        REFERENCES users (id) ON DELETE SET NULL,
    delta           bigint      NOT NULL,
    kind            text        NOT NULL,
    job_id          uuid        REFERENCES jobs (id) ON DELETE SET NULL,
    reservation_id  uuid        REFERENCES credit_ledger (id) ON DELETE RESTRICT,
    idempotency_key text        NOT NULL,
    note            text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT credit_ledger_kind_check CHECK (kind IN ('grant', 'reserve', 'settle', 'refund', 'adjustment')),
    CONSTRAINT credit_ledger_delta_check CHECK (
        CASE kind
            WHEN 'grant'      THEN delta > 0
            WHEN 'reserve'    THEN delta < 0
            WHEN 'settle'     THEN delta >= 0
            WHEN 'refund'     THEN delta > 0
            WHEN 'adjustment' THEN delta <> 0
        END
    ),
    -- Only a settle or a refund points at a reserve, and each must.
    CONSTRAINT credit_ledger_reservation_id_check CHECK ((kind IN ('settle', 'refund')) = (reservation_id IS NOT NULL)),
    CONSTRAINT credit_ledger_idempotency_key_check CHECK (char_length(idempotency_key) BETWEEN 8 AND 128),
    CONSTRAINT credit_ledger_note_check CHECK (char_length(note) <= 500),
    -- A retried request must not charge twice.
    CONSTRAINT credit_ledger_user_id_idempotency_key_key UNIQUE (user_id, idempotency_key),
    -- A reserve is settled or refunded at most once.
    CONSTRAINT credit_ledger_reservation_id_key UNIQUE (reservation_id)
);

CREATE INDEX credit_ledger_user_id_created_at_idx ON credit_ledger (user_id, created_at DESC);
CREATE INDEX credit_ledger_job_id_idx ON credit_ledger (job_id);

CREATE VIEW credit_balances AS
    SELECT user_id, sum(delta)::bigint AS balance
    FROM credit_ledger
    WHERE user_id IS NOT NULL
    GROUP BY user_id;

-- Rows are never changed or removed. The one allowed change is what the foreign keys do when a
-- user or a job is deleted: user_id or job_id becomes NULL. Nothing else may differ.
-- +goose StatementBegin
CREATE FUNCTION credit_ledger_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF (NEW.id, NEW.delta, NEW.kind, NEW.reservation_id, NEW.idempotency_key, NEW.note, NEW.created_at)
               IS DISTINCT FROM
           (OLD.id, OLD.delta, OLD.kind, OLD.reservation_id, OLD.idempotency_key, OLD.note, OLD.created_at)
           OR (NEW.user_id IS NOT NULL AND NEW.user_id IS DISTINCT FROM OLD.user_id)
           OR (NEW.job_id IS NOT NULL AND NEW.job_id IS DISTINCT FROM OLD.job_id)
        THEN
            RAISE EXCEPTION 'credit_ledger is append-only: only user_id and job_id may be cleared'
                USING ERRCODE = 'restrict_violation';
        END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'credit_ledger is append-only' USING ERRCODE = 'restrict_violation';
END
$$;
-- +goose StatementEnd

CREATE TRIGGER credit_ledger_no_change BEFORE UPDATE OR DELETE ON credit_ledger
    FOR EACH ROW EXECUTE FUNCTION credit_ledger_guard();
CREATE TRIGGER credit_ledger_no_truncate BEFORE TRUNCATE ON credit_ledger
    FOR EACH STATEMENT EXECUTE FUNCTION credit_ledger_guard();

-- Rules that look at another row: a settle or refund must answer a real reserve of the same
-- user, and cannot give back more than was held. New rows always belong to a user.
-- +goose StatementBegin
CREATE FUNCTION credit_ledger_check_entry() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    held credit_ledger;
BEGIN
    IF NEW.user_id IS NULL THEN
        RAISE EXCEPTION 'a ledger entry needs a user' USING ERRCODE = 'check_violation', CONSTRAINT = 'credit_ledger_user_id_check';
    END IF;
    -- A settle or refund with no reservation_id is left to credit_ledger_reservation_id_check.
    IF NEW.kind IN ('settle', 'refund') AND NEW.reservation_id IS NOT NULL THEN
        SELECT * INTO held FROM credit_ledger WHERE id = NEW.reservation_id;
        IF NOT FOUND OR held.kind <> 'reserve' OR held.user_id IS DISTINCT FROM NEW.user_id THEN
            RAISE EXCEPTION 'a % must answer a reserve of the same user', NEW.kind
                USING ERRCODE = 'check_violation', CONSTRAINT = 'credit_ledger_resolution_check';
        END IF;
        IF NEW.delta > -held.delta OR (NEW.kind = 'refund' AND NEW.delta <> -held.delta) THEN
            RAISE EXCEPTION 'a % cannot return more than was reserved (a refund returns exactly the reserve)', NEW.kind
                USING ERRCODE = 'check_violation', CONSTRAINT = 'credit_ledger_resolution_check';
        END IF;
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER credit_ledger_check_entry BEFORE INSERT ON credit_ledger
    FOR EACH ROW EXECUTE FUNCTION credit_ledger_check_entry();

-- Holds credits for a job. Concurrent calls for one user queue up on a transaction-scoped lock,
-- so the balance check and the insert cannot interleave and the balance never goes negative.
-- Calling again with the same key and arguments returns the first entry and charges nothing.
--   CR001 insufficient credits       CR002 the key was already used for something else
-- +goose StatementBegin
CREATE FUNCTION credit_reserve(p_user_id uuid, p_amount bigint, p_job_id uuid, p_idempotency_key text)
RETURNS credit_ledger
LANGUAGE plpgsql AS $$
DECLARE
    existing credit_ledger;
    available bigint;
    entry credit_ledger;
BEGIN
    IF p_amount IS NULL OR p_amount <= 0 THEN
        RAISE EXCEPTION 'the amount to reserve must be positive' USING ERRCODE = 'invalid_parameter_value';
    END IF;

    PERFORM pg_advisory_xact_lock(hashtextextended('credit:' || p_user_id::text, 0));

    SELECT * INTO existing FROM credit_ledger WHERE user_id = p_user_id AND idempotency_key = p_idempotency_key;
    IF FOUND THEN
        IF existing.kind <> 'reserve' OR existing.delta <> -p_amount OR existing.job_id IS DISTINCT FROM p_job_id THEN
            RAISE EXCEPTION 'the idempotency key was already used for a different request' USING ERRCODE = 'CR002';
        END IF;
        RETURN existing;
    END IF;

    SELECT COALESCE(sum(delta), 0) INTO available FROM credit_ledger WHERE user_id = p_user_id;
    IF available < p_amount THEN
        RAISE EXCEPTION 'insufficient credits' USING ERRCODE = 'CR001';
    END IF;

    INSERT INTO credit_ledger (user_id, delta, kind, job_id, idempotency_key)
    VALUES (p_user_id, -p_amount, 'reserve', p_job_id, p_idempotency_key)
    RETURNING * INTO entry;
    RETURN entry;
END
$$;
-- +goose StatementEnd

-- Only the application role may spend credits; it may add rows but never change or remove them
-- (the trigger enforces that for everyone, this makes the database say no even earlier).
REVOKE UPDATE, DELETE ON credit_ledger FROM readmely_app;
REVOKE ALL ON FUNCTION credit_reserve(uuid, bigint, uuid, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION credit_reserve(uuid, bigint, uuid, text) TO readmely_app;

-- +goose Down
DROP FUNCTION credit_reserve(uuid, bigint, uuid, text);
DROP VIEW credit_balances;
DROP TABLE credit_ledger;
DROP FUNCTION credit_ledger_check_entry();
DROP FUNCTION credit_ledger_guard();
