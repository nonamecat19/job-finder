from __future__ import annotations

import asyncio
import json
from collections.abc import Callable
from typing import Any

import httpx2
import openai
import pytest
from langchain_core.messages import AIMessage

from jobfinder_ai.capabilities.single import match
from jobfinder_ai.contracts.match_work import MatchWork
from jobfinder_ai.failures import CapabilityError


class _FakeBoundModel:
    """Stands in for the object `ChatOpenAI(...).bind(...)` returns —
    supports the one method `match.run` calls, `ainvoke`."""

    def __init__(self, respond: Callable[[list[Any]], AIMessage]) -> None:
        self._respond = respond
        self.calls: list[list[Any]] = []

    async def ainvoke(self, messages: list[Any], config: dict[str, Any] | None = None) -> AIMessage:
        self.calls.append(messages)
        return self._respond(messages)


class _FakeChatModel:
    """Stands in for `gateway.chat_model(...)` — `bind()` just captures
    kwargs and returns the fake bound model."""

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
        usage_metadata={"input_tokens": 100, "output_tokens": 20, "total_tokens": 120},
    )


def _work(snapshot: dict[str, Any]) -> MatchWork:
    return MatchWork(jobId="job_1", activityId=None, snapshot=snapshot, snapshot_hash="sha256:x")


FULL_SNAPSHOT = {
    "profile_text": "Senior Backend Engineer, 8 years Go and Python.",
    "title": "Staff Backend Engineer",
    "company": "Acme Corp",
    "location": "Remote",
    "remote": True,
    "description": "Build and operate distributed systems in Go.",
}

VALID_RESULT = {
    "score": 82.0,
    "matchedSkills": ["Go", "distributed systems"],
    "missingSkills": ["Kubernetes"],
    "summary": "Strong fit with minor gaps.",
    "redFlags": [],
}


def test_run_returns_parsed_result(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message(VALID_RESULT))
    monkeypatch.setattr(match.gateway, "chat_model", lambda task_key: fake)

    result, usage = asyncio.run(match.run(_work(FULL_SNAPSHOT)))

    assert result.score == 82.0
    assert result.matchedSkills == ["Go", "distributed systems"]
    assert result.missingSkills == ["Kubernetes"]
    assert result.summary == "Strong fit with minor gaps."
    assert result.redFlags == []
    assert usage.input_tokens == 100
    assert usage.output_tokens == 20
    assert fake.bind_kwargs is not None
    assert fake.bind_kwargs["response_format"] == {"type": "json_object"}


def test_run_sends_trace_id_as_gateway_request_metadata(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message(VALID_RESULT))
    monkeypatch.setattr(match.gateway, "chat_model", lambda task_key: fake)

    asyncio.run(match.run(_work(FULL_SNAPSHOT), trace_id="trace_123"))

    assert fake.bind_kwargs is not None
    assert fake.bind_kwargs["extra_body"]["metadata"]["trace_id"] == "trace_123"


def test_run_uses_the_verbatim_ported_prompt(monkeypatch: pytest.MonkeyPatch) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message(VALID_RESULT))
    monkeypatch.setattr(match.gateway, "chat_model", lambda task_key: fake)

    asyncio.run(match.run(_work(FULL_SNAPSHOT)))

    assert fake.bound is not None
    system_message, human_message = fake.bound.calls[0]
    assert system_message.content == (
        "You are a precise technical recruiter. Judge only from the given profile and job text."
    )
    assert (
        "CANDIDATE PROFILE:\nSenior Backend Engineer, 8 years Go and Python."
        in human_message.content
    )
    assert "Location: Remote (remote: true)" in human_message.content


def test_run_retries_once_on_malformed_json_then_succeeds(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    responses = [AIMessage(content="not json at all"), _ai_message(VALID_RESULT)]

    def respond(_msgs: list[Any]) -> AIMessage:
        return responses.pop(0)

    fake = _FakeChatModel(respond)
    monkeypatch.setattr(match.gateway, "chat_model", lambda task_key: fake)

    result, _usage = asyncio.run(match.run(_work(FULL_SNAPSHOT)))

    assert result.score == 82.0
    assert fake.bound is not None
    assert len(fake.bound.calls) == 2
    retry_prompt = fake.bound.calls[1][1].content
    assert "Your previous answer was invalid" in retry_prompt


def test_run_fails_internal_after_exhausting_every_retry(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fake = _FakeChatModel(lambda _msgs: AIMessage(content="still not json"))
    monkeypatch.setattr(match.gateway, "chat_model", lambda task_key: fake)

    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(match.run(_work(FULL_SNAPSHOT)))

    assert exc_info.value.category == "internal"
    assert exc_info.value.failed_step == "match"
    assert fake.bound is not None
    assert len(fake.bound.calls) == match.MAX_EXTRA_ATTEMPTS + 1


def test_run_rejects_a_malformed_snapshot_as_invalid_input(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fake = _FakeChatModel(lambda _msgs: _ai_message(VALID_RESULT))
    monkeypatch.setattr(match.gateway, "chat_model", lambda task_key: fake)

    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(match.run(_work({"title": "only a title"})))

    assert exc_info.value.category == "invalid_input"


def test_run_classifies_a_rate_limit_from_the_gateway(monkeypatch: pytest.MonkeyPatch) -> None:
    request = httpx2.Request("POST", "https://gateway.test/v1/chat/completions")
    response = httpx2.Response(429, request=request)

    def respond(_msgs: list[Any]) -> AIMessage:
        raise openai.RateLimitError("rate limited", response=response, body=None)

    fake = _FakeChatModel(respond)
    monkeypatch.setattr(match.gateway, "chat_model", lambda task_key: fake)

    with pytest.raises(CapabilityError) as exc_info:
        asyncio.run(match.run(_work(FULL_SNAPSHOT)))

    assert exc_info.value.category == "rate_limited"
    assert exc_info.value.retryable is True


def test_capability_declares_one_model_call_bound() -> None:
    assert match.CAPABILITY.bounds.max_model_calls == 1
    assert match.CAPABILITY.task_key == "match"
    assert match.CAPABILITY.transport == "event"
    assert match.CAPABILITY.layer == "chain"
