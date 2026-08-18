"""Integration tests for US1 (tracing): running the `ghost` capability
through `consumers._handle_ghost` end to end, with the Langfuse client and
the gateway model call both stubbed (no real network — a real Langfuse
collector or LLM provider isn't available in this environment). These tests
assert that `tracing.py`'s already-built functions (`run_trace`,
`mark_run_failed`, `gateway_call_metadata`, `callback_handler`,
`propagate_attributes`, `log_collector_error_once`) are invoked with the
shape US1's acceptance scenarios require (spec.md US1 scenarios 1-4).
"""

from __future__ import annotations

import asyncio
import json
import logging
import time
from collections.abc import Callable
from contextlib import contextmanager
from datetime import UTC, datetime
from typing import Any

import openai
import pytest
from langchain_core.messages import AIMessage

from jobfinder_ai import tracing
from jobfinder_ai.capabilities.single import ghost
from jobfinder_ai.contracts.envelope import Envelope
from jobfinder_ai.contracts.ghost_work import GhostWork
from jobfinder_ai.messaging import consumers

FAKE_CALLBACK_HANDLER = "FAKE_CALLBACK_HANDLER"


def setup_function() -> None:
    tracing._client = None
    tracing._collector_error_logged = False


def teardown_function() -> None:
    tracing._client = None
    tracing._collector_error_logged = False


class _FakeSpan:
    def __init__(self) -> None:
        self.updates: list[dict[str, Any]] = []

    def update(self, **kwargs: Any) -> None:
        self.updates.append(kwargs)


class _FakeObservation:
    def __init__(self, span: _FakeSpan) -> None:
        self._span = span

    def __enter__(self) -> _FakeSpan:
        return self._span

    def __exit__(self, *exc: object) -> bool:
        return False


class _FakeLangfuseClient:
    """Stands in for the bootstrapped `Langfuse` client. Records every
    observation (trace/span) started, purely in-process — like the real SDK,
    starting an observation is a local, synchronous operation; only export
    talks to the network, and only from a background thread (research R8)."""

    def __init__(self, *, trace_id: str = "trace_fake_1") -> None:
        self.trace_id = trace_id
        self.observations: list[dict[str, Any]] = []

    def start_as_current_observation(self, *, name: str, as_type: str) -> _FakeObservation:
        span = _FakeSpan()
        self.observations.append({"name": name, "as_type": as_type, "span": span})
        return _FakeObservation(span)

    def get_current_trace_id(self) -> str:
        return self.trace_id


def _fake_propagate_attributes_factory() -> tuple[Callable[..., Any], list[dict[str, Any]]]:
    calls: list[dict[str, Any]] = []

    @contextmanager
    def _fake(**kwargs: Any) -> Any:
        calls.append(kwargs)
        yield

    return _fake, calls


def _patch_tracing(
    monkeypatch: pytest.MonkeyPatch, fake_client: _FakeLangfuseClient
) -> list[dict[str, Any]]:
    """Stubs the collector boundary: `bootstrap()` returns `fake_client`
    rather than constructing a real Langfuse client, `propagate_attributes`
    records its kwargs instead of writing OTel baggage, and
    `callback_handler()` returns a sentinel instead of a real
    `CallbackHandler` bound to a real client."""
    monkeypatch.setattr(tracing, "bootstrap", lambda **_kwargs: fake_client)
    fake_propagate, calls = _fake_propagate_attributes_factory()
    monkeypatch.setattr(tracing, "propagate_attributes", fake_propagate)
    monkeypatch.setattr(tracing, "callback_handler", lambda: FAKE_CALLBACK_HANDLER)
    return calls


class _FakeBoundModel:
    def __init__(self, respond: Callable[[list[Any]], AIMessage]) -> None:
        self._respond = respond
        self.calls: list[tuple[list[Any], dict[str, Any] | None]] = []

    async def ainvoke(self, messages: list[Any], config: dict[str, Any] | None = None) -> AIMessage:
        self.calls.append((messages, config))
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
        usage_metadata={"input_tokens": 100, "output_tokens": 20, "total_tokens": 120},
    )


class _FakePublisher:
    def __init__(self) -> None:
        self.published: list[tuple[dict[str, Any], dict[str, Any]]] = []

    async def publish(self, message: dict[str, Any], **kwargs: Any) -> None:
        self.published.append((message, kwargs))


FULL_SNAPSHOT = {
    "job_id": "job_42",
    "title": "Senior Backend Engineer",
    "company": "Acme Corp",
    "repost_count": 1,
    "days_open": 10,
    "cross_board_count": 0,
    "always_hiring_count": 1,
}

