from __future__ import annotations

import pytest
from pydantic import ValidationError

from app.config import BaseSettings, ConfigError, ServiceSettings, parse_duration

TOKEN = "t" * 40


def valid(**overrides: str) -> dict[str, str]:
    return {"DATABASE_URL": "postgresql://u:p@localhost:5432/db", "AI_SERVICE_TOKEN": TOKEN, **overrides}


def test_defaults() -> None:
    s = ServiceSettings.from_env(valid())
    assert (s.app_env, s.log_level, s.http_host, s.http_port, s.shutdown_timeout) == (
        "development",
        "info",
        "127.0.0.1",
        8000,
        10.0,
    )


def test_overrides() -> None:
    s = ServiceSettings.from_env(
        valid(APP_ENV="production", LOG_LEVEL="debug", HTTP_HOST="0.0.0.0", HTTP_PORT="9001", SHUTDOWN_TIMEOUT="3s")  # noqa: S104
    )
    assert (s.app_env, s.log_level, s.http_host, s.http_port, s.shutdown_timeout) == (
        "production",
        "debug",
        "0.0.0.0",  # noqa: S104
        9001,
        3.0,
    )


def test_whitespace_is_trimmed_and_empty_means_unset() -> None:
    s = ServiceSettings.from_env(valid(HTTP_PORT="  9002  ", LOG_LEVEL="   "))
    assert s.http_port == 9002
    assert s.log_level == "info"


def test_secrets_are_hidden_in_repr() -> None:
    s = ServiceSettings.from_env(valid())
    assert TOKEN not in repr(s)
    assert "u:p@" not in repr(s)


@pytest.mark.parametrize(
    ("overrides", "expected"),
    [
        ({"DATABASE_URL": ""}, "DATABASE_URL: is required"),
        ({"AI_SERVICE_TOKEN": ""}, "AI_SERVICE_TOKEN: is required"),
        ({"DATABASE_URL": "mysql://u:p@h/db"}, "DATABASE_URL: scheme must be"),
        ({"DATABASE_URL": "postgresql:///db"}, "DATABASE_URL: must include a host"),
        ({"DATABASE_URL": "postgresql://u:p@h:notaport/db"}, "DATABASE_URL: must be a valid URL"),
        ({"AI_SERVICE_TOKEN": "short"}, "AI_SERVICE_TOKEN: must be at least 32 characters"),
        (
            {"AI_SERVICE_TOKEN": "a" * 20 + " " + "b" * 20},
            "AI_SERVICE_TOKEN: may only contain letters, digits, - and _",
        ),
        ({"AI_SERVICE_TOKEN": "a" * 32 + "@:/"}, "AI_SERVICE_TOKEN: may only contain letters, digits, - and _"),
        ({"AI_SERVICE_TOKEN": "a" * 32 + "\u00e9"}, "AI_SERVICE_TOKEN: may only contain letters, digits, - and _"),
        ({"HTTP_PORT": "http"}, "HTTP_PORT:"),
        ({"HTTP_PORT": "70000"}, "HTTP_PORT:"),
        ({"HTTP_PORT": "0"}, "HTTP_PORT:"),
        ({"APP_ENV": "staging"}, "APP_ENV:"),
        ({"LOG_LEVEL": "loud"}, "LOG_LEVEL:"),
        ({"SHUTDOWN_TIMEOUT": "soon"}, "SHUTDOWN_TIMEOUT:"),
        ({"SHUTDOWN_TIMEOUT": "10"}, "SHUTDOWN_TIMEOUT:"),
        ({"SHUTDOWN_TIMEOUT": "0s"}, "SHUTDOWN_TIMEOUT:"),
        ({"SHUTDOWN_TIMEOUT": "-1s"}, "SHUTDOWN_TIMEOUT:"),
        ({"SHUTDOWN_TIMEOUT": "5m"}, "SHUTDOWN_TIMEOUT:"),
    ],
)
def test_rejects_bad_values(overrides: dict[str, str], expected: str) -> None:
    with pytest.raises(ConfigError) as info:
        ServiceSettings.from_env(valid(**overrides))
    assert expected in str(info.value)


def test_reports_all_problems_at_once() -> None:
    with pytest.raises(ConfigError) as info:
        ServiceSettings.from_env({"APP_ENV": "nope", "LOG_LEVEL": "nope"})
    message = str(info.value)
    for key in ("APP_ENV", "LOG_LEVEL", "DATABASE_URL", "AI_SERVICE_TOKEN"):
        assert key in message


@pytest.mark.parametrize(
    "overrides",
    [
        {"DATABASE_URL": "mysql://user:hunter2-top-secret@host/db"},
        {"DATABASE_URL": "postgresql://user:hunter2-top-secret@host:bad/db"},
        {"AI_SERVICE_TOKEN": "hunter2-top-secret"},
        {"HTTP_PORT": "hunter2-top-secret"},
        {"APP_ENV": "hunter2-top-secret"},
        {"SHUTDOWN_TIMEOUT": "hunter2-top-secret"},
    ],
)
def test_errors_never_contain_values(overrides: dict[str, str]) -> None:
    with pytest.raises(ConfigError) as info:
        ServiceSettings.from_env(valid(**overrides))
    assert "hunter2" not in str(info.value)
    assert info.value.__cause__ is None  # the pydantic error (which holds the input) is not chained


def test_base_settings_need_no_urls() -> None:
    s = BaseSettings.from_env({})
    assert s.app_env == "development"
    with pytest.raises(ConfigError, match="LOG_LEVEL"):
        BaseSettings.from_env({"LOG_LEVEL": "loud"})


@pytest.mark.parametrize(("raw", "seconds"), [("500ms", 0.5), ("10s", 10.0), ("1m", 60.0), ("2.5s", 2.5)])
def test_parse_duration(raw: str, seconds: float) -> None:
    assert parse_duration(raw) == seconds


def test_non_string_duration_is_rejected() -> None:
    # Environment values are always strings; a bare number has no unit, so it is refused.
    with pytest.raises(ValidationError):
        BaseSettings.model_validate({"shutdown_timeout": 5})
