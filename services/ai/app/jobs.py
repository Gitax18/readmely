"""The `jobs` table as Python sees it.

The queue is shared with the Go workers (decision D2). Go owns the schema: the migrations in
services/core/migrations/ are the source of truth, and this model is checked against the
migrated database by tests/test_schema_drift.py, so a column added, removed, retyped or made
nullable on one side fails a test instead of failing in production.

The Literal types below mirror the CHECK constraints jobs_kind_check and jobs_status_check;
the same test compares them.
"""

from __future__ import annotations

from datetime import datetime
from typing import Any, Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict

JobKind = Literal["ingest", "generate_readme", "generate_diagram", "build_context_pack"]
JobStatus = Literal["queued", "running", "succeeded", "failed", "cancelled"]


class JobRow(BaseModel):
    """One row of `jobs`, exactly as the database returns it."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    id: UUID
    user_id: UUID
    project_id: UUID | None
    kind: JobKind
    payload: dict[str, Any]
    status: JobStatus
    attempts: int
    max_attempts: int
    run_after: datetime
    locked_by: str | None
    lease_expires_at: datetime | None
    idempotency_key: str
    error_code: str | None
    step: str | None
    progress: int
    created_at: datetime
    updated_at: datetime
    finished_at: datetime | None
