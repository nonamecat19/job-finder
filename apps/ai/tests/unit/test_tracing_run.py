from __future__ import annotations

import subprocess

import pytest

from jobfinder_ai import tracing


def test_resolve_workflow_version_prefers_env() -> None:
    version = tracing.resolve_workflow_version(env={"WORKFLOW_VERSION": "abc123"})
    assert version == "abc123"


def test_resolve_workflow_version_falls_back_to_unknown(monkeypatch: pytest.MonkeyPatch) -> None:
    def _raise(*_args: object, **_kwargs: object) -> None:
        raise FileNotFoundError("git not found")

    monkeypatch.setattr(subprocess, "run", _raise)
    version = tracing.resolve_workflow_version(env={})
    assert version == "unknown"


def test_gateway_call_metadata_carries_trace_id() -> None:
    metadata = tracing.gateway_call_metadata("trace_abc")
    assert metadata == {"metadata": {"trace_id": "trace_abc", "existing_trace_id": "trace_abc"}}


def test_resolve_workflow_version_distinguishes_a_redeploy() -> None:
    """Two runs either side of a prompt/step change record different
    workflow_version values, since prompts live in-repo (FR-015a) and a
    revision identifies exact prompt text (FR-015)."""
    before = tracing.resolve_workflow_version(env={"WORKFLOW_VERSION": "rev-before-edit"})
    after = tracing.resolve_workflow_version(env={"WORKFLOW_VERSION": "rev-after-edit"})
    assert before == "rev-before-edit"
    assert after == "rev-after-edit"
    assert before != after
