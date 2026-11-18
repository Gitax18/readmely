"""Keeps the Python view of the database in step with the migrations.

The comparison itself is a pure function, tested here against deliberately drifted columns on
every run. The tests marked `integration` apply it to the real migrated schema (`make up`
applies the migrations, then `make test-integration`).
"""

from __future__ import annotations

import os
import re
from dataclasses import dataclass
from datetime import UTC, datetime
from types import NoneType, UnionType
from typing import Any, Literal, Union, get_args, get_origin
from uuid import UUID

import psycopg
import pytest
from psycopg.rows import dict_row
from psycopg.types.json import Jsonb
from pydantic import BaseModel

from app.jobs import JobKind, JobRow, JobStatus


@dataclass(frozen=True)
class Column:
    name: str
    data_type: str  # information_schema.columns.data_type
    nullable: bool


# Postgres type -> the Python type a model field must have for it.
PYTHON_TYPE_FOR: dict[str, type] = {
    "uuid": UUID,
    "text": str,
    "integer": int,
    "jsonb": dict,
    "timestamp with time zone": datetime,
}


def split_optional(annotation: Any) -> tuple[Any, bool]:
    """`X | None` -> (X, True); anything else -> (annotation, False)."""
    if get_origin(annotation) in (UnionType, Union):
        args = get_args(annotation)
        rest = [a for a in args if a is not NoneType]
        if len(rest) == 1:
            return rest[0], len(rest) != len(args)
    return annotation, False


def python_type(annotation: Any) -> Any:
    """What a field's annotation means at run time: Literal[...] is a str, dict[...] is a dict."""
    origin = get_origin(annotation)
    if origin is Literal:
        return type(get_args(annotation)[0])
    return origin or annotation


def schema_problems(model: type[BaseModel], columns: list[Column]) -> list[str]:
    """Every way the model and the table disagree; empty means they match."""
    problems: list[str] = []
    fields = model.model_fields
    by_name = {c.name: c for c in columns}

    for name in sorted(by_name.keys() - fields.keys()):
        problems.append(f"column {name!r} is in the database but not in {model.__name__}")
    for name in sorted(fields.keys() - by_name.keys()):
        problems.append(f"field {name!r} is in {model.__name__} but not in the database")

    for name in sorted(fields.keys() & by_name.keys()):
        column = by_name[name]
        base, optional = split_optional(fields[name].annotation)
        if optional != column.nullable:
            want = "nullable" if column.nullable else "not nullable"
            problems.append(f"{name!r}: the column is {want} but the field is not")
        expected = PYTHON_TYPE_FOR.get(column.data_type)
        if expected is None:
            problems.append(f"{name!r}: no Python type is known for the Postgres type {column.data_type!r}")
        elif python_type(base) is not expected:
            problems.append(f"{name!r}: the column is {column.data_type} ({expected.__name__}), the field is {base}")
    return problems


COLUMNS = [
    Column("id", "uuid", False),
    Column("user_id", "uuid", False),
    Column("project_id", "uuid", True),
    Column("kind", "text", False),
    Column("payload", "jsonb", False),
    Column("status", "text", False),
    Column("attempts", "integer", False),
    Column("max_attempts", "integer", False),
    Column("run_after", "timestamp with time zone", False),
    Column("locked_by", "text", True),
    Column("lease_expires_at", "timestamp with time zone", True),
    Column("idempotency_key", "text", False),
    Column("error_code", "text", True),
    Column("created_at", "timestamp with time zone", False),
    Column("updated_at", "timestamp with time zone", False),
    Column("finished_at", "timestamp with time zone", True),
]


def test_the_comparison_accepts_a_matching_schema() -> None:
    assert schema_problems(JobRow, COLUMNS) == []


@pytest.mark.parametrize(
    ("drifted", "fragment"),
    [
        ([*COLUMNS, Column("priority", "integer", False)], "'priority' is in the database but not in JobRow"),
        ([c for c in COLUMNS if c.name != "attempts"], "'attempts' is in JobRow but not in the database"),
        (
            [Column(c.name, c.data_type, True) if c.name == "user_id" else c for c in COLUMNS],
            "'user_id': the column is nullable but the field is not",
        ),
        (
            [Column(c.name, c.data_type, False) if c.name == "locked_by" else c for c in COLUMNS],
            "'locked_by': the column is not nullable but the field is",
        ),
        (
            [Column(c.name, "bigint", c.nullable) if c.name == "attempts" else c for c in COLUMNS],
            "no Python type is known for the Postgres type 'bigint'",
        ),
        (
            [Column(c.name, "text", c.nullable) if c.name == "attempts" else c for c in COLUMNS],
            "'attempts': the column is text (str)",
        ),
        (
            [
                Column(c.name, "timestamp without time zone", c.nullable) if c.name == "run_after" else c
                for c in COLUMNS
            ],
            "no Python type is known",
        ),
    ],
)
def test_the_comparison_detects_drift(drifted: list[Column], fragment: str) -> None:
    problems = schema_problems(JobRow, drifted)
    assert any(fragment in p for p in problems), problems


# --- against the real, migrated database ---------------------------------------------------

integration = pytest.mark.integration


def database_url() -> str:
    url = os.environ.get("DATABASE_URL", "")
    if not url:
        pytest.skip("DATABASE_URL is not set; run through `make test-integration`")
    return url


def db_columns(conn: psycopg.Connection[Any], table: str) -> list[Column]:
    rows = conn.execute(
        "SELECT column_name, data_type, is_nullable FROM information_schema.columns "
        "WHERE table_schema = 'public' AND table_name = %s",
        (table,),
    ).fetchall()
    if not rows:
        pytest.fail(f"table {table!r} not found: run `make db-up` first")
    return [Column(str(r[0]), str(r[1]), r[2] == "YES") for r in rows]


@integration
def test_job_row_matches_the_migrated_jobs_table() -> None:
    with psycopg.connect(database_url()) as conn:
        assert schema_problems(JobRow, db_columns(conn, "jobs")) == []


@integration
@pytest.mark.parametrize(
    ("constraint", "literal"),
    [("jobs_kind_check", JobKind), ("jobs_status_check", JobStatus)],
)
def test_the_literals_match_the_check_constraints(constraint: str, literal: Any) -> None:
    with psycopg.connect(database_url()) as conn:
        row = conn.execute(
            "SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = %s", (constraint,)
        ).fetchone()
    assert row is not None, f"constraint {constraint} not found: run `make db-up` first"
    allowed = set(re.findall(r"'([a-z_]+)'", str(row[0])))
    assert allowed == set(get_args(literal)), f"{constraint} allows {sorted(allowed)}"


@integration
def test_a_real_row_parses_into_the_model() -> None:
    with psycopg.connect(database_url(), row_factory=dict_row) as conn, conn.transaction(force_rollback=True):
        user = conn.execute("INSERT INTO users (email) VALUES ('drift-test@example.invalid') RETURNING id").fetchone()
        assert user is not None
        conn.execute(
            "INSERT INTO jobs (user_id, kind, payload, idempotency_key) "
            "VALUES (%s, 'generate_readme', %s, 'drift-test-key-1')",
            (user["id"], Jsonb({"depth": 2})),
        )
        row = conn.execute("SELECT * FROM jobs WHERE idempotency_key = 'drift-test-key-1'").fetchone()
        job = JobRow.model_validate(row)

    assert job.kind == "generate_readme"
    assert job.status == "queued"
    assert job.payload == {"depth": 2}
    assert job.locked_by is None
    assert job.created_at.tzinfo is not None
    assert abs((datetime.now(UTC) - job.created_at).total_seconds()) < 60
