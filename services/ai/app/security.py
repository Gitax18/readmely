"""Request-level protections: request IDs, body limit, access log, auth, and generic errors.

The error body and request ID rules match the Go service exactly:
`{"error": "<code>", "request_id": "<id>"}` and `^[A-Za-z0-9._-]{8,64}$`.
"""

from __future__ import annotations

import hmac
import logging
import re
import secrets
import time
from collections.abc import Awaitable, Callable, MutableMapping
from typing import Any

from fastapi import FastAPI, HTTPException, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

Scope = MutableMapping[str, Any]
Message = MutableMapping[str, Any]
Receive = Callable[[], Awaitable[Message]]
Send = Callable[[Message], Awaitable[None]]
ASGIApp = Callable[[Scope, Receive, Send], Awaitable[None]]

REQUEST_ID_HEADER = "x-request-id"
MAX_BODY_BYTES = 1 << 20  # 1 MiB

_VALID_REQUEST_ID = re.compile(r"^[A-Za-z0-9._-]{8,64}$")
_ERROR_CODES = {
    401: "unauthorized",
    404: "not_found",
    405: "method_not_allowed",
    413: "payload_too_large",
    422: "invalid_request",
    500: "internal_error",
}
_SECURITY_HEADERS = [
    (b"x-content-type-options", b"nosniff"),
    (b"cache-control", b"no-store"),
    (b"referrer-policy", b"no-referrer"),
    (b"content-security-policy", b"default-src 'none'; frame-ancestors 'none'"),
]

access_logger = logging.getLogger("app.access")
error_logger = logging.getLogger("app.error")


def new_request_id() -> str:
    return secrets.token_hex(13)  # 26 characters, same length as the Go IDs


def request_id_from(scope: Scope) -> str:
    """Return the request ID stored by RequestContextMiddleware ("" outside a request)."""
    state = scope.get("state") or {}
    return str(state.get("request_id", ""))


class BodyTooLargeError(Exception):
    pass


class RequestContextMiddleware:
    """Pure ASGI middleware, so the body limit also applies to streamed bodies.

    For each HTTP request it assigns a request ID, enforces the body limit,
    adds security headers and the request ID to the response, and writes one
    access-log line. The query string is never logged, because it can carry tokens.
    """

    def __init__(self, app: ASGIApp, max_body_bytes: int = MAX_BODY_BYTES) -> None:
        self.app = app
        self.max_body_bytes = max_body_bytes

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        headers = dict(scope.get("headers") or [])
        client_id = headers.get(REQUEST_ID_HEADER.encode(), b"").decode("latin-1")
        request_id = client_id if _VALID_REQUEST_ID.match(client_id) else new_request_id()
        scope.setdefault("state", {})["request_id"] = request_id

        status = 500
        started = time.perf_counter()

        async def send_with_headers(message: Message) -> None:
            nonlocal status
            if message["type"] == "http.response.start":
                status = message["status"]
                existing = list(message.get("headers", []))
                present = {name.lower() for name, _ in existing}
                additions = [(REQUEST_ID_HEADER.encode(), request_id.encode()), *_SECURITY_HEADERS]
                message["headers"] = existing + [(n, v) for n, v in additions if n not in present]
            await send(message)

        try:
            declared = int(headers.get(b"content-length", b"0") or b"0")
        except ValueError:
            declared = 0

        try:
            if declared > self.max_body_bytes:
                await _send_error(send_with_headers, 413, request_id)
                return

            received = 0

            async def limited_receive() -> Message:
                nonlocal received
                message = await receive()
                if message["type"] == "http.request":
                    received += len(message.get("body", b""))
                    if received > self.max_body_bytes:
                        raise BodyTooLargeError
                return message

            try:
                await self.app(scope, limited_receive, send_with_headers)
            except BodyTooLargeError:
                await _send_error(send_with_headers, 413, request_id)
        finally:
            # In `finally` so a crashed request is still logged (as 500).
            access_logger.info(
                "request",
                extra={
                    "request_id": request_id,
                    "method": scope.get("method", ""),
                    "path": scope.get("path", ""),
                    "status": status,
                    "duration_ms": round((time.perf_counter() - started) * 1000, 2),
                },
            )


async def _send_error(send: Send, status: int, request_id: str) -> None:
    body = JSONResponse(
        {"error": _ERROR_CODES.get(status, "http_error"), "request_id": request_id},
        status_code=status,
    ).body
    await send(
        {
            "type": "http.response.start",
            "status": status,
            "headers": [(b"content-type", b"application/json"), (b"content-length", str(len(body)).encode())],
        }
    )
    await send({"type": "http.response.body", "body": body})


def error_response(request: Request, status: int, headers: dict[str, str] | None = None) -> JSONResponse:
    # A 500 is sent by Starlette's outermost error middleware, outside
    # RequestContextMiddleware, so the request ID and security headers are set here too.
    request_id = request_id_from(request.scope)
    return JSONResponse(
        {"error": _ERROR_CODES.get(status, "http_error"), "request_id": request_id},
        status_code=status,
        headers={
            REQUEST_ID_HEADER: request_id,
            **{k.decode(): v.decode() for k, v in _SECURITY_HEADERS},
            **(headers or {}),
        },
    )


def install_error_handlers(app: FastAPI) -> None:
    """Every error leaves as the generic body. Details go to the log only."""

    async def http_error(request: Request, exc: Exception) -> JSONResponse:
        if not isinstance(exc, StarletteHTTPException):  # registered for this type only
            return error_response(request, 500)
        if exc.status_code == 413 or isinstance(exc.__cause__, BodyTooLargeError):
            return error_response(request, 413)
        return error_response(request, exc.status_code, getattr(exc, "headers", None))

    async def validation_error(request: Request, exc: Exception) -> JSONResponse:
        # The validation details echo the client's input, so they are not returned or logged.
        return error_response(request, 422)

    async def unhandled(request: Request, exc: Exception) -> JSONResponse:
        if isinstance(exc, BodyTooLargeError):
            return error_response(request, 413)
        error_logger.error(
            "unhandled exception",
            exc_info=exc,
            extra={"request_id": request_id_from(request.scope)},
        )
        return error_response(request, 500)

    app.add_exception_handler(StarletteHTTPException, http_error)
    app.add_exception_handler(RequestValidationError, validation_error)
    app.add_exception_handler(BodyTooLargeError, unhandled)
    app.add_exception_handler(Exception, unhandled)


def service_token_dependency(token: str) -> Callable[[Request], Awaitable[None]]:
    """FastAPI dependency that requires `Authorization: Bearer <token>`.

    The comparison is constant-time, and an absent, malformed or wrong header
    all produce the same 401, so a caller learns nothing about which part failed.
    """
    expected = token.encode()

    async def require_service_token(request: Request) -> None:
        scheme, _, presented = request.headers.get("authorization", "").partition(" ")
        valid = scheme.lower() == "bearer" and hmac.compare_digest(presented.strip().encode(), expected)
        if not valid:
            raise HTTPException(status_code=401, headers={"WWW-Authenticate": "Bearer"})

    return require_service_token
