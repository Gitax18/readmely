from __future__ import annotations

import asyncio
import io
import os
import signal
import subprocess
import sys
from pathlib import Path

import pytest

from app.worker import run

# Nothing listens here: the worker starts anyway and keeps retrying, as it must when Postgres is down.
DB_URL = "postgresql://readmely_ai:x@127.0.0.1:9/readmely"


async def test_worker_runs_until_stopped() -> None:
    stop = asyncio.Event()
    out = io.StringIO()
    task = asyncio.create_task(run({"DATABASE_URL": DB_URL}, stop, stdout=out))
    await asyncio.sleep(0.05)
    assert not task.done()
    stop.set()
    assert await asyncio.wait_for(task, 2) == 0
    text = out.getvalue()
    assert "worker started" in text and "worker stopped" in text and '"service": "ai-worker"' in text


@pytest.mark.parametrize("sig", [signal.SIGTERM, signal.SIGINT])
def test_worker_process_exits_0_on_signal(sig: signal.Signals) -> None:
    proc = subprocess.Popen(
        [sys.executable, "-m", "app.worker"],
        cwd=Path(__file__).resolve().parents[1],
        env={
            "PATH": os.environ.get("PATH", ""),
            "DATABASE_URL": DB_URL,
            **{k: v for k, v in os.environ.items() if k.startswith("COVERAGE")},
        },
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    assert proc.stdout is not None
    line = proc.stdout.readline()  # blocks until the worker has logged that it started
    assert "worker started" in line
    proc.send_signal(sig)
    out, err = proc.communicate(timeout=10)
    assert proc.returncode == 0, err
    assert "worker stopped" in out


async def test_worker_refuses_bad_config() -> None:
    out, err = io.StringIO(), io.StringIO()
    code = await run({"DATABASE_URL": DB_URL, "LOG_LEVEL": "loud"}, asyncio.Event(), stdout=out, stderr=err)
    assert code == 2
    assert "LOG_LEVEL" in err.getvalue()
    assert out.getvalue() == ""
