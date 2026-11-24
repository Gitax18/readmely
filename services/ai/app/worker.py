"""Background worker entry point: `python -m app.worker`.

Claims `summarise` jobs from the shared Postgres queue, runs them, and settles the credits held for
them: charged what the model calls cost when the job succeeds, refunded in full when it fails for good.
"""

from __future__ import annotations

import asyncio
import contextlib
import logging
import os
import signal
import socket
import sys
from collections.abc import Coroutine, Mapping
from typing import IO, Any

from app import logs, meter, queue
from app.config import ConfigError, WorkerSettings
from app.db import Database
from app.diagram import generate_diagram
from app.llm import LlmClient, LlmError
from app.readme import generate_readme, regenerate_section
from app.summarise import summarise

logger = logging.getLogger("app.worker")

HANDLERS = {
    "summarise": summarise,
    "generate_readme": generate_readme,
    "readme_section": regenerate_section,
    "generate_diagram": generate_diagram,
}
KINDS = list(HANDLERS)
LEASE_SECONDS = 60.0
POLL_SECONDS = 1.0
ERROR_SLEEP_SECONDS = 5.0
BACKOFF_SECONDS = 30.0
REQUEUE_EVERY_SECONDS = 30.0
# Codes that describe the project, not a fault: retrying cannot help, the person is told why and
# refunded, so they end the job at once and are logged as information, not errors.
EXPECTED_OUTCOMES = frozenset(
    {"analysis_missing", "stale_analysis", "readme_missing", "section_missing", "no_database_files"}
)


async def _sleep(stop: asyncio.Event, seconds: float) -> None:
    with contextlib.suppress(TimeoutError):
        await asyncio.wait_for(stop.wait(), seconds)


async def _until_stopped(stop: asyncio.Event, coro: Coroutine[Any, Any, Any]) -> Any:
    """Run a step, but give up on it when the worker is told to stop. Returns None if it was given up."""
    task = asyncio.ensure_future(coro)
    waiter = asyncio.ensure_future(stop.wait())
    try:
        await asyncio.wait({task, waiter}, return_when=asyncio.FIRST_COMPLETED)
    finally:
        waiter.cancel()
    if not task.done():
        task.cancel()
        with contextlib.suppress(asyncio.CancelledError, Exception):
            await task
        return None
    return task.result()


async def _refund(db: Database, job_id: Any) -> None:
    try:
        async with db.connection() as conn:
            await meter.refund(conn, job_id)
    except Exception:
        logger.exception("could not refund the hold", extra={"job_id": str(job_id)})


async def _heartbeat(db: Database, job_id: Any, worker: str, cancel: asyncio.Event) -> None:
    while not cancel.is_set():
        await _sleep(cancel, LEASE_SECONDS / 3)
        if cancel.is_set():
            return
        try:
            async with db.connection() as conn:
                if not await queue.extend_lease(conn, job_id, worker, LEASE_SECONDS):
                    logger.error("lost the job lease", extra={"job_id": str(job_id)})
                    return
        except Exception:
            logger.exception("could not renew the job lease", extra={"job_id": str(job_id)})


async def run_job(db: Database, llm: LlmClient, settings: WorkerSettings, job: dict[str, Any], worker: str) -> None:
    job_id = job["id"]
    log = {"job_id": str(job_id), "attempt": job["attempts"]}
    logger.info("job started", extra=log)
    stop_beat = asyncio.Event()
    beat = asyncio.create_task(_heartbeat(db, job_id, worker, stop_beat))

    async def report(step: str, progress: int) -> None:
        async with db.connection() as conn:
            await queue.set_progress(conn, job_id, worker, step, progress)

    code: str | None = None
    permanent = False
    try:
        await HANDLERS[job["kind"]](db, llm, settings, job, report)
    except LlmError as exc:
        code, permanent = exc.code, exc.permanent or exc.code in EXPECTED_OUTCOMES
    except Exception:
        logger.exception("job crashed", extra=log)
        code = "internal_error"
    finally:
        stop_beat.set()
        await beat

    async with db.connection() as conn:
        if code is None:
            await queue.complete(conn, job_id, worker)
            logger.info("job succeeded", extra=log)
            return
        if code in EXPECTED_OUTCOMES:
            # The project's state, not a fault: the person is told why and refunded. Logging it as an
            # error would bury the real failures that monitoring should catch.
            logger.info("job ended without a result", extra={**log, "code": code})
        else:
            logger.error("job failed", extra={**log, "code": code, "permanent": permanent})
        if permanent:
            await queue.fail(conn, job_id, worker, code)
            final = True
        else:
            final = await queue.release(conn, job_id, worker, code, BACKOFF_SECONDS * job["attempts"]) == "failed"
    if final:
        await _refund(db, job_id)


async def _requeue(db: Database) -> list[dict[str, Any]]:
    async with db.connection() as conn:
        return await queue.requeue_expired(conn, KINDS)


async def _claim(db: Database, worker: str) -> dict[str, Any] | None:
    async with db.connection() as conn:
        return await queue.claim(conn, worker, LEASE_SECONDS, KINDS)


async def work(db: Database, llm: LlmClient, settings: WorkerSettings, stop: asyncio.Event) -> None:
    worker = f"{socket.gethostname()}-{os.getpid()}"
    last_requeue = 0.0
    loop = asyncio.get_running_loop()
    while not stop.is_set():
        try:
            if loop.time() - last_requeue > REQUEUE_EVERY_SECONDS:
                last_requeue = loop.time()
                for dead in await _until_stopped(stop, _requeue(db)) or []:
                    await _refund(db, dead["id"])
            job = await _until_stopped(stop, _claim(db, worker))
            if job is None:
                await _sleep(stop, POLL_SECONDS)
                continue
            await run_job(db, llm, settings, job, worker)
        except Exception as exc:
            logger.error("queue unavailable", extra={"error": type(exc).__name__})
            await _sleep(stop, ERROR_SLEEP_SECONDS)


async def run(
    environ: Mapping[str, str], stop: asyncio.Event, stdout: IO[str] | None = None, stderr: IO[str] | None = None
) -> int:
    try:
        settings = WorkerSettings.from_env(environ)
    except ConfigError as exc:
        print(f"configuration error:\n{exc}", file=stderr or sys.stderr)
        return 2
    logs.configure(settings.log_level, "ai-worker", stdout)
    llm = LlmClient(settings)
    db = Database(settings.database_url.get_secret_value(), max_size=4)
    await db.open()
    logger.info(
        "worker started",
        extra={"env": settings.app_env, "llm_configured": llm.configured, "daily_cap_usd": settings.llm_daily_cap_usd},
    )
    try:
        await work(db, llm, settings, stop)
    finally:
        await llm.close()
        await db.close()
    logger.info("worker stopped")
    return 0


async def _main() -> int:
    stop = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGTERM, signal.SIGINT):
        loop.add_signal_handler(sig, stop.set)
    return await run(os.environ, stop)


if __name__ == "__main__":
    sys.exit(asyncio.run(_main()))
