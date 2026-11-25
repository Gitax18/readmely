from app.readme import (
    TEMPLATES,
    Diagram,
    Readme,
    anchor,
    assemble,
    badges,
    build_readme_prompt,
    clean_mermaid,
    file_tree,
    guide_for,
    mermaid_ok,
    normalise_body,
    plan_sections,
    render_diagram,
    sections_of,
    tree_paths,
    with_tree,
)

FACTS = {
    "languages": [
        {"name": "Markdown", "files": 9, "lines": 900, "percent": 40.0},
        {"name": "Go", "files": 40, "lines": 4000, "percent": 35.0},
        {"name": "TypeScript", "files": 30, "lines": 3000, "percent": 20.0},
        {"name": "C++", "files": 1, "lines": 10, "percent": 5.0},
    ],
    "frameworks": ["Chi", "Cobra", "Docker Compose", "GitHub Actions", "Next.js", "PostgreSQL (pgx)", "Vitest"],
    "has_docker": True,
    "has_tests": True,
    "env_vars": ["DATABASE_URL"],
    "license": "MIT",
}


def test_badges_rank_app_frameworks_and_data_before_tooling_and_skip_unknown_names() -> None:
    out = badges(FACTS)
    assert out[0] == "![Go](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)"
    assert "C%2B%2B-00599C" in out[2]  # C++, escaped
    alts = [b[2 : b.index("]")] for b in out]
    assert alts == ["Go", "TypeScript", "C++", "Chi", "Next.js", "PostgreSQL", "GitHub Actions", "Vitest", "Docker",
                    "License: MIT"]  # fmt: skip
    assert out[-1] == "![License: MIT](https://img.shields.io/badge/License-MIT-blue?style=for-the-badge)"
    assert not any("Markdown" in b or "Cobra" in b or "Compose" in b for b in out)


def test_badges_leave_room_for_docker_and_the_licence() -> None:
    many = {**FACTS, "frameworks": ["Next.js", "React", "FastAPI", "Django", "Flask", "Express", "Redis", "Vite"]}
    alts = [b[2 : b.index("]")] for b in badges(many)]
    assert len(alts) == 10
    assert alts[-2:] == ["Docker", "License: MIT"]


def test_no_licence_badge_for_an_unrecognised_licence() -> None:
    assert badges({"license": "Other"}) == []
    assert badges({}) == []


PATHS = {"README.md", "go.mod", "cmd/api/main.go", "internal/db/db.go", "internal/http/server.go", "web/app.ts"}


def test_file_tree_shows_two_levels_directories_first_with_aligned_notes() -> None:
    tree = file_tree("demo", PATHS, {"cmd/api/": "The HTTP server", "./internal": "Packages", "nope/": "x"})
    assert tree == "\n".join(
        [
            "demo/",
            "├── cmd/",
            "│   └── api/    # The HTTP server",
            "├── internal/   # Packages",
            "│   ├── db/",
            "│   └── http/",
            "├── web/",
            "│   └── app.ts",
            "├── go.mod",
            "└── README.md",
        ]
    )
    assert tree_paths(PATHS) == ["cmd/", "cmd/api/", "internal/", "internal/db/", "internal/http/", "web/"]


def test_file_tree_lists_subfolders_collapses_files_and_keeps_top_level_files() -> None:
    paths = {f"plans/p{i:02}.md" for i in range(20)} | {f"contracts/c{i}.yaml" for i in range(5)}
    paths |= {"contracts/gen/x.go", "contracts/tests/t.py", "Makefile"}
    assert file_tree("demo", paths).splitlines() == [
        "demo/",
        "├── contracts/",
        "│   ├── gen/",
        "│   ├── tests/",
        "│   └── … 5 more",
        "├── plans/",
        "│   ├── p00.md",
        "│   ├── p01.md",
        "│   ├── p02.md",
        "│   ├── p03.md",
        "│   └── … 16 more",
        "└── Makefile",
    ]


