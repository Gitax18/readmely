-- Sign-in and sessions (plans/phase-2a-email-login.md). Codes and session tokens are never
-- stored or passed here in clear: only their HMAC (codes) or SHA-256 (tokens).

-- name: CreateLoginChallenge :one
-- Closes any open code for the address and issues a new one, atomically.
SELECT * FROM login_challenge_create(@email::text, @code_hash::bytea, @ttl_seconds::integer, @max_attempts::integer, sqlc.narg(requested_ip)::inet);

-- name: AttemptLoginChallenge :one
-- One guess at the open, unexpired, not-yet-exhausted code for an address. Every guess counts;
-- the right one also closes the challenge, so it works once. Returns no row when there is no
-- usable code; closed_at is set when the guess was right. Concurrent guesses are serialised by
-- the row lock, and the conditions are re-checked after waiting, so one code signs in once.
UPDATE login_challenges
SET attempts = attempts + 1,
    closed_at = CASE WHEN code_hash = @code_hash::bytea THEN now() END
WHERE email = @email::text
  AND closed_at IS NULL
  AND expires_at > now()
  AND attempts < max_attempts
RETURNING id, closed_at;

-- name: UpsertVerifiedUser :one
-- The account for an address that just proved it receives mail: created on first sign-in,
-- and marked verified either way.
INSERT INTO users (email, email_verified_at)
VALUES (@email::text, now())
ON CONFLICT ((lower(email))) DO UPDATE SET email_verified_at = COALESCE(users.email_verified_at, now())
RETURNING *;

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at, idle_expires_at, user_agent, ip)
VALUES (
    @user_id, @token_hash::bytea,
    now() + make_interval(secs => @absolute_seconds::integer),
    LEAST(now() + make_interval(secs => @idle_seconds::integer), now() + make_interval(secs => @absolute_seconds::integer)),
    @user_agent::text, sqlc.narg(ip)::inet
)
RETURNING *;

-- name: GetSessionByTokenHash :one
-- The live session for a cookie, with its user. Ended sessions and suspended users find nothing.
SELECT sqlc.embed(sessions), sqlc.embed(users)
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.token_hash = @token_hash::bytea
  AND sessions.idle_expires_at > now()
  AND users.status = 'active';

-- name: TouchSession :execrows
-- Slides the idle expiry, at most once per @min_interval_seconds, never past the absolute expiry.
UPDATE sessions
SET last_seen_at = now(),
    idle_expires_at = LEAST(now() + make_interval(secs => @idle_seconds::integer), expires_at)
WHERE id = @id
  AND last_seen_at < now() - make_interval(secs => @min_interval_seconds::integer);

-- name: ListUserSessions :many
SELECT * FROM sessions
WHERE user_id = @user_id AND idle_expires_at > now()
ORDER BY created_at DESC, id DESC
LIMIT 100;

-- name: DeleteUserSession :execrows
-- Revoke: scoped to the owner, so another user's session id finds nothing.
DELETE FROM sessions WHERE id = @id AND user_id = @user_id;

-- name: DeleteSessionByTokenHash :execrows
DELETE FROM sessions WHERE token_hash = @token_hash::bytea;

-- name: DeleteAllUserSessions :execrows
DELETE FROM sessions WHERE user_id = @user_id;

-- name: DeleteEndedSessions :execrows
DELETE FROM sessions WHERE idle_expires_at <= now();

-- name: DeleteOldLoginChallenges :execrows
-- Challenges are kept for @retention_days for abuse review (requested_ip), then removed.
DELETE FROM login_challenges WHERE created_at < now() - make_interval(days => @retention_days::integer);
