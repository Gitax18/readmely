"""FastAPI application factory."""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Protocol

from fastapi import APIRouter, Depends, FastAPI, Request
from fastapi.responses import JSONResponse

from app.config import ServiceSettings
from app.contract.models import Liveness
from app.health import Check, readiness
from app.security import RequestContextMiddleware, install_error_handlers, request_id_from, service_token_dependency

READINESS_TIMEOUT_SECONDS = 2.0


class DatabaseLike(Protocol):
    async def open(self) -> None: ...
    async def close(self) -> None: ...
    async def ping(self) -> None: ...


def create_app(settings: ServiceSettings, db: DatabaseLike) -> FastAPI:
    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncIterator[None]:
        await db.open()
        try:
            yield
        finally:
            await db.close()

    # Internal service: no interactive docs and no published schema.
    app = FastAPI(lifespan=lifespan, docs_url=None, redoc_url=None, openapi_url=None)
    install_error_handlers(app)
    app.add_middleware(RequestContextMiddleware)

    checks = [Check("postgres", db)]

    @app.get("/healthz")
    async def healthz() -> JSONResponse:
        """Liveness: never touches a dependency, needs no token (used by the container probe)."""
        return JSONResponse(Liveness(status="ok").model_dump(mode="json"))

    # Every route on this router requires the service token.
    internal = APIRouter(dependencies=[Depends(service_token_dependency(settings.ai_service_token.get_secret_value()))])

    @internal.get("/readyz")
    async def readyz(request: Request) -> JSONResponse:
        status, body = await readiness(checks, READINESS_TIMEOUT_SECONDS, request_id_from(request.scope))
        return JSONResponse(body.model_dump(mode="json"), status_code=status)

    app.include_router(internal)
    return app