def test_the_diagram_is_drawn_in_code_from_the_typed_graph() -> None:
    diagram = Diagram.model_validate(
        {
            "nodes": [
                {"id": "user", "label": "Browser", "kind": "user", "group": ""},
                {"id": "web", "label": 'Web app "Next.js" <b>', "kind": "app", "group": "Vercel"},
                {"id": "api", "label": "API (Go)", "kind": "service", "group": "AWS"},
                {"id": "db", "label": "Postgres", "kind": "database", "group": "AWS"},
                {"id": "x", "label": "Odd", "kind": "spaceship", "group": ""},
            ],
            "edges": [
                {"from": "user", "to": "web", "label": "HTTPS"},
                {"from": "web", "to": "api", "label": ""},
                {"from": "api", "to": "db", "label": "SQL; DROP"},
                {"from": "api", "to": "ghost", "label": "nope"},
            ],
        }
    )
    assert render_diagram(diagram) == "\n".join(
        [
            "```mermaid",
            "flowchart LR",
            '  n_user(["Browser"])',
            '  n_x["Odd"]',
            '  subgraph g0["Vercel"]',
            '    n_web["Web app Next.js b"]',
            "  end",
            '  subgraph g1["AWS"]',
            '    n_api["API (Go)"]',
            '    n_db[("Postgres")]',
            "  end",
            '  n_user -->|"HTTPS"| n_web',
            "  n_web --> n_api",
            '  n_api -->|"SQL DROP"| n_db',
            "```",
        ]
    )


def test_too_small_a_graph_draws_nothing() -> None:
    assert render_diagram(Diagram()) is None
    two = {"nodes": [{"id": i, "label": i, "kind": "app", "group": ""} for i in "ab"],
           "edges": [{"from": "a", "to": "b", "label": ""}]}  # fmt: skip
    assert render_diagram(Diagram.model_validate(two)) is None


def test_the_diagram_goes_after_the_first_paragraph_of_the_architecture_section() -> None:
    readme = _readme(["Overview", "Architecture"], "First paragraph.\n\n| a | b |")
    readme.diagram = Diagram.model_validate(
        {
            "nodes": [{"id": i, "label": i.upper(), "kind": "service", "group": ""} for i in "abc"],
            "edges": [{"from": "a", "to": "b", "label": ""}, {"from": "b", "to": "c", "label": ""}],
        }
    )
    content = assemble(readme)
    arch = content[content.index("## Architecture") :]
    assert arch.startswith("## Architecture\n\nFirst paragraph.\n\n```mermaid\nflowchart LR\n")
    assert "```mermaid" not in content[: content.index("## Architecture")]


def test_common_flowchart_forms_pass_and_decoration_is_dropped() -> None:
    cleaned = clean_mermaid(
        """flowchart LR
          user([Browser]) --> web["Next.js web app<br/>(Vercel)"]
          web -- "HTTPS + origin key" --> api["API (Go)"]
          subgraph aws ["AWS"]
            api -->|SQL| db[("Postgres")]
            api -.-> cache[(Redis)]:::store
          end
          a & b --> api
          classDef store fill:#eef
          click api "https://example.com"
          %% a comment"""
    )
    assert cleaned is not None
    assert "classDef" not in cleaned
    assert "click" not in cleaned
    assert ":::" not in cleaned


def test_mermaid_that_might_not_draw_or_carries_html_is_refused() -> None:
    assert not mermaid_ok("sequenceDiagram\n  A->>B: hi")
    assert not mermaid_ok("flowchart LR\n  a[Bad (parens)] --> b")
    assert not mermaid_ok('flowchart LR\n  a["<img src=x onerror=alert(1)>"]')
    assert not mermaid_ok('flowchart LR\n  end["Finish"]')
    assert not mermaid_ok('flowchart LR\n  subgraph g["G"]\n  a["x"]')  # unclosed


