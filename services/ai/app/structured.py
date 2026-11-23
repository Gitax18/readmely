"""One structured model call with everything around it: the daily cap, a retry on a bad answer, and
metering. Used by every job that writes with the model, so they all spend under the same rules."""

from __future__ import annotations

from collections.abc import Awaitable, Callable
from typing import Any
from uuid import UUID

from pydantic import ValidationError

from app import meter
from app.config import WorkerSettings
from app.db import Database
from app.llm import DEFAULT_OUTPUT_TOKENS, LlmClient, LlmError, LlmUsageError

Report = Callable[[str, int], Awaitable[None]]

ATTEMPTS = 2


def _call_of(  # noqa: PLR0913, PLR0917
    user_id: UUID, job_id: UUID, llm: LlmClient, purpose: str, usage: Any, status: str, code: str | None
) -> meter.Call:
    return meter.Call(
        user_id=user_id,
        job_id=job_id,
        model=usage.model if usage else llm.model,
        tier="fast",
        purpose=purpose,
        input_tokens=usage.input_tokens if usage else 0,
        output_tokens=usage.output_tokens if usage else 0,
        cost_micro_usd=usage.cost_micro_usd if usage else 0,
        status=status,
        error_code=code,
        duration_ms=usage.duration_ms if usage else 0,
    )


async def _record(db: Database, call: meter.Call) -> None:
    async with db.connection() as conn:
        await meter.record_call(conn, call)


async def structured_call[T](  # noqa: PLR0913
    db: Database,
    llm: LlmClient,
    settings: WorkerSettings,
    job: dict[str, Any],
    *,
    purpose: str,
    system: str,
    prompt: str,
    schema_name: str,
    schema: dict[str, Any],
    validate: Callable[[dict[str, Any]], T],
    report: Report,
    max_output_tokens: int = DEFAULT_OUTPUT_TOKENS,
) -> tuple[T, int]:
    """Ask for JSON, validate it, and try once more if the answer is unusable.

    Returns the validated result and the total cost in millionths of a dollar. Raises LlmError for
    anything the caller should record: the cap, a refusal, a provider error, or two bad answers.
    """
    user_id, job_id = job["user_id"], job["id"]
    cap_micro = int(settings.llm_daily_cap_usd * 1_000_000)
    total_cost = 0
    last_error = "llm_invalid_output"
    for attempt in range(ATTEMPTS):
        async with db.connection() as conn:
            spent = await meter.spent_today_micro_usd(conn)
        if spent + llm.worst_case_micro_usd(len(system) + len(prompt), max_output_tokens) > cap_micro:
            raise LlmError("spend_cap_reached")
        await report("writing", 40 + attempt * 20)
        usage = None
        result: T | None = None
        try:
            raw, usage = await llm.json_call(
                system=system, user=prompt, schema_name=schema_name, schema=schema, max_output_tokens=max_output_tokens
            )
            result = validate(raw)
            status, code = "ok", None
        except LlmUsageError as exc:
            usage, status, code, last_error = exc.usage, "error", exc.code, exc.code
        except (ValidationError, ValueError):
            status, code, last_error = "error", "llm_invalid_output", "llm_invalid_output"
        except LlmError as exc:
            await _record(db, _call_of(user_id, job_id, llm, purpose, None, "error", exc.code))
            if exc.code != "llm_invalid_output":
                raise
            status, code, last_error = "error", exc.code, exc.code
            await report("writing", 40 + attempt * 20)
            continue
        await _record(db, _call_of(user_id, job_id, llm, purpose, usage, status, code))
        total_cost += usage.cost_micro_usd if usage else 0
        if result is not None:
            return result, total_cost
        if last_error == "llm_refused":
            break
    raise LlmError(last_error)
