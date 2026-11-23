"""The job queue, from the worker's side.

The same statements as services/core/internal/db/queries/jobs.sql, which is the source of truth for
how a job moves (the queue is shared with the Go workers). Every statement that changes a claimed job
names the worker, so a worker that lost its job changes nothing and gets no row back.
"""

from __future__ import annotations

from typing import Any
from uuid import UUID

from psycopg import AsyncConnection

Row = dict[str, Any]


async def claim(conn: AsyncConnection[Any], worker: str, lease_seconds: float, kinds: list[str]) -> Row | None:
    cur = await conn.execute(
        """
        WITH next AS (
            SELECT j.id FROM jobs j
            WHERE j.status = 'queued' AND j.run_after <= now() AND j.kind = ANY(%(kinds)s)
            ORDER BY j.run_after, j.id
            FOR UPDATE SKIP LOCKED
            LIMIT 1
        )
        UPDATE jobs
        SET status = 'running', attempts = jobs.attempts + 1, locked_by = %(worker)s,
            lease_expires_at = now() + make_interval(secs => %(lease)s)
        FROM next WHERE jobs.id = next.id
        RETURNING jobs.*
        """,
        {"kinds": kinds, "worker": worker, "lease": lease_seconds},
    )
    return await cur.fetchone()


async def extend_lease(conn: AsyncConnection[Any], job_id: UUID, worker: str, lease_seconds: float) -> bool:
    cur = await conn.execute(
        """
        UPDATE jobs SET lease_expires_at = now() + make_interval(secs => %s)
        WHERE id = %s AND locked_by = %s AND status = 'running' AND lease_expires_at > now()
        """,
        (lease_seconds, job_id, worker),
    )
    return cur.rowcount == 1


async def set_progress(conn: AsyncConnection[Any], job_id: UUID, worker: str, step: str, progress: int) -> None:
    await conn.execute(
        "UPDATE jobs SET step = %s, progress = %s WHERE id = %s AND locked_by = %s AND status = 'running'",
        (step, max(0, min(progress, 100)), job_id, worker),
    )


async def complete(conn: AsyncConnection[Any], job_id: UUID, worker: str) -> bool:
    cur = await conn.execute(
        """
        UPDATE jobs SET status = 'succeeded', finished_at = now(), locked_by = NULL, lease_expires_at = NULL,
                        error_code = NULL
        WHERE id = %s AND locked_by = %s AND status = 'running'
        """,
        (job_id, worker),
    )
    return cur.rowcount == 1


async def release(
    conn: AsyncConnection[Any], job_id: UUID, worker: str, error_code: str, backoff_seconds: float
) -> str | None:
    """Queue the job again after a backoff, or fail it when its attempts are used up. Returns the new status."""
    cur = await conn.execute(
        """
        UPDATE jobs
        SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
            finished_at = CASE WHEN attempts >= max_attempts THEN now() END,
            run_after = CASE WHEN attempts >= max_attempts THEN run_after
                             ELSE now() + make_interval(secs => %s) END,
            error_code = %s, locked_by = NULL, lease_expires_at = NULL
        WHERE id = %s AND locked_by = %s AND status = 'running'
        RETURNING status
        """,
        (backoff_seconds, error_code, job_id, worker),
    )
    row = await cur.fetchone()
    return None if row is None else str(row["status"])


async def fail(conn: AsyncConnection[Any], job_id: UUID, worker: str, error_code: str) -> bool:
    """End the job for good, for an error that retrying cannot fix."""
    cur = await conn.execute(
        """
        UPDATE jobs SET status = 'failed', finished_at = now(), error_code = %s, locked_by = NULL,
                        lease_expires_at = NULL
        WHERE id = %s AND locked_by = %s AND status = 'running'
        """,
        (error_code, job_id, worker),
    )
    return cur.rowcount == 1


async def requeue_expired(conn: AsyncConnection[Any], kinds: list[str]) -> list[Row]:
    """Recover jobs whose worker stopped renewing the lease. Returns those that ran out of attempts."""
    cur = await conn.execute(
        """
        UPDATE jobs
        SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
            finished_at = CASE WHEN attempts >= max_attempts THEN now() END,
            error_code = 'lease_expired', locked_by = NULL, lease_expires_at = NULL
        WHERE status = 'running' AND lease_expires_at < now() AND kind = ANY(%s)
        RETURNING id, status
        """,
        (kinds,),
    )
    return [r for r in await cur.fetchall() if r["status"] == "failed"]