VALID_RESULT = {
    "score": 20.0,
    "confidence": 0.9,
    "explanation": "Signals look normal.",
    "topSignals": ["repost count is low"],
}


def _envelope(**overrides: Any) -> Envelope:
    defaults: dict[str, Any] = dict(
        event_id="evt_1",
        event_type="ghost.requested",
        schema_version=1,
        occurred_at=datetime.now(UTC),
        work_id="job_42",
        correlation_id="corr_99",
        idempotency_key="ghost:job_42:corr_99",
        run_id="run_77",
        activity_id=None,
        trace_id=None,
    )
    defaults.update(overrides)
    return Envelope(**defaults)


def _work(**overrides: Any) -> GhostWork:
    defaults: dict[str, Any] = dict(
        jobId="job_42",
        activityId=None,
        snapshot=FULL_SNAPSHOT,
        snapshot_hash="sha256:abc",
    )
    defaults.update(overrides)
    return GhostWork(**defaults)


def test_successful_run_produces_one_trace_with_one_span_per_model_call_step(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fake_client = _FakeLangfuseClient(trace_id="trace_abc")
    _patch_tracing(monkeypatch, fake_client)

    fake_model = _FakeChatModel(lambda _msgs: _ai_message(VALID_RESULT))
    monkeypatch.setattr(ghost.gateway, "chat_model", lambda task_key: fake_model)

    publisher = _FakePublisher()
    asyncio.run(consumers._handle_ghost(_envelope(), _work(), publisher))  # type: ignore[arg-type]

    assert len(fake_client.observations) == 1
    assert fake_client.observations[0]["name"] == "ghost"
    assert fake_client.observations[0]["as_type"] == "span"

    assert fake_model.bound is not None
    assert len(fake_model.bound.calls) == 1
    _messages, config = fake_model.bound.calls[0]
    assert config == {"callbacks": [FAKE_CALLBACK_HANDLER]}
    assert fake_model.bind_kwargs is not None
    assert fake_model.bind_kwargs["extra_body"] == tracing.gateway_call_metadata("trace_abc")

    message, _kwargs = publisher.published[0]
    assert message["status"] == "succeeded"
    assert message["trace_id"] == "trace_abc"
    assert message["usage"] == {"input_tokens": 100, "output_tokens": 20}

    span = fake_client.observations[0]["span"]
    assert span.updates == []


def test_failed_run_produces_one_trace_marked_failed_naming_the_step_after_retries(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fake_client = _FakeLangfuseClient(trace_id="trace_failed")
    _patch_tracing(monkeypatch, fake_client)

    fake_model = _FakeChatModel(lambda _msgs: AIMessage(content="still not json"))
    monkeypatch.setattr(ghost.gateway, "chat_model", lambda task_key: fake_model)

    publisher = _FakePublisher()
    asyncio.run(consumers._handle_ghost(_envelope(), _work(), publisher))  # type: ignore[arg-type]

    assert len(fake_client.observations) == 1
    span = fake_client.observations[0]["span"]

    assert fake_model.bound is not None
    assert len(fake_model.bound.calls) == ghost.MAX_EXTRA_ATTEMPTS + 1

    assert len(span.updates) == 1
    update = span.updates[0]
    assert update["level"] == "ERROR"
    assert update["metadata"] == {"failed_step": "ghost"}
    assert update["status_message"].startswith("ghost:")

    message, _kwargs = publisher.published[0]
    assert message["status"] == "failed"
    assert message["trace_id"] == "trace_failed"
    assert message["failure"]["category"] == "internal"
    assert message["failure"]["failed_step"] == "ghost"


def test_failed_run_from_a_classified_provider_error_marks_the_trace_failed(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """A gateway-level failure (E5, failures.py's `classify_provider_error`,
    already built) is classified before it reaches tracing, and still
    produces exactly one failed trace naming the step (FR-013)."""
    fake_client = _FakeLangfuseClient(trace_id="trace_rate_limited")
    _patch_tracing(monkeypatch, fake_client)

    request = openai.APIConnectionError(request=None)  # type: ignore[call-arg]

    def respond(_msgs: list[Any]) -> AIMessage:
        raise request

    fake_model = _FakeChatModel(respond)
    monkeypatch.setattr(ghost.gateway, "chat_model", lambda task_key: fake_model)

    publisher = _FakePublisher()
    asyncio.run(consumers._handle_ghost(_envelope(), _work(), publisher))  # type: ignore[arg-type]

    assert len(fake_client.observations) == 1
    span = fake_client.observations[0]["span"]
    assert span.updates[0]["level"] == "ERROR"
    assert span.updates[0]["metadata"] == {"failed_step": "ghost"}

    message, _kwargs = publisher.published[0]
    assert message["status"] == "failed"
    assert message["failure"]["category"] == "provider_unavailable"
    assert message["failure"]["retryable"] is True


def test_trace_metadata_carries_user_job_and_capability_for_findability(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fake_client = _FakeLangfuseClient(trace_id="trace_findable")
    propagate_calls = _patch_tracing(monkeypatch, fake_client)

    fake_model = _FakeChatModel(lambda _msgs: _ai_message(VALID_RESULT))
    monkeypatch.setattr(ghost.gateway, "chat_model", lambda task_key: fake_model)

    envelope = _envelope(work_id="job_42", run_id="run_77", correlation_id="corr_99")
    work = _work(snapshot_hash="sha256:findme")
    publisher = _FakePublisher()

    asyncio.run(consumers._handle_ghost(envelope, work, publisher))  # type: ignore[arg-type]

    assert len(propagate_calls) == 1
    call = propagate_calls[0]
    assert call["trace_name"] == "ghost"
    assert call["user_id"] == "job_42"
    assert call["session_id"] == "job_42"
    assert call["tags"] == ["ghost", "ghost"]

    metadata = call["metadata"]
    assert metadata["work_type"] == "ghost"
    assert metadata["run_id"] == "run_77"
    assert metadata["correlation_id"] == "corr_99"
    assert metadata["task_key"] == "ghost"
    assert metadata["snapshot_hash"] == "sha256:findme"
    assert "workflow_version" in metadata

    message, _kwargs = publisher.published[0]
    assert message["trace_id"] == "trace_findable"
    assert message["work_id"] == "job_42"
    assert message["run_id"] == "run_77"
    assert message["correlation_id"] == "corr_99"


def test_run_completes_fast_even_though_starting_a_trace_never_touches_the_network(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Starting an observation (the root trace span) is a local, synchronous
    operation on the real SDK too — only export talks to the collector, and
    only from a background thread (research R8) — so a stopped collector
    cannot add latency to the run itself."""
    fake_client = _FakeLangfuseClient(trace_id="trace_fast")
    _patch_tracing(monkeypatch, fake_client)

    fake_model = _FakeChatModel(lambda _msgs: _ai_message(VALID_RESULT))
    monkeypatch.setattr(ghost.gateway, "chat_model", lambda task_key: fake_model)

    publisher = _FakePublisher()
    started = time.perf_counter()
    asyncio.run(consumers._handle_ghost(_envelope(), _work(), publisher))  # type: ignore[arg-type]
    elapsed = time.perf_counter() - started

    assert elapsed < 1.0
    assert publisher.published[0][0]["status"] == "succeeded"


def test_shutdown_does_not_hang_or_raise_when_the_collector_is_unreachable(
    caplog: pytest.LogCaptureFixture,
) -> None:
    """Exercises `log_collector_error_once`/`_run_bounded` directly: an
    unreachable collector makes the underlying client's flush raise, and
    `tracing.shutdown()` must swallow it (log once) rather than raise or
    hang past its bound (FR-016)."""

    class _RaisingClient:
        def shutdown(self) -> None:
            raise ConnectionError("collector unreachable")

    tracing._client = _RaisingClient()  # type: ignore[assignment]

    with caplog.at_level(logging.WARNING):
        started = time.perf_counter()
        tracing.shutdown(timeout=1.0)
        elapsed = time.perf_counter() - started

    assert elapsed < 1.0
    assert tracing._collector_error_logged is True
    assert any("collector error" in record.message for record in caplog.records)


def test_shutdown_is_bounded_when_the_collector_hangs(
    caplog: pytest.LogCaptureFixture,
) -> None:
    """A collector that never responds must not hang process shutdown —
    `shutdown()` returns at its timeout bound regardless (FR-016)."""

    class _HangingClient:
        def shutdown(self) -> None:
            time.sleep(5.0)

    tracing._client = _HangingClient()  # type: ignore[assignment]

    with caplog.at_level(logging.WARNING):
        started = time.perf_counter()
        tracing.shutdown(timeout=0.2)
        elapsed = time.perf_counter() - started

    assert elapsed < 1.0
    assert any("exceeded" in record.message for record in caplog.records)
