"""Tests for the interactive HTTP surface's shared-secret auth (H7-1, H7-2,
T109) and its failure-category-to-status-code mapping (H4-2, H4-3, H4-4,
T110)."""

from __future__ import annotations

import asyncio
from typing import Any

import pytest
from fastapi import HTTPException

import jobfinder_ai.main as main
from jobfinder_ai.capabilities.single import outreach, recruiter, rephrase
from jobfinder_ai.contracts.usage import Usage
from jobfinder_ai.failures import CapabilityError


def _request(input_body: dict[str, Any]) -> main.InvokeRequest:
    return main.InvokeRequest(
        input=input_body, context=main.InvokeContext(user_id="usr_1", work_id="job_1")
    )


def test_require_shared_secret_accepts_the_configured_token() -> None:
    main.require_shared_secret(authorization=f"Bearer {main.settings.ai_service_token}")


def test_require_shared_secret_rejects_a_missing_header() -> None:
    with pytest.raises(HTTPException) as exc_info:
        main.require_shared_secret(authorization=None)
    assert exc_info.value.status_code == 401


def test_require_shared_secret_rejects_the_wrong_token() -> None:
    with pytest.raises(HTTPException) as exc_info:
        main.require_shared_secret(authorization="Bearer not-the-secret")
    assert exc_info.value.status_code == 401


def test_require_shared_secret_never_accepts_a_bare_token_without_bearer_scheme() -> None:
    with pytest.raises(HTTPException) as exc_info:
        main.require_shared_secret(authorization=main.settings.ai_service_token)
    assert exc_info.value.status_code == 401


def test_invoke_succeeds_returns_200_with_trace_id_and_usage(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    async def fake_run(
        data: rephrase.RephraseInput, *, trace_id: str | None = None
    ) -> tuple[rephrase.RephraseResult, Usage]:
        return rephrase.RephraseResult(rephrase="Built a Kafka pipeline."), Usage(
            input_tokens=10, output_tokens=5
        )

    monkeypatch.setattr(rephrase, "run", fake_run)

    response = asyncio.run(
        main.invoke_capability(
            "rephrase",
            _request({"term": "Kafka", "source_bullet": "Built a pipeline."}),
        )
    )

    assert response.status_code == 200
    import json

    body = json.loads(response.body)
    assert body["status"] == "succeeded"
    assert body["result"] == {"rephrase": "Built a Kafka pipeline."}
    assert body["failure"] is None
    assert "trace_id" in body
    assert body["usage"]["input_tokens"] == 10


def test_invoke_with_malformed_input_returns_422_and_a_trace_id(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    response = asyncio.run(main.invoke_capability("rephrase", _request({"term": "Kafka"})))

    assert response.status_code == 422
    import json

    body = json.loads(response.body)
    assert body["status"] == "failed"
    assert body["failure"]["category"] == "invalid_input"
    assert "trace_id" in body
    assert body["result"] is None


@pytest.mark.parametrize(
    ("category", "expected_status"),
    [
        ("invalid_input", 422),
        ("rate_limited", 429),
        ("provider_unavailable", 503),
        ("model_unavailable", 503),
        ("timeout", 504),
        ("internal", 500),
        ("bound_exceeded", 500),
    ],
)
def test_invoke_maps_failure_category_to_http_status(
    category: str, expected_status: int, monkeypatch: pytest.MonkeyPatch
) -> None:
    async def fake_run(
        data: rephrase.RephraseInput, *, trace_id: str | None = None
    ) -> tuple[rephrase.RephraseResult, Usage]:
        raise CapabilityError(category, "boom", failed_step="rephrase")

    monkeypatch.setattr(rephrase, "run", fake_run)

    response = asyncio.run(
        main.invoke_capability(
            "rephrase", _request({"term": "Kafka", "source_bullet": "Built a pipeline."})
        )
    )

    assert response.status_code == expected_status
    import json

    body = json.loads(response.body)
    assert body["status"] == "failed"
    assert body["failure"]["category"] == category
    assert "trace_id" in body
    assert body["result"] is None


def test_invoke_rate_limited_sets_retry_after_header(monkeypatch: pytest.MonkeyPatch) -> None:
    async def fake_run(
        data: rephrase.RephraseInput, *, trace_id: str | None = None
    ) -> tuple[rephrase.RephraseResult, Usage]:
        raise CapabilityError("rate_limited", "too many requests", failed_step="rephrase")

    monkeypatch.setattr(rephrase, "run", fake_run)

    response = asyncio.run(
        main.invoke_capability(
            "rephrase", _request({"term": "Kafka", "source_bullet": "Built a pipeline."})
        )
    )

    assert response.headers["Retry-After"] == str(main.RATE_LIMITED_RETRY_AFTER_SECONDS)


def test_failure_body_never_carries_profile_or_posting_content(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """H4-4: a failure body must not leak the input it failed on."""

    async def fake_run(
        data: recruiter.RecruiterInput, *, trace_id: str | None = None
    ) -> tuple[recruiter.RecruiterResult, Usage]:
        raise CapabilityError("internal", "structured output failed", failed_step="recruiter")

    monkeypatch.setattr(recruiter, "run", fake_run)

    secret_text = "SECRET-CANDIDATE-PROFILE-CONTENT"
    response = asyncio.run(
        main.invoke_capability("recruiter", _request({"source": "posting", "text": secret_text}))
    )

    assert secret_text not in response.body.decode()


def test_invoke_routes_to_the_right_capability_module(monkeypatch: pytest.MonkeyPatch) -> None:
    called: dict[str, Any] = {}

    async def fake_run(
        data: outreach.OutreachInput, *, trace_id: str | None = None
    ) -> tuple[outreach.OutreachResult, Usage]:
        called["tone"] = data.tone
        return outreach.OutreachResult(text="hi", specific_claims=[]), Usage()

    monkeypatch.setattr(outreach, "run", fake_run)

    response = asyncio.run(main.invoke_capability("outreach", _request({"tone": "direct"})))

    assert response.status_code == 200
    assert called["tone"] == "direct"
