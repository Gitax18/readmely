from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator, Callable

import httpx
import pytest
from fastapi import FastAPI

from app.config import ServiceSettings
from app.main import create_app

TOKEN = "t" * 40


class FakeDB:
    """Stands in for app.db.Database. `mode` is "up", "down" or "hung"."""

    def __init__(self, mode: str = "up") -> None:
        self.mode = mode
        self.opened = False
        self.closed = False

    async def open(self) -> None:
        self.opened = True

    async def close(self) -> None:
        self.closed = True

    async def ping(self) -> None:
        if self.mode == "down":
            raise ConnectionError("connection to secret-host:5432 refused")
        if self.mode == "hung":
            await asyncio.Event().wait()  # never returns


def settings(**overrides: str) -> ServiceSettings:
    env = {"DATABASE_URL": "postgresql://u:p@localhost:5432/db", "AI_SERVICE_TOKEN": TOKEN, **overrides}
    return ServiceSettings.from_env(env)


@pytest.fixture
def make_app() -> Callable[..., FastAPI]:
    def _make(db: FakeDB | None = None) -> FastAPI:
        return create_app(settings(), db or FakeDB())

    return _make


@pytest.fixture
async def client(make_app: Callable[..., FastAPI]) -> AsyncIterator[httpx.AsyncClient]:
    async with _client(make_app()) as c:
        yield c


def _client(app: FastAPI) -> httpx.AsyncClient:
    transport = httpx.ASGITransport(app=app, raise_app_exceptions=False)
    return httpx.AsyncClient(transport=transport, base_url="http://ai")


def client_for(app: FastAPI) -> httpx.AsyncClient:
    return _client(app)


def auth(token: str = TOKEN) -> dict[str, str]:
    return {"Authorization": f"Bearer {token}"}
