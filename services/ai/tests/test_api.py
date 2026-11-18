from __future__ import annotations

import time
from collections.abc import AsyncIterator, Callable

import pytest
from fastapi import FastAPI
from pydantic import BaseModel

from tests.conftest import TOKEN, FakeDB, auth, client_for

SECURITY_HEADERS = {
    "x-content-type-options": "nosniff",
    "cache-control": "no-store",
    "referrer-policy": "no-referrer",
    "content-security-policy": "default-src 'none'; frame-ancestors 'none'",
}


def assert_error(resp: object, status: int, code: str) -> None:
    """Every error must have exactly the Go service's shape."""
    import httpx

    assert isinstance(resp, httpx.Response)
    assert resp.status_code == status
    body = resp.json()
    assert set(body) == {"error", "request_id"}
    assert body["error"] == code
    assert body["request_id"] and body["request_id"] == resp.headers["x-request-id"]
    for name, value in SECURITY_HEADERS.items():
        assert resp.headers.get(name) == value, name


# --- /healthz ---------------------------------------------------------------


async def test_healthz_needs_no_token_and_has_security_headers(client: object) -> None:
    import httpx

    assert isinstance(client, httpx.AsyncClient)
    resp = await client.get("/healthz")
    assert resp.status_code == 200
    assert resp.json() == {"status": "ok"}
    assert len(resp.headers["x-request-id"]) >= 8
    for name, value in SECURITY_HEADERS.items():
        assert resp.headers[name] == value
    assert "server" not in resp.headers


async def test_healthz_ignores_a_down_database(make_app: Callable[..., FastAPI]) -> None:
    async with client_for(make_app(FakeDB("down"))) as c:
        assert (await c.get("/healthz")).status_code == 200


# --- /readyz auth -----------------------------------------------------------


@pytest.mark.parametrize(
    "headers",
    [
        {},
        {"Authorization": "Bearer wrong-token-" + "x" * 30},
        {"Authorization": f"Basic {TOKEN}"},
        {"Authorization": TOKEN},
        {"Authorization": "Bearer"},
        {"Authorization": f"Bearer {TOKEN}x"},
        {"Authorization": f"Bearer {TOKEN[:-1]}"},
    ],
)
async def test_readyz_rejects_missing_or_wrong_tokens(client: object, headers: dict[str, str]) -> None:
    import httpx

    assert isinstance(client, httpx.AsyncClient)
    resp = await client.get("/readyz", headers=headers)
    assert_error(resp, 401, "unauthorized")
    assert resp.headers["www-authenticate"] == "Bearer"


async def test_readyz_scheme_is_case_insensitive(client: object) -> None:
    import httpx

    assert isinstance(client, httpx.AsyncClient)
    resp = await client.get("/readyz", headers={"Authorization": f"bearer {TOKEN}"})
    assert resp.status_code == 200


# --- /readyz checks -----------------------------------------------------------


async def test_readyz_ready(client: object) -> None:
    import httpx

    assert isinstance(client, httpx.AsyncClient)
    resp = await client.get("/readyz", headers=auth())
    assert resp.status_code == 200
    assert resp.json() == {"status": "ready", "checks": {"postgres": "ok"}}


async def test_readyz_database_down(make_app: Callable[..., FastAPI]) -> None:
    async with client_for(make_app(FakeDB("down"))) as c:
        resp = await c.get("/readyz", headers=auth())
    assert resp.status_code == 503
    assert resp.json() == {"status": "unavailable", "checks": {"postgres": "down"}}
    assert "secret-host" not in resp.text and "refused" not in resp.text


async def test_readyz_hung_database_times_out(
    make_app: Callable[..., FastAPI], monkeypatch: pytest.MonkeyPatch
) -> None:
    import app.main

    monkeypatch.setattr(app.main, "READINESS_TIMEOUT_SECONDS", 0.2)
    started = time.perf_counter()
    async with client_for(make_app(FakeDB("hung"))) as c:
        resp = await c.get("/readyz", headers=auth())
    assert time.perf_counter() - started < 1.5
    assert resp.status_code == 503
    assert resp.json()["checks"] == {"postgres": "down"}


# --- request IDs --------------------------------------------------------------


async def test_valid_client_request_id_is_kept(client: object) -> None:
    import httpx

    assert isinstance(client, httpx.AsyncClient)
    resp = await client.get("/healthz", headers={"X-Request-ID": "client-id-12345"})
    assert resp.headers["x-request-id"] == "client-id-12345"


@pytest.mark.parametrize(
    "bad",
    ["short", "x" * 65, "has space in it", "semi;colon-value", "<script>alert(1)</script>", "ünïcödé-value"],
)
async def test_unsafe_client_request_ids_are_replaced(client: object, bad: str) -> None:
    import httpx

    assert isinstance(client, httpx.AsyncClient)
    resp = await client.get("/healthz", headers={b"X-Request-ID": bad.encode("utf-8")})
    assert resp.headers["x-request-id"] != bad
    assert len(resp.headers["x-request-id"]) == 26


