from __future__ import annotations

import io
import json
import logging

from app import logs


def test_json_lines_with_service_and_extra_fields() -> None:
    stream = io.StringIO()
    logs.configure("info", "ai", stream)
    logging.getLogger("t").info("hello", extra={"request_id": "abc12345"})
    line = json.loads(stream.getvalue().strip().splitlines()[-1])
    assert line["msg"] == "hello"
    assert line["service"] == "ai"
    assert line["level"] == "INFO"
    assert line["request_id"] == "abc12345"


def test_level_filters_and_exceptions_are_included() -> None:
    stream = io.StringIO()
    logs.configure("warning", "ai", stream)
    log = logging.getLogger("t")
    log.info("hidden")
    try:
        raise ValueError("bad")
    except ValueError:
        log.exception("failed")
    out = stream.getvalue()
    assert "hidden" not in out
    assert "ValueError" in json.loads(out.strip().splitlines()[-1])["exception"]


def test_uvicorn_loggers_are_routed_to_json() -> None:
    stream = io.StringIO()
    logs.configure("info", "ai", stream)
    logging.getLogger("uvicorn.error").info("from uvicorn")
    assert json.loads(stream.getvalue().strip().splitlines()[-1])["msg"] == "from uvicorn"
