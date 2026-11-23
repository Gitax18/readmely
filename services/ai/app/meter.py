"""What model calls cost, and the daily cap that stops them.

Every call is written to `llm_calls`, failures included. The cap is checked against that table before
each call, so it holds across every worker process and survives a restart.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any
from uuid import UUID

from psycopg import AsyncConnection

# One credit is half a US cent. Held credits are settled to what the calls really cost, at least one.
MICRO_USD_PER_CREDIT = 5_000


def credits_for(cost_micro_usd: int) -> int:
    return max(1, -(-cost_micro_usd // MICRO_USD_PER_CREDIT))


async def spent_today_micro_usd(conn: AsyncConnection[Any]) -> int:
    cur = await conn.execute(
        """
        SELECT COALESCE(sum(cost_micro_usd), 0)::bigint AS spent FROM llm_calls
        WHERE created_at >= ((now() AT TIME ZONE 'UTC')::date::timestamp AT TIME ZONE 'UTC')
        """
    )
    row = await cur.fetchone()
    return int(row["spent"]) if row else 0


@dataclass(frozen=True)
class Call:
    user_id: UUID
    job_id: UUID
    model: str
    tier: str
    purpose: str
    input_tokens: int
    output_tokens: int
    cost_micro_usd: int
    status: str
    error_code: str | None
    duration_ms: int


async def record_call(conn: AsyncConnection[Any], c: Call) -> None:
    await conn.execute(
        """
        INSERT INTO llm_calls (user_id, job_id, provider, model, tier, purpose, input_tokens, output_tokens,
                               cost_micro_usd, status, error_code, duration_ms)
        VALUES (%s, %s, 'openai', %s, %s, %s, %s, %s, %s, %s, %s, %s)
        """,
        (
            c.user_id,
            c.job_id,
            c.model,
            c.tier,
            c.purpose,
            c.input_tokens,
            c.output_tokens,
            c.cost_micro_usd,
            c.status,
            c.error_code,
            c.duration_ms,
        ),
    )


async def settle(conn: AsyncConnection[Any], job_id: UUID, cost_micro_usd: int) -> int:
    """Charge what the job cost and return the rest of its hold. Returns the credits charged."""
    charge = credits_for(cost_micro_usd)
    await conn.execute("SELECT credit_settle(%s, %s)", (job_id, charge))
    return charge


async def refund(conn: AsyncConnection[Any], job_id: UUID) -> None:
    await conn.execute("SELECT credit_refund(%s)", (job_id,))
