"""Background worker entry point: `python -m app.worker`.

For now it only starts, logs, and exits cleanly on SIGTERM or SIGINT.
Job processing arrives with the job queue in a later phase.
"""

from __future__ import annotations

import asyncio
import logging
import os
import signal
import sys
from collections.abc import Mapping
from typing import IO

from app import logs
from app.config import BaseSettings, ConfigError

logger = logging.getLogger("app.worker")


async def run(
    environ: Mapping[str, str], stop: asyncio.Event, stdout: IO[str] | None = None, stderr: IO[str] | None = None
) -> int:
    try:
        settings = BaseSettings.from_env(environ)
    except ConfigError as exc:
        print(f"configuration error:\n{exc}", file=stderr or sys.stderr)
        return 2
    logs.configure(settings.log_level, "ai-worker", stdout)
    logger.info("worker started", extra={"env": settings.app_env})
    await stop.wait()
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
