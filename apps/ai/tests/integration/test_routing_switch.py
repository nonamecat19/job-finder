"""US4 scenario 1: a partially migrated system serves both sets of
capabilities correctly, and cutting one capability over never disturbs
another's registration or behavior.

H1-2/T131 (unit, `tests/unit/test_main.py`) already proves an event-transport
capability 404s through `/invoke`. This test cross-checks that fact through
the real ASGI app (FastAPI's TestClient, not a direct function call) and adds
the "rest of the system is unaffected" angle: invoking one HTTP-transport
capability, an event-transport capability's rejection, and the same
HTTP-transport capability again, in sequence, all behave identically —
nothing about serving one capability changes another's outcome.
"""

from __future__ import annotations

from typing import Any

import pytest
from fastapi.testclient import TestClient

import jobfinder_ai.main as main
from jobfinder_ai.capabilities.single import rephrase
from jobfinder_ai.contracts.usage import Usage

AUTH_HEADERS = {"Authorization": f"Bearer {main.settings.ai_service_token}"}


@pytest.fixture(name="client")
def _client() -> TestClient:
    return TestClient(main.app)


def _rephrase_payload() -> dict[str, Any]:
    return {
        "input": {"term": "Kafka", "source_bullet": "Built a pipeline."},
        "context": {"user_id": "usr_1", "work_id": "job_1"},
    }


def test_http_transport_capability_invokes_normally_through_the_real_app(
    client: TestClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    async def fake_run(
        data: rephrase.RephraseInput, *, trace_id: str | None = None
    ) -> tuple[rephrase.RephraseResult, Usage]:
        return rephrase.RephraseResult(rephrase="Built a Kafka pipeline."), Usage(
            input_tokens=10, output_tokens=5
        )

    monkeypatch.setattr(rephrase, "run", fake_run)

    response = client.post(
        "/v1/capabilities/rephrase/invoke", json=_rephrase_payload(), headers=AUTH_HEADERS
    )

    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "succeeded"
    assert body["result"] == {"rephrase": "Built a Kafka pipeline."}


def test_event_transport_capability_404s_through_the_real_app(client: TestClient) -> None:
    """Cross-checks H1-2/T131 through the ASGI transport rather than a direct call."""
    response = client.post(
        "/v1/capabilities/ghost/invoke",
        json={"input": {}, "context": {"user_id": "usr_1", "work_id": "job_1"}},
        headers=AUTH_HEADERS,
    )

    assert response.status_code == 404


def test_switching_between_capabilities_does_not_disturb_either(
    client: TestClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    """US4 scenario 1: HTTP-transport and event-transport capabilities are
    interleaved on the same running app; neither's outcome depends on the
    other having just been called."""

    async def fake_run(
        data: rephrase.RephraseInput, *, trace_id: str | None = None
    ) -> tuple[rephrase.RephraseResult, Usage]:
        return rephrase.RephraseResult(rephrase="Built a Kafka pipeline."), Usage()

    monkeypatch.setattr(rephrase, "run", fake_run)

    first = client.post(
        "/v1/capabilities/rephrase/invoke", json=_rephrase_payload(), headers=AUTH_HEADERS
    )
    assert first.status_code == 200

    blocked = client.post(
        "/v1/capabilities/ghost/invoke",
        json={"input": {}, "context": {"user_id": "usr_1", "work_id": "job_1"}},
        headers=AUTH_HEADERS,
    )
    assert blocked.status_code == 404

    second = client.post(
        "/v1/capabilities/rephrase/invoke", json=_rephrase_payload(), headers=AUTH_HEADERS
    )
    assert second.status_code == 200
    assert second.json()["result"] == {"rephrase": "Built a Kafka pipeline."}
