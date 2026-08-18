from __future__ import annotations

import asyncio
from typing import Any

import httpx2
import openai
import pytest
from pydantic import ValidationError

from jobfinder_ai.capabilities.single import embed
from jobfinder_ai.failures import CapabilityError


class _FakeEmbeddingsModel:
    """Stands in for `gateway.embeddings_model(...)` — supports the one
    method `embed.run` calls, `aembed_query`."""

    def __init__(self, respond: Any) -> None:
        self._respond = respond
        self.calls: list[str] = []

    async def aembed_query(self, text: str) -> list[float]:
        self.calls.append(text)
        return self._respond(text)


def _vector(dims: int = embed.EMBED_DIMS, *, fill: float = 0.5) -> list[float]:
    return [fill] * dims


def test_run_returns_a_vector_of_the_configured_dimensionality(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fake = _FakeEmbeddingsModel(lambda _text: _vector())
    monkeypatch.setattr(embed.gateway, "embeddings_model", lambda task_key: fake)

    result, usage = asyncio.run(embed.run(embed.EmbedInput(text="hello world")))

    assert len(result.vector) == embed.EMBED_DIMS
    assert result.vector == _vector()
    assert usage.input_tokens is None
    assert fake.calls == ["hello world"]


def test_run_truncates_text_past_the_backstop_limit(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeEmbeddingsModel(lambda _text: _vector())
    monkeypatch.setattr(embed.gateway, "embeddings_model", lambda task_key: fake)

    long_text = "a" * (embed.EMBED_MAX_CHARS + 500)
    asyncio.run(embed.run(embed.EmbedInput(text=long_text)))

    assert len(fake.calls[0]) == embed.EMBED_MAX_CHARS


def test_run_leaves_short_text_untouched(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeEmbeddingsModel(lambda _text: _vector())
    monkeypatch.setattr(embed.gateway, "embeddings_model", lambda task_key: fake)

    asyncio.run(embed.run(embed.EmbedInput(text="short")))

    assert fake.calls == ["short"]


def test_run_rejects_a_vector_of_the_wrong_length_as_internal(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fake = _FakeEmbeddingsModel(lambda _text: _vector(dims=embed.EMBED_DIMS - 1))
    monkeypatch.setattr(embed.gateway, "embeddings_model", lambda task_key: fake)

    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(embed.run(embed.EmbedInput(text="hello")))

    assert exc_info.value.category == "internal"
    assert exc_info.value.failed_step == "embed"


def test_run_classifies_a_rate_limit_from_the_gateway(monkeypatch: pytest.MonkeyPatch) -> None:
    request = httpx2.Request("POST", "https://gateway.test/v1/embeddings")
    response = httpx2.Response(429, request=request)

    class _RaisingModel:
        async def aembed_query(self, text: str) -> list[float]:
            raise openai.RateLimitError("rate limited", response=response, body=None)

    monkeypatch.setattr(embed.gateway, "embeddings_model", lambda task_key: _RaisingModel())

    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(embed.run(embed.EmbedInput(text="hello")))

    assert exc_info.value.category == "rate_limited"
    assert exc_info.value.retryable is True


def test_embed_result_rejects_a_vector_of_the_wrong_length() -> None:
    with pytest.raises(ValidationError):
        embed.EmbedResult(vector=_vector(dims=embed.EMBED_DIMS - 1))


def test_capability_declares_http_transport_and_one_model_call() -> None:
    assert embed.CAPABILITY.bounds.max_model_calls == 1
    assert embed.CAPABILITY.task_key == "embed"
    assert embed.CAPABILITY.transport == "http"
    assert embed.CAPABILITY.layer == "chain"
    assert embed.CAPABILITY.name == "embed"
