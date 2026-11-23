"""Configuration from environment variables.

Validation reports every problem at once and names the variable, but never
includes a value: values can contain passwords and tokens. Variable names and
formats match the Go service so both can share one environment.
"""

from __future__ import annotations

import re
from collections.abc import Mapping
from typing import Any, Literal, Self
from urllib.parse import urlsplit

from pydantic import BaseModel, ConfigDict, Field, SecretStr, ValidationError, field_validator

MIN_TOKEN_LENGTH = 32
MAX_SHUTDOWN_SECONDS = 60.0

_TOKEN_CHARS = re.compile(r"[A-Za-z0-9_-]+")
_DURATION = re.compile(r"^(\d+(?:\.\d+)?)(ms|s|m)$")
_UNIT_SECONDS = {"ms": 0.001, "s": 1.0, "m": 60.0}


class ConfigError(Exception):
    """Raised with one line per invalid variable; never contains a value."""


def parse_duration(raw: str) -> float:
    """Parse a Go-style duration ("500ms", "10s", "1m") into seconds."""
    match = _DURATION.match(raw)
    if not match:
        raise ValueError("must be a duration such as 10s, 500ms or 1m")
    seconds = float(match.group(1)) * _UNIT_SECONDS[match.group(2)]
    if seconds <= 0 or seconds > MAX_SHUTDOWN_SECONDS:
        raise ValueError("must be a duration between 1ms and 60s")
    return seconds


class BaseSettings(BaseModel):
    """Settings shared by every process (the HTTP service and the worker)."""

    model_config = ConfigDict(frozen=True, extra="ignore")

    app_env: Literal["development", "production"] = "development"
    log_level: Literal["debug", "info", "warning", "error"] = "info"
    shutdown_timeout: float = 10.0

    @field_validator("shutdown_timeout", mode="before")
    @classmethod
    def _duration(cls, value: Any) -> float:
        if isinstance(value, str):
            return parse_duration(value)
        raise ValueError("must be a duration such as 10s")

    @classmethod
    def from_env(cls, environ: Mapping[str, str]) -> Self:
        """Build settings from environment variables named after the fields in upper case."""
        data: dict[str, str] = {}
        for name in cls.model_fields:
            raw = environ.get(name.upper(), "").strip()
            if raw:  # empty counts as unset, so defaults apply and required fields fail
                data[name] = raw
        try:
            return cls.model_validate(data)
        except ValidationError as exc:
            raise ConfigError(_describe(exc)) from None


class DatabaseSettings(BaseSettings):
    """Settings for any process that talks to Postgres."""

    database_url: SecretStr

    @field_validator("database_url")
    @classmethod
    def _database_url(cls, value: SecretStr) -> SecretStr:
        try:
            parts = urlsplit(value.get_secret_value())
            host = parts.hostname
            _ = parts.port  # raises ValueError on a malformed port
        except ValueError:
            raise ValueError("must be a valid URL") from None
        if parts.scheme not in ("postgres", "postgresql"):
            raise ValueError("scheme must be one of: postgres, postgresql")
        if not host:
            raise ValueError("must include a host")
        return value


class ServiceSettings(DatabaseSettings):
    """Settings for the HTTP service."""

    # Localhost by default so a developer machine never exposes the service;
    # the container sets 0.0.0.0 inside its private network.
    http_host: str = "127.0.0.1"
    http_port: int = Field(default=8000, ge=1, le=65535)
    ai_service_token: SecretStr

    @field_validator("ai_service_token")
    @classmethod
    def _token(cls, value: SecretStr) -> SecretStr:
        token = value.get_secret_value()
        if len(token) < MIN_TOKEN_LENGTH:
            raise ValueError(f"must be at least {MIN_TOKEN_LENGTH} characters")
        # Same rule as the Go API (config.validateToken), so one value starts both or neither (audit F11).
        if not _TOKEN_CHARS.fullmatch(token):
            raise ValueError("may only contain letters, digits, - and _")
        return value


class WorkerSettings(DatabaseSettings):
    """Settings for the job worker, which is the only process that calls a model provider.

    Prices are US dollars per million tokens, set here because providers change them; the daily cap
    is checked against what the calls cost by these prices. With no OPENAI_API_KEY the worker still
    runs: its jobs fail cleanly and their credits are refunded.
    """

    openai_api_key: SecretStr | None = None
    llm_daily_cap_usd: float = Field(default=3.0, gt=0, le=1000)
    llm_fast_model: str = Field(default="gpt-4o-mini", min_length=1, max_length=100, pattern=r"^[A-Za-z0-9._:-]+$")
    llm_fast_input_usd_per_mtok: float = Field(default=0.15, ge=0, le=1000)
    llm_fast_output_usd_per_mtok: float = Field(default=0.60, ge=0, le=1000)
    llm_timeout: float = 45.0

    @field_validator("llm_timeout", mode="before")
    @classmethod
    def _llm_timeout(cls, value: Any) -> float:
        if isinstance(value, str):
            return parse_duration(value)
        raise ValueError("must be a duration such as 45s")


def _describe(exc: ValidationError) -> str:
    """Turn a ValidationError into "VAR: message" lines without any input values."""
    lines = []
    for err in exc.errors(include_input=False, include_url=False, include_context=False):
        field = str(err["loc"][0]).upper() if err["loc"] else "CONFIG"
        message = "is required" if err["type"] == "missing" else err["msg"]
        message = message.removeprefix("Value error, ")
        lines.append(f"{field}: {message}")
    return "\n".join(sorted(lines))
