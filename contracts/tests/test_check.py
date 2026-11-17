"""The contract checker must fail on each kind of fault, not just pass on good files."""

from __future__ import annotations

import shutil
import sys
from pathlib import Path

import pytest
import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import check

CONTRACTS = Path(__file__).resolve().parents[1]


@pytest.fixture
def specs(tmp_path: Path) -> Path:
    for name in check.SPECS:
        shutil.copy(CONTRACTS / f"{name}.yaml", tmp_path / f"{name}.yaml")
    return tmp_path


def edit(directory: Path, name: str, fn) -> None:  # type: ignore[no-untyped-def]
    path = directory / f"{name}.yaml"
    doc = yaml.safe_load(path.read_text())
    fn(doc)
    path.write_text(yaml.safe_dump(doc, sort_keys=False))


def test_the_real_contracts_pass() -> None:
    assert check.run(CONTRACTS) == []


def test_a_missing_spec_is_reported(specs: Path) -> None:
    (specs / "ai.yaml").unlink()
    assert any("ai.yaml not found" in p for p in check.run(specs))


def test_an_invalid_spec_is_rejected(specs: Path) -> None:
    edit(specs, "ai", lambda d: d["components"]["schemas"]["Liveness"].update(type="strng"))
    problems = check.run(specs)
    assert any(p.startswith("ai: not valid OpenAPI") for p in problems)


def test_a_missing_required_section_is_rejected(specs: Path) -> None:
    edit(specs, "core-public", lambda d: d.pop("paths"))
    assert any(p.startswith("core-public: not valid OpenAPI") for p in check.run(specs))


def test_a_changed_shared_schema_is_reported_in_the_file_that_differs(specs: Path) -> None:
    def change(d: dict) -> None:  # type: ignore[type-arg]
        d["components"]["schemas"]["Error"]["properties"]["error"]["pattern"] = "x"

    edit(specs, "core-internal", change)
    problems = check.run(specs)
    assert "core-internal: shared schema Error differs from core-public" in problems


def test_a_missing_shared_schema_is_reported(specs: Path) -> None:
    edit(specs, "ai", lambda d: d["components"]["schemas"].pop("CheckMap"))
    # CheckMap is still referenced, so the spec itself is also invalid; either way it must fail.
    assert check.run(specs)


def test_a_changed_shared_header_is_reported(specs: Path) -> None:
    edit(specs, "ai", lambda d: d["components"]["headers"]["RequestId"]["schema"].update(pattern="y"))
    assert "ai: shared header RequestId differs from core-public" in check.run(specs)


def test_an_example_that_breaks_its_schema_is_reported(specs: Path) -> None:
    def break_example(d: dict) -> None:  # type: ignore[type-arg]
        d["paths"]["/api/v1/status"]["get"]["responses"]["200"]["content"]["application/json"]["examples"][
            "operational"
        ]["value"]["status"] = "fine"

    edit(specs, "core-internal", break_example)
    problems = check.run(specs)
    assert any("core-internal: GET /api/v1/status 200" in p and "operational" in p for p in problems)


def test_an_example_with_an_extra_field_is_reported(specs: Path) -> None:
    def add_field(d: dict) -> None:  # type: ignore[type-arg]
        d["paths"]["/healthz"]["get"]["responses"]["200"]["content"]["application/json"]["example"]["debug"] = "x"

    edit(specs, "core-public", add_field)
    assert any("core-public: GET /healthz 200" in p for p in check.run(specs))


def test_deref_resolves_nested_references() -> None:
    doc = {"a": {"$ref": "#/b"}, "b": {"c": [{"$ref": "#/d"}]}, "d": 1}
    assert check.deref(doc["a"], doc) == {"c": [1]}


def test_deref_refuses_remote_references() -> None:
    with pytest.raises(ValueError, match="only local"):
        check.deref({"$ref": "http://example.com/x.yaml"}, {})


def test_main_exit_codes(monkeypatch: pytest.MonkeyPatch, tmp_path: Path, capsys: pytest.CaptureFixture[str]) -> None:
    assert check.main() == 0
    assert capsys.readouterr().out.startswith("ok")
    monkeypatch.setattr(check, "run", lambda _d: ["x: broken"])
    assert check.main() == 1
    assert "FAIL x: broken" in capsys.readouterr().out


def test_examples_extraction_is_complete_and_stable() -> None:
    import extract_examples

    doc = check.load(CONTRACTS / "core-internal.yaml")
    first = extract_examples.render(doc)
    assert first == extract_examples.render(doc)
    extracted = extract_examples.extract(doc)
    assert set(extracted["GET /api/v1/status 200"]) == {
        "operational",
        "ai_unreachable",
        "ai_down",
        "ai_unconfigured",
        "redis_down",
        "unavailable",
    }
    assert extracted["GET /readyz 200"]["example"]["status"] == "ready"
