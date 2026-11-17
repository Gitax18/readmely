from __future__ import annotations

import importlib.util
import subprocess
import sys
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parents[1] / "check-commit-msg.py"
spec = importlib.util.spec_from_file_location("check_commit_msg", SCRIPT)
assert spec and spec.loader
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


@pytest.mark.parametrize(
    "message",
    [
        "feat(core): add TOTP enrolment endpoint",
        "fix(db): make credit debit atomic",
        "chore(infra): bump postgres image",
        "ci: add the CI workflow",
        "docs(plans): mark Step 5 done\n\nbody text\n\nCo-Authored-By: Someone <a@b.c>",
        "Merge branch 'feat/x' into main",
        "feat(web): merge Next.js status shell\n# a comment git adds",
        "feat(core): " + "x" * (72 - len("feat(core): ")),
    ],
)
def test_accepts(message: str) -> None:
    assert mod.check(message) == []


@pytest.mark.parametrize(
    ("message", "fragment"),
    [
        ("added a thing", "must look like"),
        ("feature(core): add thing", "unknown type"),
        ("Feat(core): add thing", "must look like"),
        ("feat(frontend): add thing", "unknown scope"),
        ("feat(): add thing", "unknown scope"),
        ("feat(core): add thing.", "period"),
        ("feat(core):add thing", "must look like"),
        ("feat(core):  padded", "whitespace"),
        ("feat(core): ", "summary is empty"),
        ("feat(core): " + "x" * 61, "characters"),
        ("", "empty"),
        ("# only a comment", "empty"),
        ("feat(core): add thing\nbody without blank line", "blank line"),
    ],
)
def test_rejects(message: str, fragment: str) -> None:
    problems = mod.check(message)
    assert problems and any(fragment in p for p in problems), problems


def test_cli_file(tmp_path: Path) -> None:
    good = tmp_path / "good"
    good.write_text("fix(ai): handle empty request id\n")
    bad = tmp_path / "bad"
    bad.write_text("whatever\n")
    run = lambda p: subprocess.run([sys.executable, str(SCRIPT), str(p)], capture_output=True, text=True)  # noqa: E731
    assert run(good).returncode == 0
    result = run(bad)
    assert result.returncode == 1 and "rejected" in result.stderr


def test_cli_range(tmp_path: Path) -> None:
    def git(*args: str) -> str:
        return subprocess.run(
            ["git", "-c", "user.name=t", "-c", "user.email=t@example.invalid", *args],
            cwd=tmp_path,
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()

    git("init", "-q", "-b", "main")
    git("commit", "-q", "--allow-empty", "-m", "chore: root")
    base = git("rev-parse", "HEAD")
    git("commit", "-q", "--allow-empty", "-m", "feat(core): fine")
    git("commit", "-q", "--allow-empty", "-m", "oops not conventional")
    run = lambda spec_: subprocess.run(  # noqa: E731
        [sys.executable, str(SCRIPT), "--range", spec_], cwd=tmp_path, capture_output=True, text=True
    )
    assert run(f"{base}..HEAD").returncode == 1
    assert run(f"{base}..HEAD~1").returncode == 0


def test_usage_error() -> None:
    result = subprocess.run([sys.executable, str(SCRIPT)], capture_output=True, text=True)
    assert result.returncode == 2
