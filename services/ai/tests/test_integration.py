"""Against the real local Postgres: `make up`, then `make test-integration`."""

from __future__ import annotations

import os
from urllib.parse import urlsplit, urlunsplit

import pytest

from app.contract.models import Readiness
from app.db import Database
from app.health import Check, readiness

pytestmark = pytest.mark.integration


def database_url() -> str:
    url = os.environ.get("DATABASE_URL", "")
    if not url:
        pytest.skip("DATABASE_URL is not set; run through `make test-integration`")
    return url


def with_password(url: str, password: str) -> str:
    parts = urlsplit(url)
    netloc = f"{parts.username}:{password}@{parts.hostname}:{parts.port}"
    return urlunsplit(parts._replace(netloc=netloc))


async def check(url: str) -> tuple[int, Readiness]:
    db = Database(url)
    await db.open()
    try:
        return await readiness([Check("postgres", db)], 3.0, "integration-test")
    finally:
        await db.close()


async def test_ready_against_real_postgres() -> None:
    status, body = await check(database_url())
    assert status == 200 and body.model_dump(mode="json")["checks"] == {"postgres": "ok"}


async def test_wrong_password_is_down() -> None:
    status, body = await check(with_password(database_url(), "definitely-wrong"))
    assert status == 503 and body.model_dump(mode="json")["checks"] == {"postgres": "down"}
