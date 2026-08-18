from __future__ import annotations

import asyncio
import json
from collections.abc import Callable
from typing import Any

import pytest
from langchain_core.messages import AIMessage

from jobfinder_ai.capabilities.single import outreach
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


def _ai_message(payload: dict[str, Any]) -> AIMessage:
    return AIMessage(
        content=json.dumps(payload),
        usage_metadata={"input_tokens": 90, "output_tokens": 40, "total_tokens": 130},
    )


VALID_RESULT = {
    "text": "Hi Jane! I just applied and would love to chat about the team.",
    "specific_claims": [],
}


def test_run_returns_parsed_draft(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message(VALID_RESULT))
    monkeypatch.setattr(outreach.gateway, "chat_model", lambda task_key: fake)

    data = outreach.OutreachInput(tone="warm", contact_name="Jane", company_name="Acme")
    result, usage = asyncio.run(outreach.run(data))

    assert result.text == VALID_RESULT["text"]
    assert result.specific_claims == []
    assert usage.input_tokens == 90


def test_run_includes_allowed_facts_in_the_prompt(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message(VALID_RESULT))
    monkeypatch.setattr(outreach.gateway, "chat_model", lambda task_key: fake)

    data = outreach.OutreachInput(
        tone="direct",
        facts=[outreach.Fact(kind="funding", value="Series B, $40M")],
    )
    asyncio.run(outreach.run(data))

    assert fake.bound is not None
    prompt = fake.bound.calls[0][1].content
    assert "ALLOWED FACTS" in prompt
    assert "Series B, $40M" in prompt


def test_run_includes_last_violation_feedback(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message(VALID_RESULT))
    monkeypatch.setattr(outreach.gateway, "chat_model", lambda task_key: fake)

    data = outreach.OutreachInput(tone="warm", last_violation="text was empty")
    asyncio.run(outreach.run(data))

    assert fake.bound is not None
    prompt = fake.bound.calls[0][1].content
    assert "Your previous attempt was rejected: text was empty" in prompt


def test_run_retries_once_on_malformed_json_then_succeeds(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    responses = [AIMessage(content="not json"), _ai_message(VALID_RESULT)]

    def respond(_msgs: list[Any]) -> AIMessage:
        return responses.pop(0)

    fake = _FakeChatModel(respond)
    monkeypatch.setattr(outreach.gateway, "chat_model", lambda task_key: fake)

    data = outreach.OutreachInput(tone="formal")
    result, _usage = asyncio.run(outreach.run(data))

    assert result.text == VALID_RESULT["text"]
    assert fake.bound is not None
    assert len(fake.bound.calls) == 2


def test_run_fails_internal_after_exhausting_every_retry(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fake = _FakeChatModel(lambda _msgs: AIMessage(content="still not json"))
    monkeypatch.setattr(outreach.gateway, "chat_model", lambda task_key: fake)

    data = outreach.OutreachInput(tone="warm")
    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(outreach.run(data))

    assert exc_info.value.category == "internal"
    assert exc_info.value.failed_step == "outreach"
    assert fake.bound is not None
    assert len(fake.bound.calls) == outreach.MAX_EXTRA_ATTEMPTS + 1


def test_capability_declares_http_transport_and_one_call_bound() -> None:
    assert outreach.CAPABILITY.transport == "http"
    assert outreach.CAPABILITY.task_key == "outreach"
    assert outreach.CAPABILITY.bounds.max_model_calls == 1
