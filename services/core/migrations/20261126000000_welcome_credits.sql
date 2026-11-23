-- +goose Up

-- Accounts created before credits existed get the same one-time grant new accounts get. The
-- fixed key matches the one the API uses, so nothing is granted twice.
INSERT INTO credit_ledger (user_id, delta, kind, idempotency_key, note)
SELECT id, 100, 'grant', 'welcome-grant', 'Welcome credits' FROM users
ON CONFLICT (user_id, idempotency_key) DO NOTHING;

-- +goose Down
DELETE FROM credit_ledger WHERE kind = 'grant' AND idempotency_key = 'welcome-grant';
