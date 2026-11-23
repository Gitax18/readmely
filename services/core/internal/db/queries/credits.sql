-- name: GetCreditBalance :one
SELECT COALESCE((SELECT balance FROM credit_balances WHERE user_id = @user_id::uuid), 0)::bigint AS balance;

-- name: GrantCredits :one
INSERT INTO credit_ledger (user_id, delta, kind, idempotency_key, note)
VALUES (@user_id::uuid, @amount, 'grant', @idempotency_key, @note)
RETURNING *;

-- name: ReserveCredits :one
-- Holds credits for a job. Fails with SQLSTATE CR001 when the balance is too small and CR002
-- when the key was used for something else; a replay returns the first entry and charges nothing.
SELECT * FROM credit_reserve(@user_id, @amount, sqlc.narg(job_id), @idempotency_key);

-- name: ListCreditEntries :many
SELECT * FROM credit_ledger
WHERE user_id = @user_id::uuid
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;

-- name: GrantWelcomeCredits :exec
-- The one-time grant for a new account. The fixed key makes a second call a no-op, so a retried
-- or raced sign-up never grants twice.
INSERT INTO credit_ledger (user_id, delta, kind, idempotency_key, note)
VALUES (@user_id::uuid, @amount, 'grant', 'welcome-grant', 'Welcome credits')
ON CONFLICT (user_id, idempotency_key) DO NOTHING;
