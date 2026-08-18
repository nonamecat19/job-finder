from __future__ import annotations

import asyncio
from datetime import UTC, datetime
from typing import Any

import pytest

from jobfinder_ai import tracing
from jobfinder_ai.capabilities.single import ghost
from jobfinder_ai.contracts.envelope import Envelope
from jobfinder_ai.contracts.ghost_work import GhostWork
from jobfinder_ai.contracts.usage import Usage
from jobfinder_ai.failures import CapabilityError
from jobfinder_ai.messaging import consumers


def setup_function() -> None:
    tracing._client = None


def teardown_function() -> None:
    tracing.shutdown()


class _FakePublisher:
    def __init__(self) -> None:
        self.published: list[tuple[dict[str, Any], dict[str, Any]]] = []

    async def publish(self, message: dict[str, Any], **kwargs: Any) -> None:
        self.published.append((message, kwargs))


def _envelope(**overrides: Any) -> Envelope:
    defaults: dict[str, Any] = dict(
        event_id="evt_1",
        event_type="ghost.requested",
        schema_version=1,
        occurred_at=datetime.now(UTC),
        work_id="job_1",
        correlation_id="corr_1",
        idempotency_key="ghost:job_1:corr_1",
        run_id="run_1",
        activity_id=None,
        trace_id=None,
    )
    defaults.update(overrides)
    return Envelope(**defaults)


def _work(**overrides: Any) -> GhostWork:
    defaults: dict[str, Any] = dict(
        jobId="job_1",
        activityId=None,
        snapshot={
            "job_id": "job_1",
            "title": "Backend Engineer",
            "company": "Acme",
            "repost_count": 1,
            "days_open": 5,
            "cross_board_count": 0,
            "always_hiring_count": 1,
        },
        snapshot_hash="sha256:abc",
    )
    defaults.update(overrides)
    return GhostWork(**defaults)


FLAT_REQUEST_BODY: dict[str, Any] = {
    "event_id": "evt_1",
    "event_type": "ghost.requested",
    "schema_version": 1,
    "occurred_at": "2026-08-18T10:00:00Z",
    "work_id": "job_1",
    "correlation_id": "corr_1",
    "idempotency_key": "ghost:job_1:corr_1",
    "run_id": "run_1",
    "activity_id": "act_1",
    "trace_id": None,
    "jobId": "job_1",
    "activityId": "act_1",
    "snapshot": {
        "job_id": "job_1",
        "title": "Backend Engineer",
        "company": "Acme",
        "repost_count": 1,
        "days_open": None,
        "cross_board_count": None,
        "always_hiring_count": None,
    },
    "snapshot_hash": "sha256:abc",
}


def test_split_ghost_requested_separates_envelope_and_payload_from_one_flat_body() -> None:
    envelope, work = consumers._split_ghost_requested(FLAT_REQUEST_BODY)

    assert envelope.event_id == "evt_1"
    assert envelope.work_id == "job_1"
    assert envelope.trace_id is None
    assert work.jobId == "job_1"
    assert work.snapshot_hash == "sha256:abc"
    assert work.snapshot["title"] == "Backend Engineer"


def test_handle_ghost_publishes_a_succeeded_result(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    async def fake_run(work: GhostWork, *, trace_id: str | None = None) -> Any:
        return (
            ghost.GhostResult(score=10.0, confidence=0.8, explanation="fine", topSignals=[]),
            Usage(input_tokens=50, output_tokens=10, cost_usd=None),
        )

    monkeypatch.setattr(ghost, "run", fake_run)

    envelope = _envelope()
    work = _work()
    publisher = _FakePublisher()

    asyncio.run(consumers._handle_ghost(envelope, work, publisher))  # type: ignore[arg-type]

    assert len(publisher.published) == 1
    message, kwargs = publisher.published[0]
    assert message["event_type"] == "ghost.completed"
    assert message["status"] == "succeeded"
    assert "failure" not in message
    assert message["result"] == {
        "score": 10.0,
        "confidence": 0.8,
        "explanation": "fine",
        "topSignals": [],
    }
    assert message["snapshot_hash"] == "sha256:abc"
    # E1-3/M5-2/M5-3: correlation_id, idempotency_key and run_id are echoed
    # from the request so the backend's idempotency ledger dedupes on the
    # same key and can detect a superseded run.
    assert message["correlation_id"] == "corr_1"
    assert message["idempotency_key"] == "ghost:job_1:corr_1"
    assert message["run_id"] == "run_1"
    assert message["work_id"] == "job_1"
    assert kwargs["correlation_id"] == "corr_1"


def test_handle_ghost_publishes_a_failed_result_on_capability_error(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    async def fake_run(work: GhostWork, *, trace_id: str | None = None) -> Any:
        raise CapabilityError("rate_limited", "gateway: rate limited", failed_step="ghost")

    monkeypatch.setattr(ghost, "run", fake_run)

    envelope = _envelope()
    work = _work()
    publisher = _FakePublisher()

    asyncio.run(consumers._handle_ghost(envelope, work, publisher))  # type: ignore[arg-type]

    assert len(publisher.published) == 1
    message, _kwargs = publisher.published[0]
    assert message["status"] == "failed"
    assert "result" not in message
    assert message["failure"] == {
        "category": "rate_limited",
        "retryable": True,
        "message": "gateway: rate limited",
        "failed_step": "ghost",
    }
