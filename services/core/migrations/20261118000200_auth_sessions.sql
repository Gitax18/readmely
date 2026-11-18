-- +goose Up

-- Passwordless sign-in (plans/phase-2a-email-login.md). A login challenge is one emailed code.
-- The code itself is never stored: code_hash is HMAC-SHA-256 of it under a server key, because a
-- 6-digit code is too small to protect with a plain hash. closed_at is set when the code is used
-- or replaced by a newer one; either way it can never be used again.
CREATE TABLE login_challenges (
    id           uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    email        text        NOT NULL,
    code_hash    bytea       NOT NULL,
    attempts     integer     NOT NULL DEFAULT 0,
    max_attempts integer     NOT NULL DEFAULT 5,
    expires_at   timestamptz NOT NULL,
    closed_at    timestamptz,
    requested_ip inet,
    created_at   timestamptz NOT NULL DEFAULT now(),

    -- Stored normalised, so one address is one key.
    CONSTRAINT login_challenges_email_check CHECK (
        email = lower(btrim(email)) AND char_length(email) BETWEEN 3 AND 254 AND strpos(email, '@') > 1
    ),
    CONSTRAINT login_challenges_code_hash_check CHECK (octet_length(code_hash) = 32),
    CONSTRAINT login_challenges_max_attempts_check CHECK (max_attempts BETWEEN 1 AND 10),
    CONSTRAINT login_challenges_attempts_check CHECK (attempts BETWEEN 0 AND max_attempts),
    -- A code lives at most an hour (the service uses 10 minutes).
    CONSTRAINT login_challenges_expires_at_check CHECK (expires_at > created_at AND expires_at <= created_at + interval '1 hour')
);

-- At most one usable code per address. login_challenge_create closes the old one first.
CREATE UNIQUE INDEX login_challenges_email_open_key ON login_challenges (email) WHERE closed_at IS NULL;
-- Recent requests for an address, for abuse review; and the retention cleanup.
CREATE INDEX login_challenges_email_created_at_idx ON login_challenges (email, created_at DESC);
CREATE INDEX login_challenges_created_at_idx ON login_challenges (created_at);

-- A session is a random 256-bit token held in the browser's cookie; only its SHA-256 is stored.
-- It ends at idle_expires_at, which slides forward with use but never past expires_at.
CREATE TABLE sessions (
    id              uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash      bytea       NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    idle_expires_at timestamptz NOT NULL,
    user_agent      text        NOT NULL DEFAULT '',
    ip              inet,

    CONSTRAINT sessions_token_hash_check CHECK (octet_length(token_hash) = 32),
    CONSTRAINT sessions_token_hash_key UNIQUE (token_hash),
    CONSTRAINT sessions_expires_at_check CHECK (expires_at > created_at AND expires_at <= created_at + interval '90 days'),
    CONSTRAINT sessions_idle_expires_at_check CHECK (idle_expires_at <= expires_at),
    CONSTRAINT sessions_user_agent_check CHECK (char_length(user_agent) <= 200)
);

-- A user's sessions, newest first (also serves the foreign key).
CREATE INDEX sessions_user_id_created_at_idx ON sessions (user_id, created_at DESC);
-- Cleanup: idle_expires_at <= expires_at, so this one column finds every ended session.
CREATE INDEX sessions_idle_expires_at_idx ON sessions (idle_expires_at);

-- Issues a new code for an address: closes any open one and stores the new one, under a lock per
-- address, so concurrent requests cannot leave two usable codes. Returns the new challenge.
-- +goose StatementBegin
CREATE FUNCTION login_challenge_create(
    p_email text, p_code_hash bytea, p_ttl_seconds integer, p_max_attempts integer, p_requested_ip inet
) RETURNS login_challenges
LANGUAGE plpgsql AS $$
DECLARE
    created login_challenges;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended('login:' || p_email, 0));
    UPDATE login_challenges SET closed_at = now() WHERE email = p_email AND closed_at IS NULL;
    INSERT INTO login_challenges (email, code_hash, max_attempts, expires_at, requested_ip)
    VALUES (p_email, p_code_hash, p_max_attempts, now() + make_interval(secs => p_ttl_seconds), p_requested_ip)
    RETURNING * INTO created;
    RETURN created;
END
$$;
-- +goose StatementEnd

-- New functions are private by default (audit F12); only the API may issue codes. The AI role
-- gets nothing on these tables.
GRANT EXECUTE ON FUNCTION login_challenge_create(text, bytea, integer, integer, inet) TO readmely_app;

-- +goose Down
DROP FUNCTION login_challenge_create(text, bytea, integer, integer, inet);
DROP TABLE sessions;
DROP TABLE login_challenges;
