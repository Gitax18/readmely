"""Container health probe: `python -m app.probe`. Exit 0 when /healthz answers 200.

Uses the standard library only, so the image needs no curl.
"""

from __future__ import annotations

import os
import sys
import urllib.error
import urllib.request
from collections.abc import Mapping


def main(environ: Mapping[str, str] | None = None) -> int:
    env = os.environ if environ is None else environ
    port = env.get("HTTP_PORT", "8000").strip() or "8000"
    if not port.isdigit():
        print("probe: HTTP_PORT is not a number", file=sys.stderr)
        return 1
    url = f"http://127.0.0.1:{port}/healthz"
    try:
        with urllib.request.urlopen(url, timeout=2) as resp:
            return 0 if resp.status == 200 else 1
    except (urllib.error.URLError, OSError):
        print("probe: not reachable", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
