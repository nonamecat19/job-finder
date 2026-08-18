"""Langfuse bootstrap (FR-016, research R8).

The SDK batches and exports on its own background thread, so nothing here
issues a synchronous network call. Two things are this module's job on top of
the default client: a shutdown flush bounded to a short timeout (a stuck
collector must not hang process shutdown), and collector errors logged once
then otherwise ignored rather than spamming or raising into a run.
"""

from __future__ import annotations

import logging
import os
import subprocess
import threading
from collections.abc import Callable, Generator, Mapping
from contextlib import contextmanager
from functools import lru_cache
from typing import Any

from langfuse import Langfuse, propagate_attributes
from langfuse.langchain import CallbackHandler

from jobfinder_ai.settings import Settings, get_settings

logger = logging.getLogger(__name__)

SHUTDOWN_FLUSH_TIMEOUT_SECONDS: float = 5.0

_lock = threading.Lock()
_client: Langfuse | None = None
_collector_error_logged = False


def bootstrap(*, settings: Settings | None = None) -> Langfuse:
    """Create (or return) the process-wide Langfuse client. Idempotent."""
    global _client
    with _lock:
        if _client is not None:
            return _client
        settings = settings or get_settings()
        _client = Langfuse(
            public_key=settings.langfuse_public_key,
            secret_key=settings.langfuse_secret_key,
            base_url=settings.langfuse_host,
        )
        return _client


def callback_handler() -> CallbackHandler:
    """A LangChain callback handler bound to the bootstrapped client.

    Call `bootstrap()` before this during service startup; if it was never
    called, the handler falls back to the SDK's own default client.
    """
    return CallbackHandler()


def log_collector_error_once(exc: BaseException) -> None:
    """A collector error is logged the first time, then ignored (FR-016)."""
    global _collector_error_logged
    if _collector_error_logged:
        return
    logger.warning("langfuse collector error, tracing continues degraded: %s", exc)
    _collector_error_logged = True


def shutdown(*, timeout: float = SHUTDOWN_FLUSH_TIMEOUT_SECONDS) -> None:
    """Flush and close the client, bounded to `timeout` seconds.

    Runs the SDK's own shutdown on a background thread and does not wait past
    the bound — an unreachable collector must never hang process shutdown.
    """
    global _client
    if _client is None:
        return
    client, _client = _client, None
    _run_bounded(client.shutdown, timeout=timeout)


def resolve_workflow_version(*, env: Mapping[str, str] | None = None) -> str:
    """The orchestration service's committed revision (FR-015): resolved
    once, at process startup, never per request — before/after runs are
    distinguishable because prompts live in-repo (FR-015a), so a revision
    identifies exact prompt text.

    Prefers the `WORKFLOW_VERSION` environment variable, set at build/deploy
    time (the container image does not carry a `.git` directory), falling
    back to `git rev-parse HEAD` for local/dev runs, and finally to
    `"unknown"` rather than failing startup over a cosmetic trace field.

    Passing `env` explicitly (tests only) bypasses the process-wide cache;
    the no-argument production call path is cached, since the answer cannot
    change without a redeploy.
    """
    if env is not None:
        return _resolve_workflow_version(env)
    return _cached_workflow_version()


@lru_cache(maxsize=1)
def _cached_workflow_version() -> str:
    return _resolve_workflow_version(os.environ)


def _resolve_workflow_version(source: Mapping[str, str]) -> str:
    if version := source.get("WORKFLOW_VERSION"):
        return version
    try:
        result = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            capture_output=True,
            text=True,
            timeout=2.0,
            check=True,
        )
        return result.stdout.strip()
    except Exception:  # noqa: BLE001 - a missing .git must never fail startup
        return "unknown"


@contextmanager
def run_trace(
    *,
    name: str,
    user_id: str,
    session_id: str,
    metadata: Mapping[str, Any],
    tags: list[str],
) -> Generator[Any]:
    """One trace per run, shaped per data-model.md § 9 (FR-012, FR-014):
    `name` = capability name, `user_id` = the user the work ran for,
    `session_id` = `work_id` (every attempt for a unit of work groups
    together), `metadata` carries `work_type`, `run_id`, `correlation_id`,
    `task_key`, `workflow_version` and `snapshot_hash` (FR-015), `tags` =
    `[capability, work_type]`.

    Yields the root span so callers can end it explicitly on failure
    (T069) before the `with` block exits normally on success.
    """
    client = bootstrap()
    with client.start_as_current_observation(name=name, as_type="span") as span:
        with propagate_attributes(
            trace_name=name,
            user_id=user_id,
            session_id=session_id,
            metadata=dict(metadata),
            tags=tags,
        ):
            yield span


def mark_run_failed(span: Any, *, failed_step: str, error: str) -> None:
    """Marks the current run's trace failed, naming the failing step and its
    error (FR-013, US1 scenario 2) — called from inside a `run_trace` block
    on any classified failure, including one that exhausted every retry.
    """
    span.update(
        level="ERROR",
        status_message=f"{failed_step}: {error}",
        metadata={"failed_step": failed_step},
    )


def gateway_call_metadata(trace_id: str) -> dict[str, dict[str, str]]:
    """Request metadata for a gateway call, correlating LiteLLM's own
    Langfuse record with this run's trace (FR-017, research R8). Pass as
    `extra_body` on the chat model call — LiteLLM forwards the `metadata`
    body field into its `success_callback`/`failure_callback` Langfuse
    logger, which reads `trace_id` (and honours `existing_trace_id` to
    attach to an existing trace rather than starting a new one).
    """
    return {"metadata": {"trace_id": trace_id, "existing_trace_id": trace_id}}


def _run_bounded(fn: Callable[[], None], *, timeout: float) -> None:
    done = threading.Event()

    def run() -> None:
        try:
            fn()
        except Exception as exc:  # noqa: BLE001 - a collector failure must never propagate
            log_collector_error_once(exc)
        finally:
            done.set()

    thread = threading.Thread(target=run, daemon=True)
    thread.start()
    if not done.wait(timeout):
        logger.warning(
            "langfuse shutdown flush exceeded %.1fs, continuing without waiting", timeout
        )
