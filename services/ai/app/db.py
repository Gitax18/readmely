"""Postgres access through a lazily connecting pool.

The pool keeps no idle connections and connects on demand, so the service
starts, and reports "postgres: down" through /readyz, even when Postgres
is unreachable.
"""

from __future__ import annotations

from contextlib import AbstractAsyncContextManager
from typing import Any

from psycopg import AsyncConnection
from psycopg.rows import dict_row
from psycopg_pool import AsyncConnectionPool

CONNECT_TIMEOUT_SECONDS = 2


class Database:
    def __init__(self, url: str, max_size: int = 5) -> None:
        self._pool = AsyncConnectionPool(
            url,
            min_size=0,
            max_size=max_size,
            open=False,
            kwargs={"connect_timeout": CONNECT_TIMEOUT_SECONDS, "row_factory": dict_row},
            name="ai",
        )

    async def open(self) -> None:
        await self._pool.open(wait=False)

    async def close(self) -> None:
        await self._pool.close()

    def connection(self) -> AbstractAsyncContextManager[AsyncConnection[Any]]:
        """A pooled connection whose rows are dicts. Commits on a clean exit, rolls back on an error."""
        return self._pool.connection()

    async def ping(self) -> None:
        async with self._pool.connection(timeout=CONNECT_TIMEOUT_SECONDS) as conn:
            await conn.execute("SELECT 1")
