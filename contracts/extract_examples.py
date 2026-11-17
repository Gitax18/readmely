"""Writes the examples of core-internal.yaml to JSON so the web tests can feed them to the Zod schema.

Output: apps/web/src/contract/core-internal.examples.json, shaped
{"GET /api/v1/status 200": {"operational": {...}, ...}, ...}. Deterministic: sorted keys, fixed indent.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parent))
import check

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "apps" / "web" / "src" / "contract" / "core-internal.examples.json"


def extract(doc: check.Doc) -> dict[str, dict[str, Any]]:
    found: dict[str, dict[str, Any]] = {}
    for path, item in doc["paths"].items():
        for method, op in item.items():
            for code, response in op["responses"].items():
                for media in (response.get("content") or {}).values():
                    examples = {label.removeprefix("examples."): value for label, value in check.examples_of(media)}
                    if examples:
                        found[f"{method.upper()} {path} {code}"] = examples
    return found


def render(doc: check.Doc) -> str:
    return json.dumps(extract(doc), indent=2, sort_keys=True) + "\n"


def main() -> int:
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT.write_text(render(check.load(ROOT / "contracts" / "core-internal.yaml")), encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main())