def test_normalise_body_demotes_headings_and_drops_mermaid_that_would_not_draw() -> None:
    body = "\n".join(
        [
            "# Big heading",
            "Some text.",
            "```mermaid",
            "sequenceDiagram",
            "  a->>b: hi",
            "```",
            "```mermaid",
            "flowchart TD",
            '  a["Fine"] --> b["Also fine"]',
            "```",
            "```bash",
            "# a comment, not a heading",
            "make test",
            "```",
        ]
    )
    out = normalise_body(body)
    assert out.startswith('### Big heading\nSome text.\n```mermaid\nflowchart TD\n  a["Fine"] --> b["Also fine"]\n```')
    assert "sequenceDiagram" not in out
    assert "# a comment, not a heading" in out


def test_an_unclosed_fence_is_closed_rather_than_swallowing_the_rest() -> None:
    assert normalise_body("```bash\nmake run") == "```bash\nmake run\n```"


def _readme(headings: list[str], body: str = "Text.") -> Readme:
    return Readme.model_validate(
        {
            "title": "Demo",
            "quote": '"Ship the docs with the code."',
            "description": "Demo turns a repository\ninto a README.",
            "tree_notes": [],
            "sections": [{"heading": h, "body": body} for h in headings],
        }
    )


def test_assemble_builds_the_header_and_a_contents_line() -> None:
    readme = _readme(["Overview", "Tech stack", "Getting started", "Project structure"])
    content = assemble(readme, ["![Go](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge)"])
    head = content.split("\n\n")
    assert head[0] == "# Demo"
    assert head[1] == "> *Ship the docs with the code.*"
    assert head[2].startswith("![Go]")
    assert head[3] == "Demo turns a repository into a README."
    assert head[4] == (
        "**Contents:** [Overview](#overview) · [Tech stack](#tech-stack) · "
        "[Getting started](#getting-started) · [Project structure](#project-structure)"
    )
    assert [t for t, _a, _b in sections_of(content)] == [
        "Overview",
        "Tech stack",
        "Getting started",
        "Project structure",
    ]


def test_assemble_puts_the_drawn_tree_first_in_the_structure_section_and_drops_a_guessed_licence() -> None:
    drawn = "```text\nmodel-tree/\n└── made-up/\n```\nStart in cmd/."
    readme = _readme(["Project structure", "License"], drawn)
    tree = "```text\ndemo/\n└── cmd/\n```"
    content = assemble(readme, None, tree, has_licence=False)
    assert "made-up" not in content
    assert "## Project structure\n\n```text\ndemo/\n└── cmd/\n```\n\nStart in cmd/." in content
    assert "## License" not in content


def test_with_tree_keeps_a_body_without_a_tree_as_it_is() -> None:
    assert with_tree("Just words.", None) == "Just words."
    assert with_tree("```bash\nmake\n```", "```text\nx/\n└── y\n```") == "```text\nx/\n└── y\n```\n\n```bash\nmake\n```"


def test_anchor_matches_github() -> None:
    assert anchor("Getting started") == "getting-started"
    assert anchor("FAQ & Help!") == "faq--help"


def test_sections_the_facts_cannot_support_are_not_asked_for() -> None:
    assert plan_sections("detailed", {}) == [
        "Overview", "Features", "Architecture", "Tech stack", "Getting started", "Usage", "Project structure",
        "Contributing",
    ]  # fmt: skip
    assert plan_sections("detailed", FACTS) == TEMPLATES["detailed"]


def test_every_template_section_has_a_guide_and_the_prompt_carries_it() -> None:
    for sections in TEMPLATES.values():
        assert all(guide_for(h) for h in sections)
    material = {"name": "acme/demo", "description": "", "facts": FACTS, "paths": sorted(PATHS), "readme": "",
                "excerpts": {}}  # fmt: skip
    prompt = build_readme_prompt(material, None, "standard", PATHS)
    assert "- Tech stack: A table: Layer | Technology | Purpose" in prompt
    assert 'part="tree">\ndemo/\n├── cmd/' in prompt
    assert "Paths: cmd/ cmd/api/ internal/" in prompt
    assert guide_for("getting STARTED") == guide_for("Getting started")
