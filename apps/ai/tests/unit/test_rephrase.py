from __future__ import annotations

import asyncio
from collections.abc import Callable
from typing import Any

import httpx2
import openai
import pytest
from langchain_core.messages import AIMessage

from jobfinder_ai.capabilities.single import rephrase
from jobfinder_ai.failures import CapabilityError


class _FakeBoundModel:
    def __init__(self, respond: Callable[[list[Any]], AIMessage]) -> None:
        self._respond = respond
        self.calls: list[list[Any]] = []

    async def ainvoke(self, messages: list[Any], config: dict[str, Any] | None = None) -> AIMessage:
        self.calls.append(messages)
        return self._respond(messages)


class _FakeChatModel:
    def __init__(self, respond: Callable[[list[Any]], AIMessage]) -> None:
        self._respond = respond
        self.bind_kwargs: dict[str, Any] | None = None
        self.bound: _FakeBoundModel | None = None

    def bind(self, **kwargs: Any) -> _FakeBoundModel:
        self.bind_kwargs = kwargs
        self.bound = _FakeBoundModel(self._respond)
        return self.bound


def _ai_message(text: str) -> AIMessage:
    return AIMessage(
        content=text, usage_metadata={"input_tokens": 40, "output_tokens": 15, "total_tokens": 55}
    )


def test_run_returns_the_trimmed_model_text(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message("  Built a Kafka pipeline.  "))
    monkeypatch.setattr(rephrase.gateway, "chat_model", lambda task_key: fake)

    data = rephrase.RephraseInput(
        term="Kafka",
        canonical="Apache Kafka",
        source_bullet="Built a streaming pipeline.",
    )
    result, usage = asyncio.run(rephrase.run(data))

    assert result.rephrase == "Built a Kafka pipeline."
    assert usage.input_tokens == 40
    assert usage.output_tokens == 15


def test_run_makes_exactly_one_call_with_no_retry(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message("unchanged bullet"))
    monkeypatch.setattr(rephrase.gateway, "chat_model", lambda task_key: fake)

    data = rephrase.RephraseInput(term="Kafka", source_bullet="Built a pipeline.")
    asyncio.run(rephrase.run(data))

    assert fake.bound is not None
    assert len(fake.bound.calls) == 1


def test_run_omits_source_label_section_when_absent(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message("ok"))
    monkeypatch.setattr(rephrase.gateway, "chat_model", lambda task_key: fake)

    data = rephrase.RephraseInput(term="Kafka", source_bullet="Built a pipeline.")
    asyncio.run(rephrase.run(data))

    assert fake.bound is not None
    prompt = fake.bound.calls[0][1].content
    assert "SOURCE LABEL" not in prompt


def test_run_includes_source_label_section_when_present(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message("ok"))
    monkeypatch.setattr(rephrase.gateway, "chat_model", lambda task_key: fake)

    data = rephrase.RephraseInput(
        term="Kafka", source_bullet="Built a pipeline.", source_label="Junior Engineer, 2022-2024"
    )
    asyncio.run(rephrase.run(data))

    assert fake.bound is not None
    prompt = fake.bound.calls[0][1].content
    assert "SOURCE LABEL" in prompt
    assert "Junior Engineer, 2022-2024" in prompt
    assert "Do NOT inflate seniority" in prompt


def test_run_includes_prior_violations_in_the_prompt(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message("ok"))
    monkeypatch.setattr(rephrase.gateway, "chat_model", lambda task_key: fake)

    data = rephrase.RephraseInput(
        term="Kafka",
        source_bullet="Built a pipeline.",
        prior_violations=["proper noun 'Redis' not in profile"],
    )
    asyncio.run(rephrase.run(data))

    assert fake.bound is not None
    prompt = fake.bound.calls[0][1].content
    assert "violated the no-invention rule" in prompt
    assert "proper noun 'Redis' not in profile" in prompt


def test_run_sends_trace_id_as_gateway_request_metadata(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message("ok"))
    monkeypatch.setattr(rephrase.gateway, "chat_model", lambda task_key: fake)

    data = rephrase.RephraseInput(term="Kafka", source_bullet="Built a pipeline.")
    asyncio.run(rephrase.run(data, trace_id="trace_123"))

    assert fake.bind_kwargs is not None
    assert fake.bind_kwargs["extra_body"]["metadata"]["trace_id"] == "trace_123"


def test_run_classifies_a_rate_limit_from_the_gateway(monkeypatch: pytest.MonkeyPatch) -> None:
    request = httpx2.Request("POST", "https://gateway.test/v1/chat/completions")
    response = httpx2.Response(429, request=request)

    def respond(_msgs: list[Any]) -> AIMessage:
        raise openai.RateLimitError("rate limited", response=response, body=None)

    fake = _FakeChatModel(respond)
    monkeypatch.setattr(rephrase.gateway, "chat_model", lambda task_key: fake)

    data = rephrase.RephraseInput(term="Kafka", source_bullet="Built a pipeline.")
    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(rephrase.run(data))

    assert exc_info.value.category == "rate_limited"
    assert exc_info.value.retryable is True


def test_capability_declares_http_transport_and_one_call_bound() -> None:
    assert rephrase.CAPABILITY.transport == "http"
    assert rephrase.CAPABILITY.task_key == "rephrase"
    assert rephrase.CAPABILITY.bounds.max_model_calls == 1
    assert rephrase.CAPABILITY.layer == "chain"
