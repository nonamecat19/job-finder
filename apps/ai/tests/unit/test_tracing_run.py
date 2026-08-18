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
