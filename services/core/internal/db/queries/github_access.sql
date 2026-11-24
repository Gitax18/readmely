-- A person's sealed GitHub token (plans/phase-9b-private-repos.md). The caller seals and opens it;
-- these queries only move bytes, always scoped to one user.

-- name: UpsertGitHubAccess :exec
INSERT INTO github_access (user_id, sealed, scope)
VALUES (@user_id, @sealed, @scope::text)
ON CONFLICT (user_id) DO UPDATE SET sealed = EXCLUDED.sealed, scope = EXCLUDED.scope, created_at = now();

-- name: GetGitHubAccess :one
SELECT * FROM github_access WHERE user_id = @user_id;

-- name: DeleteGitHubAccess :execrows
DELETE FROM github_access WHERE user_id = @user_id;
