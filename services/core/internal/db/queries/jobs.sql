-- The job queue. Lifecycle: EnqueueJob -> ClaimJob -> (ExtendJobLease)* -> CompleteJob | ReleaseJob.
-- A worker that disappears is handled by RequeueExpiredJobs, which any worker may call.
-- Every statement that changes a claimed job names the worker, so a worker that lost its job
-- (lease expired, job given to someone else) changes nothing and gets no rows back.

-- name: EnqueueJob :one
-- Replaying a request with the same idempotency key returns the original job and ignores the new
-- payload. The no-op update is what makes RETURNING work on conflict; it does not touch updated_at.
INSERT INTO jobs (user_id, project_id, kind, payload, idempotency_key, max_attempts, run_after)
VALUES (
    @user_id, sqlc.narg(project_id), @kind, @payload, @idempotency_key, @max_attempts,
    COALESCE(sqlc.narg(run_after)::timestamptz, now())
)
ON CONFLICT (user_id, idempotency_key) DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key
RETURNING *;

-- name: ClaimJob :one
-- Takes the oldest runnable job, optionally only of the given kinds. SKIP LOCKED lets any number
-- of workers claim at the same time without waiting for or duplicating each other.
WITH next AS (
    SELECT j.id FROM jobs j
    WHERE j.status = 'queued'
      AND j.run_after <= now()
      AND (sqlc.narg(kinds)::text[] IS NULL OR j.kind = ANY (sqlc.narg(kinds)::text[]))
    ORDER BY j.run_after, j.id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE jobs
SET status = 'running',
    attempts = jobs.attempts + 1,
    locked_by = @worker::text,
    lease_expires_at = now() + make_interval(secs => @lease_seconds::float8)
FROM next
WHERE jobs.id = next.id
RETURNING jobs.*;

-- name: ExtendJobLease :one
UPDATE jobs
SET lease_expires_at = now() + make_interval(secs => @lease_seconds::float8)
WHERE id = @id AND locked_by = @worker::text AND status = 'running' AND lease_expires_at > now()
RETURNING *;

-- name: CompleteJob :one
UPDATE jobs
SET status = 'succeeded', finished_at = now(), locked_by = NULL, lease_expires_at = NULL, error_code = NULL
WHERE id = @id AND locked_by = @worker::text AND status = 'running'
RETURNING *;

-- name: ReleaseJob :one
-- Gives a job back after a failure: queued again after the backoff, or failed for good when the
-- attempts are used up. error_code is a short machine code, never provider or stack text.
UPDATE jobs
SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
    finished_at = CASE WHEN attempts >= max_attempts THEN now() END,
    run_after = CASE WHEN attempts >= max_attempts THEN run_after ELSE now() + make_interval(secs => @backoff_seconds::float8) END,
    error_code = @error_code::text,
    locked_by = NULL,
    lease_expires_at = NULL
WHERE id = @id AND locked_by = @worker::text AND status = 'running'
RETURNING *;

-- name: RequeueExpiredJobs :many
-- Recovers jobs whose worker stopped renewing the lease. Same retry rule as ReleaseJob.
UPDATE jobs
SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
    finished_at = CASE WHEN attempts >= max_attempts THEN now() END,
    error_code = 'lease_expired',
    locked_by = NULL,
    lease_expires_at = NULL
WHERE status = 'running' AND lease_expires_at < now()
RETURNING *;

-- name: CancelJob :one
-- Only the owner, and only while the job has not started.
UPDATE jobs
SET status = 'cancelled', finished_at = now()
WHERE id = @id AND user_id = @user_id AND status = 'queued'
RETURNING *;

-- name: GetJob :one
SELECT * FROM jobs WHERE id = @id AND user_id = @user_id;

-- name: ListProjectJobs :many
SELECT * FROM jobs
WHERE project_id = @project_id AND user_id = @user_id
ORDER BY created_at DESC, id DESC
LIMIT @row_limit;
