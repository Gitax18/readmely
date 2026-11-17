"""Process-level behaviour: real Uvicorn server, real sockets, real signals."""

from __future__ import annotations

import asyncio
import os
import signal
import socket
import subprocess
import sys
import time
from pathlib import Path

import httpx
import pytest

from app import probe, server
from app.main import create_app
from tests.conftest import TOKEN, FakeDB, settings

SERVICE_DIR = Path(__file__).resolve().parents[1]


def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return int(s.getsockname()[1])


def wait_until_up(port: int, timeout: float = 10.0) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            if httpx.get(f"http://127.0.0.1:{port}/healthz", timeout=0.5).status_code == 200:
                return
        except httpx.HTTPError:
            time.sleep(0.05)
    raise AssertionError("server did not start")


def spawn(env: dict[str, str]) -> subprocess.Popen[str]:
    return subprocess.Popen(
        [sys.executable, "-m", "app.server"],
        cwd=SERVICE_DIR,
        env={
            "PATH": os.environ.get("PATH", ""),
            # Lets coverage follow the subprocess; nothing else from the parent leaks in.
            **{k: v for k, v in os.environ.items() if k.startswith("COVERAGE")},
            **env,
        },
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )


def dead_db_env(port: int) -> dict[str, str]:
    return {
        "DATABASE_URL": "postgresql://u:secretpw@127.0.0.1:1/db",
        "AI_SERVICE_TOKEN": TOKEN,
        "HTTP_PORT": str(port),
        "SHUTDOWN_TIMEOUT": "5s",
    }


def test_bad_config_exits_2_and_names_the_variable() -> None:
    proc = spawn({"AI_SERVICE_TOKEN": "short"})
    _, err = proc.communicate(timeout=20)
    assert proc.returncode == 2
    assert "AI_SERVICE_TOKEN" in err and "DATABASE_URL" in err
    assert "short" not in err


def test_service_stays_up_with_database_down_and_stops_cleanly_on_sigterm() -> None:
    port = free_port()
    proc = spawn(dead_db_env(port))
    try:
        wait_until_up(port)
        resp = httpx.get(f"http://127.0.0.1:{port}/readyz", headers={"Authorization": f"Bearer {TOKEN}"}, timeout=5)
        assert resp.status_code == 503
        assert resp.json() == {"status": "unavailable", "checks": {"postgres": "down"}}
        assert "server" not in resp.headers
    finally:
        proc.send_signal(signal.SIGTERM)
        out, err = proc.communicate(timeout=20)
    assert proc.returncode == 0, err
    assert "ai service stopped" in out
    assert "secretpw" not in out + err


def test_port_in_use_fails_with_a_clear_message() -> None:
    with socket.socket() as busy:
        busy.bind(("127.0.0.1", 0))
        busy.listen()
        port = busy.getsockname()[1]
        proc = spawn(dead_db_env(port))
        out, err = proc.communicate(timeout=20)
    assert proc.returncode == 1
    assert "address already in use" in (out + err).lower()


def test_binds_to_localhost_by_default() -> None:
    port = free_port()
    proc = spawn(dead_db_env(port))
    try:
        wait_until_up(port)
        conns = subprocess.run(
            ["lsof", "-nP", f"-iTCP:{port}", "-sTCP:LISTEN"], capture_output=True, text=True, check=False
        ).stdout
        assert "127.0.0.1" in conns and "*:" not in conns
    finally:
        proc.send_signal(signal.SIGTERM)
        proc.communicate(timeout=20)


async def test_sigterm_lets_an_in_flight_request_finish() -> None:
    """Uses build_server (the same config main() uses) with a slow test route."""
    port = free_port()
    s = settings(HTTP_PORT=str(port), SHUTDOWN_TIMEOUT="5s")
    app = create_app(s, FakeDB())

    @app.get("/slow")
    async def slow() -> dict[str, str]:
        await asyncio.sleep(0.5)
        return {"done": "yes"}

    srv = server.build_server(app, s)
    serving = asyncio.create_task(srv.serve())
    while not srv.started:  # noqa: ASYNC110 - Uvicorn only exposes a flag
        await asyncio.sleep(0.01)

    async with httpx.AsyncClient() as c:
        request = asyncio.create_task(c.get(f"http://127.0.0.1:{port}/slow"))
        await asyncio.sleep(0.1)
        srv.should_exit = True  # what Uvicorn's SIGTERM handler sets
        resp = await request

    assert resp.status_code == 200 and resp.json() == {"done": "yes"}
    await asyncio.wait_for(serving, 10)


def test_probe_against_a_running_service() -> None:
    port = free_port()
    proc = spawn(dead_db_env(port))
    try:
        wait_until_up(port)
        assert probe.main({"HTTP_PORT": str(port)}) == 0
    finally:
        proc.send_signal(signal.SIGTERM)
        proc.communicate(timeout=20)


@pytest.mark.parametrize("env", [{"HTTP_PORT": "notaport"}, {"HTTP_PORT": str(free_port())}])
def test_probe_fails_when_unreachable_or_misconfigured(env: dict[str, str]) -> None:
    assert probe.main(env) == 1


def test_main_returns_2_on_bad_config(capsys: pytest.CaptureFixture[str]) -> None:
    assert server.main({}) == 2
    assert "DATABASE_URL" in capsys.readouterr().err
