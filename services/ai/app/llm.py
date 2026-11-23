"""The one place that talks to a model provider.

OpenAI's chat completions API with structured output. The host is fixed, redirects are refused,
responses are size-capped, and errors carry a short code, never text the provider sent (it can echo
the prompt, which holds repository text). The caller validates the JSON again against its own model:
the provider's schema enforcement is a help, not a guarantee.
"""

from __future__ import annotations

import json
import math
import time
from dataclasses import dataclass
from typing import Any

import httpx
from pydantic import SecretStr

from app.config import WorkerSettings

BASE_URL = "https://api.openai.com/v1"
MAX_RESPONSE_BYTES = 1 << 20
MAX_OUTPUT_TOKENS = 4_000  # the most any call may ask for
DEFAULT_OUTPUT_TOKENS = 2_000

# Errors a retry cannot fix, and errors it might.
PERMANENT = {"llm_not_configured", "llm_auth", "llm_bad_request", "llm_refused", "spend_cap_reached"}


class LlmError(Exception):
    def __init__(self, code: str) -> None:
        super().__init__(code)
        self.code = code

    @property
    def permanent(self) -> bool:
        return self.code in PERMANENT


@dataclass(frozen=True)
class Usage:
    input_tokens: int
    output_tokens: int
    cost_micro_usd: int
    model: str
    duration_ms: int


class LlmClient:
    def __init__(self, settings: WorkerSettings, transport: httpx.AsyncBaseTransport | None = None) -> None:
        self._settings = settings
        key: SecretStr | None = settings.openai_api_key
        self._key = key.get_secret_value() if key else None
        self._http = httpx.AsyncClient(
            base_url=BASE_URL, timeout=settings.llm_timeout, follow_redirects=False, transport=transport
        )

    @property
    def configured(self) -> bool:
        return self._key is not None

    @property
    def model(self) -> str:
        return self._settings.llm_fast_model

    def cost_micro_usd(self, input_tokens: int, output_tokens: int) -> int:
        s = self._settings
        return math.ceil(input_tokens * s.llm_fast_input_usd_per_mtok + output_tokens * s.llm_fast_output_usd_per_mtok)

    def worst_case_micro_usd(self, prompt_chars: int, max_output_tokens: int) -> int:
        """An upper estimate before the call: a token is at least one character in the worst case."""
        return self.cost_micro_usd(prompt_chars, max_output_tokens)

    async def close(self) -> None:
        await self._http.aclose()

    async def json_call(
        self,
        *,
        system: str,
        user: str,
        schema_name: str,
        schema: dict[str, Any],
        max_output_tokens: int = DEFAULT_OUTPUT_TOKENS,
    ) -> tuple[dict[str, Any], Usage]:
        """Ask the fast model for a JSON object that matches `schema`."""
        if self._key is None:
            raise LlmError("llm_not_configured")
        body = {
            "model": self.model,
            "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
            "response_format": {
                "type": "json_schema",
                "json_schema": {"name": schema_name, "strict": True, "schema": schema},
            },
            "max_completion_tokens": min(max_output_tokens, MAX_OUTPUT_TOKENS),
        }
        started = time.monotonic()
        try:
            async with self._http.stream(
                "POST", "/chat/completions", json=body, headers={"Authorization": f"Bearer {self._key}"}
            ) as res:
                status = res.status_code
                raw = bytearray()
                async for chunk in res.aiter_bytes():
                    raw += chunk
                    if len(raw) > MAX_RESPONSE_BYTES:
                        raise LlmError("llm_unavailable")
        except httpx.HTTPError:
            raise LlmError("llm_unavailable") from None
        duration_ms = int((time.monotonic() - started) * 1000)
        return self._interpret(status, bytes(raw), duration_ms)

    def _interpret(self, status: int, raw: bytes, duration_ms: int) -> tuple[dict[str, Any], Usage]:
        if status in (401, 403):
            raise LlmError("llm_auth")
        if status == 429:
            raise LlmError("llm_rate_limited")
        if status >= 500:
            raise LlmError("llm_unavailable")
        if status != 200:
            raise LlmError("llm_bad_request")
        try:
            data = json.loads(raw)
            choice = data["choices"][0]
            usage = data["usage"]
            used_in, used_out = int(usage["prompt_tokens"]), int(usage["completion_tokens"])
        except (ValueError, KeyError, IndexError, TypeError):
            raise LlmError("llm_invalid_output") from None
        meta = Usage(used_in, used_out, self.cost_micro_usd(used_in, used_out), self.model, duration_ms)
        message = choice.get("message") or {}
        content = message.get("content")
        if message.get("refusal") or choice.get("finish_reason") == "content_filter":
            raise LlmUsageError("llm_refused", meta)
        if choice.get("finish_reason") == "length" or not isinstance(content, str):
            raise LlmUsageError("llm_invalid_output", meta)
        try:
            parsed = json.loads(content)
        except ValueError:
            raise LlmUsageError("llm_invalid_output", meta) from None
        if not isinstance(parsed, dict):
            raise LlmUsageError("llm_invalid_output", meta)
        return parsed, meta


class LlmUsageError(LlmError):
    """An error after the provider answered, so the call still cost something and is metered."""

    def __init__(self, code: str, usage: Usage) -> None:
        super().__init__(code)
        self.usage = usage