# --- errors -------------------------------------------------------------------


class Payload(BaseModel):
    name: str


@pytest.fixture
async def test_routes_client(make_app: Callable[..., FastAPI]) -> AsyncIterator[object]:
    """The app plus routes that exist only in tests, to exercise error paths."""
    app = make_app()

    @app.post("/echo")
    async def echo(payload: Payload) -> dict[str, str]:
        return {"name": payload.name}

    @app.get("/boom")
    async def boom() -> None:
        raise RuntimeError("db password is hunter2")

    async with client_for(app) as c:
        yield c


async def test_unknown_route_is_404(client: object) -> None:
    import httpx

    assert isinstance(client, httpx.AsyncClient)
    assert_error(await client.get("/nope"), 404, "not_found")


async def test_wrong_method_is_405(client: object) -> None:
    import httpx

    assert isinstance(client, httpx.AsyncClient)
    assert_error(await client.post("/healthz"), 405, "method_not_allowed")


@pytest.mark.parametrize("path", ["/docs", "/redoc", "/openapi.json"])
async def test_api_docs_are_disabled(client: object, path: str) -> None:
    import httpx

    assert isinstance(client, httpx.AsyncClient)
    assert_error(await client.get(path), 404, "not_found")


async def test_handler_exception_is_a_generic_500(test_routes_client: object, caplog: pytest.LogCaptureFixture) -> None:
    import httpx

    assert isinstance(test_routes_client, httpx.AsyncClient)
    resp = await test_routes_client.get("/boom")
    assert_error(resp, 500, "internal_error")
    assert "hunter2" not in resp.text and "Traceback" not in resp.text
    assert "hunter2" in caplog.text  # details belong in the log
    assert any(getattr(r, "request_id", None) == resp.headers["x-request-id"] for r in caplog.records)


async def test_validation_error_does_not_echo_input(test_routes_client: object) -> None:
    import httpx

    assert isinstance(test_routes_client, httpx.AsyncClient)
    resp = await test_routes_client.post("/echo", json={"name": {"secret": "hunter2"}})
    assert_error(resp, 422, "invalid_request")
    assert "hunter2" not in resp.text


async def test_body_within_limit_is_accepted(test_routes_client: object) -> None:
    import httpx

    assert isinstance(test_routes_client, httpx.AsyncClient)
    resp = await test_routes_client.post("/echo", json={"name": "ok"})
    assert resp.status_code == 200


async def test_oversized_body_rejected_by_content_length(test_routes_client: object) -> None:
    import httpx

    assert isinstance(test_routes_client, httpx.AsyncClient)
    resp = await test_routes_client.post(
        "/echo", content=b"x" * ((1 << 20) + 1), headers={"content-type": "application/json"}
    )
    assert_error(resp, 413, "payload_too_large")


async def test_oversized_streamed_body_without_content_length(test_routes_client: object) -> None:
    import httpx

    assert isinstance(test_routes_client, httpx.AsyncClient)

    async def chunks() -> AsyncIterator[bytes]:
        for _ in range(20):
            yield b"x" * 100_000  # 2 MB total, no Content-Length

    resp = await test_routes_client.post("/echo", content=chunks(), headers={"content-type": "application/json"})
    assert_error(resp, 413, "payload_too_large")


async def test_access_log_has_no_query_string(client: object, caplog: pytest.LogCaptureFixture) -> None:
    import httpx

    assert isinstance(client, httpx.AsyncClient)
    caplog.set_level("DEBUG")
    await client.get("/healthz?token=abc123")
    records = [r for r in caplog.records if r.name == "app.access"]
    assert records
    assert all("abc123" not in str(vars(r)) for r in records)
    assert getattr(records[-1], "path", None) == "/healthz"
    assert getattr(records[-1], "status", None) == 200


async def test_successful_liveness_probes_are_logged_at_debug(client: object, caplog: pytest.LogCaptureFixture) -> None:
    import httpx

    assert isinstance(client, httpx.AsyncClient)
    caplog.set_level("DEBUG")
    await client.get("/healthz")
    await client.post("/healthz")  # 405: a failing probe stays visible
    await client.get("/readyz")  # 401 without a token
    levels = {(r.path, r.status): r.levelname for r in caplog.records if r.name == "app.access"}  # type: ignore[attr-defined]
    assert levels == {("/healthz", 200): "DEBUG", ("/healthz", 405): "INFO", ("/readyz", 401): "INFO"}


async def test_lifespan_opens_and_closes_the_database(make_app: Callable[..., FastAPI]) -> None:
    db = FakeDB()
    app = make_app(db)
    async with app.router.lifespan_context(app):
        assert db.opened and not db.closed
    assert db.closed
