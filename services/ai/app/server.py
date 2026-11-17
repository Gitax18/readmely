"""Entry point: `python -m app.server`.

Exit codes: 0 clean stop, 1 runtime failure (for example the port is taken), 2 bad configuration.
"""

from __future__ import annotations

import logging
import os
import signal
import sys
from collections.abc import Mapping
from types import FrameType

import uvicorn
from fastapi import FastAPI

from app import logs
from app.config import ConfigError, ServiceSettings
from app.db import Database
from app.main import create_app

logger = logging.getLogger("app.server")


def build_server(app: FastAPI, settings: ServiceSettings) -> uvicorn.Server:
    config = uvicorn.Config(
        app,
        host=settings.http_host,
        port=settings.http_port,
        log_config=None,  # our JSON logging handles Uvicorn's loggers too
        access_log=False,  # RequestContextMiddleware writes the access log
        server_header=False,
        proxy_headers=False,  # nothing in front of this service is trusted to set them
        lifespan="on",
        timeout_graceful_shutdown=int(max(1, settings.shutdown_timeout)),
    )
    return uvicorn.Server(config)


def install_stop_handlers(server: uvicorn.Server) -> None:
    """Make SIGTERM and SIGINT mean "stop gracefully, exit 0".

    While serving, Uvicorn uses its own handlers. Before it starts, and after it
    finishes, these apply: a signal during startup asks the server to stop as
    soon as it can, and the copy of the signal Uvicorn re-raises after a graceful
    shutdown lands here instead of killing the process (exit -15 / 143).
    """

    def stop(_signum: int, _frame: FrameType | None) -> None:
        server.should_exit = True

    for sig in (signal.SIGTERM, signal.SIGINT):
        signal.signal(sig, stop)


def main(environ: Mapping[str, str] | None = None) -> int:
    try:
        settings = ServiceSettings.from_env(os.environ if environ is None else environ)
    except ConfigError as exc:
        print(f"configuration error:\n{exc}", file=sys.stderr)
        return 2

    logs.configure(settings.log_level, "ai")
    server = build_server(create_app(settings, Database(settings.database_url.get_secret_value())), settings)
    logger.info(
        "ai service starting", extra={"host": settings.http_host, "port": settings.http_port, "env": settings.app_env}
    )
    install_stop_handlers(server)
    try:
        server.run()  # replaces our handlers with its own graceful-shutdown handlers while serving
    except SystemExit as exc:  # Uvicorn exits this way when it cannot start (for example, the port is taken)
        return 0 if exc.code in (0, None) else 1
    if not server.started:
        return 1
    logger.info("ai service stopped")
    return 0


if __name__ == "__main__":
    sys.exit(main())
