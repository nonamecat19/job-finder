from __future__ import annotations

import asyncio
import json
from collections.abc import Callable
from typing import Any

import pytest
from langchain_core.messages import AIMessage

from jobfinder_ai.capabilities.single import recruiter
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
        usage_metadata={"input_tokens": 200, "output_tokens": 30, "total_tokens": 230},
    )


def test_run_returns_named_contacts(monkeypatch: pytest.MonkeyPatch) -> None:
    payload = {
        "contacts": [
            {"name": "Jane Doe", "title": "Recruiter", "email": "jane@acme.com"},
        ]
    }
    fake = _FakeChatModel(lambda _msgs: _ai_message(payload))
    monkeypatch.setattr(recruiter.gateway, "chat_model", lambda task_key: fake)

    data = recruiter.RecruiterInput(source="posting", text="Contact: Jane Doe, jane@acme.com")
    result, usage = asyncio.run(recruiter.run(data))

    assert len(result.contacts) == 1
    assert result.contacts[0].name == "Jane Doe"
    assert result.contacts[0].email == "jane@acme.com"
    assert result.contacts[0].phone == ""
    assert usage.input_tokens == 200


def test_run_returns_empty_contacts_when_none_named(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message({"contacts": []}))
    monkeypatch.setattr(recruiter.gateway, "chat_model", lambda task_key: fake)

    data = recruiter.RecruiterInput(source="company_page", text="Generic about page.")
    result, _usage = asyncio.run(recruiter.run(data))

    assert result.contacts == []


@pytest.mark.parametrize("source", ["posting", "company_page", "linkedin"])
def test_run_builds_a_source_specific_prompt(source: str, monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message({"contacts": []}))
    monkeypatch.setattr(recruiter.gateway, "chat_model", lambda task_key: fake)

    data = recruiter.RecruiterInput(source=source, text="some scraped text")  # type: ignore[arg-type]
    asyncio.run(recruiter.run(data))

    assert fake.bound is not None
    prompt = fake.bound.calls[0][1].content
    assert "some scraped text" in prompt


def test_run_retries_once_on_malformed_json_then_succeeds(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    responses = [AIMessage(content="not json"), _ai_message({"contacts": []})]

    def respond(_msgs: list[Any]) -> AIMessage:
        return responses.pop(0)

    fake = _FakeChatModel(respond)
    monkeypatch.setattr(recruiter.gateway, "chat_model", lambda task_key: fake)

    data = recruiter.RecruiterInput(source="posting", text="text")
    result, _usage = asyncio.run(recruiter.run(data))

    assert result.contacts == []
    assert fake.bound is not None
    assert len(fake.bound.calls) == 2


def test_run_fails_internal_after_exhausting_every_retry(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fake = _FakeChatModel(lambda _msgs: AIMessage(content="still not json"))
    monkeypatch.setattr(recruiter.gateway, "chat_model", lambda task_key: fake)

    data = recruiter.RecruiterInput(source="posting", text="text")
    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(recruiter.run(data))

    assert exc_info.value.category == "internal"
    assert exc_info.value.failed_step == "recruiter"
    assert fake.bound is not None
    assert len(fake.bound.calls) == recruiter.MAX_EXTRA_ATTEMPTS + 1


def test_capability_declares_http_transport_and_one_call_bound() -> None:
    assert recruiter.CAPABILITY.transport == "http"
    assert recruiter.CAPABILITY.task_key == "recruiter"
    assert recruiter.CAPABILITY.bounds.max_model_calls == 1
