"""Postgres access through a lazily connecting pool.

The pool keeps no idle connections and connects on demand, so the service
starts, and reports "postgres: down" through /readyz, even when Postgres
is unreachable.
"""

from __future__ import annotations

from psycopg_pool import AsyncConnectionPool

CONNECT_TIMEOUT_SECONDS = 2


class Database:
    def __init__(self, url: str, max_size: int = 5) -> None:
        self._pool = AsyncConnectionPool(
            url,
            min_size=0,
            max_size=max_size,
            open=False,
            kwargs={"connect_timeout": CONNECT_TIMEOUT_SECONDS},
            name="ai",
        )

    async def open(self) -> None:
        await self._pool.open(wait=False)

    async def close(self) -> None:
        await self._pool.close()

    async def ping(self) -> None:
        async with self._pool.connection(timeout=CONNECT_TIMEOUT_SECONDS) as conn:
            await conn.execute("SELECT 1")
