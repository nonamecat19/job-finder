from __future__ import annotations

import logging
import time

import pytest

import jobfinder_ai.tracing as tracing
from jobfinder_ai.settings import Settings

SETTINGS = Settings(
    gateway_url="http://litellm:4000",
    litellm_master_key="sk-test",
    rabbitmq_url="amqp://guest:guest@localhost:5672/",
    ai_service_token="secret",
)


def setup_function() -> None:
    tracing._client = None
    tracing._collector_error_logged = False


def teardown_function() -> None:
    tracing.shutdown()
    tracing._collector_error_logged = False


def test_bootstrap_is_idempotent() -> None:
    first = tracing.bootstrap(settings=SETTINGS)
    second = tracing.bootstrap(settings=SETTINGS)
    assert first is second


def test_log_collector_error_once_only_logs_the_first_call(
    caplog: pytest.LogCaptureFixture,
) -> None:
    caplog.set_level(logging.WARNING, logger="jobfinder_ai.tracing")
    tracing.log_collector_error_once(RuntimeError("boom"))
    tracing.log_collector_error_once(RuntimeError("boom again"))
    warnings = [r for r in caplog.records if r.levelno == logging.WARNING]
    assert len(warnings) == 1


def test_run_bounded_never_blocks_past_the_bound() -> None:
    def never_returns() -> None:
        time.sleep(10)

    started = time.monotonic()
    tracing._run_bounded(never_returns, timeout=0.2)
    elapsed = time.monotonic() - started
    assert elapsed < 2.0


def test_shutdown_with_no_client_is_a_no_op() -> None:
    tracing._client = None
    tracing.shutdown()
