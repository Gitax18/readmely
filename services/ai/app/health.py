"""Readiness checks. The response shape matches the Go service exactly."""

from __future__ import annotations

import asyncio
import logging
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Protocol

from app.contract.models import CheckMap, CheckState, Readiness

logger = logging.getLogger("app.health")


class Pinger(Protocol):
    async def ping(self) -> None: ...


@dataclass(frozen=True)
class Check:
    name: str
    pinger: Pinger


async def _probe(check: Check, limit_seconds: float, request_id: str) -> bool:
    # Each check gets its own deadline, so a hung dependency can never cause a
    # healthy one to be reported as down.
    try:
        await asyncio.wait_for(check.pinger.ping(), limit_seconds)
    except Exception as exc:  # any failure means "down"; the reason is logged, never returned
        logger.warning(
            "readiness check failed",
            extra={"request_id": request_id, "check": check.name, "error": f"{type(exc).__name__}: {exc}"},
        )
        return False
    return True


async def readiness(checks: Sequence[Check], limit_seconds: float, request_id: str) -> tuple[int, Readiness]:
    """Run every check concurrently. Returns (HTTP status, body); the body type comes from contracts/ai.yaml."""
    results = await asyncio.gather(*(_probe(c, limit_seconds, request_id) for c in checks))
    states = {c.name: CheckState(root="ok" if ok else "down") for c, ok in zip(checks, results, strict=True)}
    ready = all(results)
    body = Readiness(status="ready" if ready else "unavailable", checks=CheckMap(root=states))
    return (200 if ready else 503), body
