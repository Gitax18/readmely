"""Checks the OpenAPI contracts in this directory.

1. Each spec is valid OpenAPI 3.0.
2. The schemas and headers shared by all specs are identical in every file
   (the files are self-contained, so copies must not drift apart).
3. Every example in a spec matches the schema of the response it sits in.

Exit code 0 when everything passes, 1 otherwise. Messages name the spec and the
location of the problem.
"""

from __future__ import annotations

import sys
from pathlib import Path
from typing import Any

import yaml
from openapi_schema_validator import OAS30Validator
from openapi_spec_validator import validate as validate_spec

SPECS = ("core-public", "core-internal", "ai")
SHARED_SCHEMAS = ("Error", "Liveness", "CheckState", "CheckMap", "Readiness")
SHARED_HEADERS = ("RequestId",)

Doc = dict[str, Any]


def load(path: Path) -> Doc:
    with path.open(encoding="utf-8") as f:
        data = yaml.safe_load(f)
    if not isinstance(data, dict):
        msg = f"{path.name}: not a YAML mapping"
        raise TypeError(msg)
    return data


def deref(node: Any, doc: Doc) -> Any:
    """Return node with every local $ref replaced by what it points to."""
    if isinstance(node, dict):
        ref = node.get("$ref")
        if isinstance(ref, str):
            if not ref.startswith("#/"):
                msg = f"only local references are supported: {ref}"
                raise ValueError(msg)
            target: Any = doc
            for part in ref[2:].split("/"):
                target = target[part]
            return deref(target, doc)
        return {k: deref(v, doc) for k, v in node.items()}
    if isinstance(node, list):
        return [deref(v, doc) for v in node]
    return node


def check_valid(name: str, doc: Doc) -> list[str]:
    try:
        validate_spec(doc)
    except Exception as exc:  # the validator raises several error types
        return [f"{name}: not valid OpenAPI: {str(exc).splitlines()[0][:200]}"]
    return []


def check_shared(docs: dict[str, Doc]) -> list[str]:
    problems: list[str] = []
    first = next(iter(docs))
    for section, names in (("schemas", SHARED_SCHEMAS), ("headers", SHARED_HEADERS)):
        for item in names:
            reference = docs[first].get("components", {}).get(section, {}).get(item)
            for name, doc in docs.items():
                found = doc.get("components", {}).get(section, {}).get(item)
                if found is None:
                    problems.append(f"{name}: shared {section[:-1]} {item} is missing")
                elif found != reference:
                    problems.append(f"{name}: shared {section[:-1]} {item} differs from {first}")
    return problems


def _examples(media: Doc) -> list[tuple[str, Any]]:
    found: list[tuple[str, Any]] = []
    if "example" in media:
        found.append(("example", media["example"]))
    for key, ex in (media.get("examples") or {}).items():
        found.append((f"examples.{key}", ex.get("value")))
    return found


def check_examples(name: str, doc: Doc) -> list[str]:
    problems: list[str] = []
    for path, item in doc.get("paths", {}).items():
        for method, op in item.items():
            for code, response in (op.get("responses") or {}).items():
                for ctype, media in (response.get("content") or {}).items():
                    schema = deref(media.get("schema", {}), doc)
                    for label, value in _examples(media):
                        errors = sorted(OAS30Validator(schema).iter_errors(value), key=lambda e: list(e.path))
                        problems.extend(
                            f"{name}: {method.upper()} {path} {code} {ctype} {label}: {e.message[:160]}" for e in errors
                        )
    return problems


def run(directory: Path) -> list[str]:
    docs: dict[str, Doc] = {}
    problems: list[str] = []
    for name in SPECS:
        path = directory / f"{name}.yaml"
        if not path.is_file():
            problems.append(f"{name}: {path.name} not found")
            continue
        docs[name] = load(path)
    for name, doc in docs.items():
        spec_problems = check_valid(name, doc)
        problems.extend(spec_problems)
        if not spec_problems:
            problems.extend(check_examples(name, doc))
    if len(docs) == len(SPECS):
        problems.extend(check_shared(docs))
    return problems


def main() -> int:
    problems = run(Path(__file__).resolve().parent)
    for p in problems:
        print(f"FAIL {p}")
    if not problems:
        print("ok   contracts are valid, consistent and their examples match their schemas")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
