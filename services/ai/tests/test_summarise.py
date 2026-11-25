from app.summarise import check

KNOWN = {"cmd/api/main.go", "internal/db/db.go", "README.md"}


def test_too_many_components_are_cut_after_unknown_paths_are_dropped() -> None:
    raw = {
        "headline": "A demo.",
        "overview": "It does things.",
        "how_it_works": ["One.", "Two.", "", "Three.", "Four.", "Five.", "Six.", "Seven."],
        "components": [{"path": "nope/x.go", "role": "Invented."}]
        + [{"path": "cmd/api/main.go", "role": f"Entry {i}."} for i in range(5)]
        + [{"path": "internal/db/", "role": "Database access."}]
        + [{"path": "README.md", "role": "x " * 150}] * 4,
    }
    s = check(raw, KNOWN)
    assert len(s.components) == 8
    assert all(c.path != "nope/x.go" for c in s.components)
    assert s.components[5].path == "internal/db/"
    assert len(s.components[6].role) <= 200 and s.components[6].role.endswith("…")
    assert s.how_it_works == ["One.", "Two.", "Three.", "Four.", "Five.", "Six."]


def test_long_text_is_cut_at_a_word() -> None:
    raw = {"headline": "word " * 60, "overview": "Fine.", "how_it_works": ["a", "b"], "components": []}
    s = check(raw, KNOWN)
    assert len(s.headline) <= 160 and s.headline.endswith("word…")
