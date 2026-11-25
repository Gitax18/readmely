import pytest

from app.diagram import check, draw, grounded_names
from app.diagram_render import Edge, Node, layout, lint, render
from app.summarise import representative_paths

KNOWN = {
    "apps/web/package.json",
    "services/core/cmd/api/main.go",
    "services/core/internal/github/client.go",
    "services/ai/app/worker.py",
}
FACTS = {
    "frameworks": ["PostgreSQL (pgx)", "Redis"],
    "services": ["postgres", "redis", "api"],
    "env_vars": ["OPENAI_API_KEY", "DATABASE_URL", "API_PORT"],
}


def node(id_: str, kind: str, evidence: list[str], label: str = "") -> dict[str, object]:
    return {"id": id_, "label": label or id_.title(), "sublabel": "", "detail": "", "kind": kind, "evidence": evidence}


def edge(src: str, dst: str, evidence: list[str]) -> dict[str, object]:
    return {"from": src, "to": dst, "label": "calls", "kind": "sync", "evidence": evidence}


def test_representative_paths_cover_every_directory_before_a_second_file_from_any() -> None:
    paths = sorted(
        [f"apps/web/src/c{i:03}.ts" for i in range(200)]
        + ["services/core/cmd/api/main.go", "services/core/internal/db/db.go", "infra/docker-compose.yml"]
        + ["services/core/internal/db/db_test.go", "Makefile", "README.md"]
    )
    out = representative_paths(paths, ["services/core/cmd/api/main.go"], 10)
    assert {"services/core/cmd/api/main.go", "services/core/internal/db/db.go", "infra/docker-compose.yml",
            "Makefile"} <= set(out)  # fmt: skip
    assert "services/core/internal/db/db_test.go" not in out  # tests come last
    assert out == sorted(out)
    assert len(out) == 10


def test_grounded_names_come_from_frameworks_services_and_setting_names() -> None:
    names = grounded_names(FACTS)
    assert {"postgres", "postgresql", "redis", "openai"} <= names
    assert not names & {"api", "port", "database", "key"} - {"database"}  # generic words are left out


def test_a_cited_directory_counts_as_evidence() -> None:
    raw = {
        "title": "Demo",
        "nodes": [
            node("web", "client", ["apps/web/package.json"]),
            node("api", "gateway", ["services/core/cmd/api/main.go"]),
            node("github", "external", ["services/core/internal/github"]),
            node("ghost", "service", ["services/nowhere"]),
        ],
        "edges": [edge("web", "api", ["apps/web"]), edge("api", "github", ["services/core/internal/github/"])],
    }
    _title, nodes, edges = check(raw, KNOWN, min_nodes=3)
    assert [n.id for n in nodes] == ["web", "api", "github"]
    assert len(edges) == 2


def test_stores_and_outside_services_named_by_the_analysis_need_no_file() -> None:
    raw = {
        "title": "Demo",
        "nodes": [
            node("api", "gateway", ["services/core/cmd/api/main.go"]),
            node("pg", "datastore", [], "Postgres"),
            node("llm", "external", [], "OpenAI"),
            node("kafka", "queue", [], "Kafka"),  # not in the facts: invented
            node("billing", "service", [], "Postgres billing"),  # a service still needs a file
        ],
        "edges": [edge("api", "pg", []), edge("api", "llm", []), edge("api", "kafka", [])],
    }
    _title, nodes, edges = check(raw, KNOWN, min_nodes=3, grounded=grounded_names(FACTS))
    assert [n.id for n in nodes] == ["api", "pg", "llm"]
    assert [(e.src, e.dst) for e in edges] == [("api", "pg"), ("api", "llm")]
    with pytest.raises(ValueError, match="too little"):
        check(raw, KNOWN, min_nodes=3)  # without the facts, only the API is left


def test_the_detail_line_is_drawn_and_boxes_grow_to_fit_it() -> None:
    nodes = [
        Node("web", "Web app", "Next.js", "client", "Pages and the API bridge"),
        Node("api", "API", "Go, chi", "gateway", "Handles every request"),
        Node("pg", "Postgres", "", "datastore"),
    ]
    edges = [Edge("web", "api", "REST", "sync"), Edge("api", "pg", "SQL", "data")]
    lay = layout(nodes, edges)
    svg = render("Demo", nodes, edges, lay)
    assert not lint(nodes, edges, lay, svg)
    assert 'font-style="italic"' in svg and "Handles every request" in svg
    assert {b.h for b in lay.boxes.values() if b.w} == {74}
    assert ">Request</text>" in svg and ">Data</text>" in svg  # the line-style legend
    drawn, problems = draw("Demo", nodes, edges)
    assert drawn and not problems
