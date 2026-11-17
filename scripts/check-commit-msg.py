#!/usr/bin/env python3
"""Check commit messages against the convention in README.md.

    check-commit-msg.py FILE          check the message in FILE (the commit-msg hook)
    check-commit-msg.py --range A..B  check every commit in a git range (CI)

Format: <type>(<scope>): <summary>   (the scope is optional)
"""

from __future__ import annotations

import re
import subprocess
import sys

TYPES = ("feat", "fix", "docs", "test", "refactor", "perf", "security", "build", "ci", "chore")
SCOPES = ("web", "core", "ai", "layout", "db", "contracts", "infra", "plans")
MAX_FIRST_LINE = 72

FIRST_LINE = re.compile(r"^(?P<type>[a-z]+)(?:\((?P<scope>[^()\s]*)\))?: (?P<summary>.*)$")


def check(message: str) -> list[str]:
    """Return the problems with a commit message; an empty list means it is fine."""
    lines = [line for line in message.splitlines() if not line.startswith("#")]
    first = lines[0] if lines else ""

    if first.startswith("Merge "):
        return []  # merge commits are written by git and by the merge command
    if not first.strip():
        return ["the first line is empty"]

    problems: list[str] = []
    if len(first) > MAX_FIRST_LINE:
        problems.append(f"the first line is {len(first)} characters; the limit is {MAX_FIRST_LINE}")

    match = FIRST_LINE.match(first)
    if match is None:
        problems.append("the first line must look like '<type>(<scope>): <summary>'")
        return problems

    if match["type"] not in TYPES:
        problems.append(f"unknown type '{match['type']}'; use one of: {', '.join(TYPES)}")
    if match["scope"] is not None and match["scope"] not in SCOPES:
        problems.append(f"unknown scope '{match['scope']}'; use one of: {', '.join(SCOPES)}")
    summary = match["summary"]
    if not summary.strip():
        problems.append("the summary is empty")
    elif summary != summary.strip():
        problems.append("the summary has extra whitespace at the edges")
    elif summary.endswith("."):
        problems.append("the summary must not end with a period")
    if len(lines) > 1 and lines[1].strip():
        problems.append("leave a blank line between the first line and the body")
    return problems


def report(label: str, problems: list[str]) -> None:
    print(f"commit message rejected ({label}):", file=sys.stderr)
    for problem in problems:
        print(f"  - {problem}", file=sys.stderr)
    print("  see 'Git conventions' in README.md", file=sys.stderr)


def check_range(spec: str) -> int:
    if spec.startswith("-"):
        raise SystemExit("the range must not start with '-'")
    log = subprocess.run(  # noqa: S603  fixed argument list, no shell, range cannot be an option
        ["git", "log", "--format=%H%x00%B%x01", spec],  # noqa: S607
        check=True,
        capture_output=True,
        text=True,
    ).stdout
    failed = 0
    for entry in log.split("\x01"):
        if not entry.strip():
            continue  # the newline after the last separator
        sha, _, message = entry.strip("\n").partition("\x00")
        if problems := check(message):
            report(sha[:10], problems)
            failed += 1
    return 1 if failed else 0


def main(argv: list[str]) -> int:
    if len(argv) == 3 and argv[1] == "--range":
        return check_range(argv[2])
    if len(argv) == 2 and not argv[1].startswith("-"):
        with open(argv[1], encoding="utf-8") as handle:
            problems = check(handle.read())
        if problems:
            report("your message", problems)
            return 1
        return 0
    print(__doc__, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
