"""The AI service must match contracts/ai.yaml: every response, every documented example, every route."""

from __future__ import annotations

from collections.abc import Callable
from pathlib import Path
from typing import Any

import httpx
import pytest
import yaml
from fastapi import FastAPI
from fastapi.routing import APIRoute
from openapi_core import OpenAPI
from openapi_core.testing import MockRequest, MockResponse
from pydantic import BaseModel

from app.contract.models import Error, Liveness, Readiness
from app.main import create_app
from app.security import error_body
from tests.conftest import TOKEN, FakeDB, auth, client_for, settings

SPEC_PATH = Path(__file__).resolve().parents[3] / "contracts" / "ai.yaml"
SERVER = "http://ai:8000"  # the spec's server URL
HTTP_METHODS = {"get", "put", "post", "delete", "options", "head", "patch", "trace"}


@pytest.fixture(scope="module")
def openapi() -> OpenAPI:
    return OpenAPI.from_file_path(str(SPEC_PATH))


@pytest.fixture(scope="module")
def spec() -> dict[str, Any]:
    loaded = yaml.safe_load(SPEC_PATH.read_text())
    assert isinstance(loaded, dict)
    return loaded


async def conforms(
    openapi: OpenAPI,
    app: FastAPI,
    method: str,
    path: str,
    status: int,
    *,
    documented_method: str = "GET",
    headers: dict[str, str] | None = None,
    body: bytes | None = None,
) -> httpx.Response:
    """Call the app and validate the response (status, headers, body) against the spec.

    A 405 is documented under the path's real operation, so the spec lookup uses
    `documented_method` while the request is sent with `method`.
    """
    async with client_for(app) as client:
        resp = await client.request(method, path, headers=headers, content=body)
    assert resp.status_code == status, resp.text
    request = MockRequest(SERVER, documented_method.lower(), path)
    response = MockResponse(
        resp.content,
        status_code=resp.status_code,
        headers=dict(resp.headers),
        content_type=resp.headers["content-type"].split(";")[0],
    )
    openapi.validate_response(request, response)
    return resp


def example(spec: dict[str, Any], path: str, code: str, name: str | None = None) -> Any:
    media = spec["paths"][path]["get"]["responses"][code]["content"]["application/json"]
    return media["example"] if name is None else media["examples"][name]["value"]


# --- responses --------------------------------------------------------------


async def test_healthz_matches_the_spec_and_its_example(
    openapi: OpenAPI, spec: dict[str, Any], make_app: Callable[..., FastAPI]
) -> None:
    resp = await conforms(openapi, make_app(), "GET", "/healthz", 200)
    assert resp.json() == example(spec, "/healthz", "200")


async def test_readyz_ready_matches_the_spec_and_its_example(
    openapi: OpenAPI, spec: dict[str, Any], make_app: Callable[..., FastAPI]
) -> None:
    resp = await conforms(openapi, make_app(), "GET", "/readyz", 200, headers=auth())
    assert resp.json() == example(spec, "/readyz", "200")


async def test_readyz_unavailable_matches_the_spec_and_its_example(
    openapi: OpenAPI, spec: dict[str, Any], make_app: Callable[..., FastAPI]
) -> None:
    resp = await conforms(openapi, make_app(FakeDB("down")), "GET", "/readyz", 503, headers=auth())
    assert resp.json() == example(spec, "/readyz", "503")


@pytest.mark.parametrize(
    "headers",
    [
        None,
        {"Authorization": "Bearer wrong"},
        {"Authorization": "Basic abc"},
        {"Authorization": f"Bearer {TOKEN} extra"},
    ],
    ids=["missing", "wrong-token", "wrong-scheme", "malformed"],
)
async def test_readyz_unauthorized_matches_the_spec(
    openapi: OpenAPI, make_app: Callable[..., FastAPI], headers: dict[str, str] | None
) -> None:
    resp = await conforms(openapi, make_app(), "GET", "/readyz", 401, headers=headers)
    assert resp.headers["www-authenticate"] == "Bearer"
    assert resp.json()["error"] == "unauthorized"


@pytest.mark.parametrize("path", ["/healthz", "/readyz"])
async def test_wrong_method_matches_the_spec(openapi: OpenAPI, make_app: Callable[..., FastAPI], path: str) -> None:
    resp = await conforms(openapi, make_app(), "POST", path, 405, headers=auth())
    assert resp.json()["error"] == "method_not_allowed"


async def test_oversized_body_matches_the_default_error_response(
    openapi: OpenAPI, make_app: Callable[..., FastAPI]
) -> None:
    resp = await conforms(openapi, make_app(), "POST", "/healthz", 413, body=b"x" * ((1 << 20) + 1))
    assert resp.json()["error"] == "payload_too_large"


async def test_unknown_route_matches_the_error_schema(make_app: Callable[..., FastAPI]) -> None:
    async with client_for(make_app()) as client:
        resp = await client.get("/nope")
    assert resp.status_code == 404
    assert Error.model_validate(resp.json()).error == "not_found"


async def test_a_crashing_handler_matches_the_error_schema() -> None:
    app = create_app(settings(), FakeDB())

    @app.get("/boom")
    async def boom() -> None:
        raise RuntimeError("secret detail")

    async with client_for(app) as client:
        resp = await client.get("/boom")
    assert resp.status_code == 500
    assert Error.model_validate(resp.json()).error == "internal_error"
    assert "secret detail" not in resp.text


def test_the_error_body_is_valid_even_without_a_request_id() -> None:
    # Error handling must never fail: an empty ID is replaced, not rejected.
    body = Error.model_validate(error_body(500, ""))
    assert body.error == "internal_error"
    assert len(body.request_id) >= 8


# --- examples and generated models ------------------------------------------


def test_every_spec_example_round_trips_through_the_generated_models(spec: dict[str, Any]) -> None:
    models: dict[str, type[BaseModel]] = {"/healthz": Liveness, "/readyz": Readiness}
    for path, model in models.items():
        for code, response in spec["paths"][path]["get"]["responses"].items():
            media = (response.get("content") or {}).get("application/json", {})
            if "example" in media and code in {"200", "503"}:
                assert model.model_validate(media["example"]).model_dump(mode="json") == media["example"], (path, code)


# --- route parity -----------------------------------------------------------


def registered(app: FastAPI) -> set[str]:
    """Every (method, path) the app serves, including routes added with include_router."""
    found: set[str] = set()

    def walk(routes: list[Any]) -> None:
        for route in routes:
            if isinstance(route, APIRoute):
                found.update(f"{method} {route.path}" for method in route.methods or set())
            elif (included := getattr(route, "original_router", None)) is not None:
                walk(list(included.routes))

    walk(list(app.routes))
    return found


def documented(spec: dict[str, Any]) -> set[str]:
    return {f"{m.upper()} {path}" for path, item in spec["paths"].items() for m in item if m in HTTP_METHODS}


def test_routes_match_the_spec(spec: dict[str, Any], make_app: Callable[..., FastAPI]) -> None:
    code, doc = registered(make_app()), documented(spec)
    assert code - doc == set(), "routes in code but not in the spec"
    assert doc - code == set(), "routes in the spec but not in code"


def test_the_parity_check_detects_drift(spec: dict[str, Any], make_app: Callable[..., FastAPI]) -> None:
    extra = make_app()

    @extra.get("/secret")
    async def secret() -> None: ...

    assert registered(extra) - documented(spec) == {"GET /secret"}

    bare = FastAPI()
    assert documented(spec) - registered(bare) == {"GET /healthz", "GET /readyz"}
