from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock

import pytest
from fastapi import HTTPException

import jobfinder_ai.main as main
from jobfinder_ai.capabilities.registry import Capability, CapabilityBounds


def test_health_live_never_touches_a_dependency() -> None:
    assert asyncio.run(main.health_live()) == {"status": "ok"}


def test_health_ready_reports_ok_when_broker_connects(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(main.broker_router.broker, "ping", AsyncMock(return_value=True))
    result = asyncio.run(main.health_ready())
    assert result["status"] == "ok"


def test_health_ready_503s_when_broker_is_unreachable(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(main.broker_router.broker, "ping", AsyncMock(return_value=False))
    with pytest.raises(HTTPException) as exc_info:
        asyncio.run(main.health_ready())
    assert exc_info.value.status_code == 503


def test_health_ready_never_issues_a_model_call(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(main.broker_router.broker, "ping", AsyncMock(return_value=True))

    def _fail_if_called(*args: object, **kwargs: object) -> None:
        raise AssertionError("health_ready must not construct a gateway client")

    monkeypatch.setattr("jobfinder_ai.gateway.chat_model", _fail_if_called)
    monkeypatch.setattr("jobfinder_ai.gateway.embeddings_model", _fail_if_called)
    asyncio.run(main.health_ready())


def test_invoke_unknown_capability_is_404() -> None:
    request = main.InvokeRequest(
        input={}, context=main.InvokeContext(user_id="usr_1", work_id="job_1")
    )
    with pytest.raises(HTTPException) as exc_info:
        asyncio.run(main.invoke_capability("does-not-exist", request))
    assert exc_info.value.status_code == 404


def test_invoke_event_transport_capability_is_404() -> None:
    """H1-2/T131: a queued capability is unreachable through /invoke."""
    capability = Capability(
        name="test-only-event-capability",
        task_key="generation-analyze",
        layer="chain",
        input_model="jobfinder_ai.main:InvokeContext",
        output_model="jobfinder_ai.main:InvokeContext",
        bounds=CapabilityBounds(max_model_calls=1),
        prompt_module="jobfinder_ai.capabilities",
        transport="event",
    )
    main.registry.register(capability)
    try:
        request = main.InvokeRequest(
            input={}, context=main.InvokeContext(user_id="usr_1", work_id="job_1")
        )
        with pytest.raises(HTTPException) as exc_info:
            asyncio.run(main.invoke_capability("test-only-event-capability", request))
        assert exc_info.value.status_code == 404
    finally:
        del main.registry._capabilities["test-only-event-capability"]
        del main.registry._task_keys_used["generation-analyze"]
