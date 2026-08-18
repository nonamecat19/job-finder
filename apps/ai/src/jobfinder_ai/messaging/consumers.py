"""FastStream consumers for AI work events (T064, research R4/R9).

`register(router)` attaches every queued capability's subscriber and result
publisher to the shared `RabbitRouter` `main.py` owns — this module does not
construct its own broker, so there is exactly one connection and one place
health checks (`/health/ready`) look at (research R9).

Topology (exchange/queue names, routing keys, arguments) mirrors
`apps/api/internal/events/topology.go`'s `DeclareTopology` exactly — same
`jobfinder.work` exchange, `work.<work_type>` quorum queue with its DLX
arguments, and `jobfinder.results` topic exchange routed by `event_type`
(data-model.md § 4) — so this side's declaration is idempotent against the
Go side's rather than conflicting with it (M1-1, M1-3).

**Wire shape**: confirmed against `apps/api/internal/ghostjob/application/
publish.go` (T065) and `apps/api/internal/events/results.go` (T061) — a work
or result message body is the envelope's fields merged flat with the
payload's fields into one JSON object (Go builds this by embedding
`events.Envelope` and `events.GhostWork`/a private `resultMessage` struct
together), **not** `{"envelope": {...}, "payload": {...}}` nesting. This
module parses/builds that same flat shape by splitting a plain `dict` on the
generated `Envelope`/`GhostWork`/`Result` models' own field names, rather
than reusing those `extra="forbid"` models to validate the whole flattened
body (E1, E2, data-model.md § 1, § 3).
"""

from __future__ import annotations

import logging
from datetime import UTC, datetime
from typing import Any
from uuid import uuid4

from faststream.rabbit import ExchangeType, RabbitExchange, RabbitQueue
from faststream.rabbit.fastapi import RabbitRouter
from faststream.rabbit.publisher.usecase import RabbitPublisher
from faststream.rabbit.schemas.queue import QueueType

from jobfinder_ai import tracing
from jobfinder_ai.capabilities.single import ghost
from jobfinder_ai.contracts.envelope import Envelope
from jobfinder_ai.contracts.ghost_work import GhostWork
from jobfinder_ai.contracts.result import Failure, Usage
from jobfinder_ai.failures import CapabilityError

logger = logging.getLogger(__name__)

WORK_EXCHANGE = RabbitExchange("jobfinder.work", type=ExchangeType.DIRECT, durable=True)
RESULTS_EXCHANGE = RabbitExchange("jobfinder.results", type=ExchangeType.TOPIC, durable=True)

GHOST_WORK_QUEUE = RabbitQueue(
    "work.ghost",
    queue_type=QueueType.QUORUM,
    durable=True,
    routing_key="ghost",
    arguments={
        "x-dead-letter-exchange": "jobfinder.dlx",
        "x-dead-letter-routing-key": "ghost",
    },
)

_ENVELOPE_FIELDS = frozenset(Envelope.model_fields)
_GHOST_WORK_FIELDS = frozenset(GhostWork.model_fields)


def _split_ghost_requested(body: dict[str, Any]) -> tuple[Envelope, GhostWork]:
    """Splits a flat `ghost.requested` body into its envelope and payload
    halves by field name — the inverse of the Go publisher's struct
    embedding, since neither generated model accepts the other's fields
    (`extra="forbid"`, C3-1)."""
    envelope = Envelope.model_validate({k: v for k, v in body.items() if k in _ENVELOPE_FIELDS})
    work = GhostWork.model_validate(
        {k: v for k, v in body.items() if k in _GHOST_WORK_FIELDS}
    )
    return envelope, work


def register(router: RabbitRouter) -> None:
    """Attaches the `ghost` consumer (and its result publisher) to `router`.
    Called once from `main.py` at import time, alongside capability
    registration (C1-4)."""
    ghost_results = router.publisher(exchange=RESULTS_EXCHANGE, routing_key="ghost.completed")

    @router.subscriber(GHOST_WORK_QUEUE, WORK_EXCHANGE)
    async def handle_ghost_requested(body: dict[str, Any]) -> None:
        envelope, work = _split_ghost_requested(body)
        await _handle_ghost(envelope, work, ghost_results)


async def _handle_ghost(envelope: Envelope, work: GhostWork, publisher: RabbitPublisher) -> None:
    """Runs the `ghost` capability for one `ghost.requested` delivery and
    publishes `ghost.completed`. Never raises for a classified failure — a
    failed run still produces a result with `status=failed` (E4-1) — so the
    only way this function raises is an unclassified bug, which nacks the
    delivery for redelivery rather than silently losing the failure (M3-2
    only acks after the publish below is awaited and confirmed)."""
    with tracing.run_trace(
        name="ghost",
        user_id=envelope.work_id,
        session_id=envelope.work_id,
        metadata={
            "work_type": "ghost",
            "run_id": envelope.run_id,
            "correlation_id": envelope.correlation_id,
            "task_key": ghost.TASK_KEY,
            "workflow_version": tracing.resolve_workflow_version(),
            "snapshot_hash": work.snapshot_hash,
        },
        tags=["ghost", "ghost"],
    ) as span:
        trace_id = tracing.bootstrap().get_current_trace_id()
        try:
            result, usage = await ghost.run(work, trace_id=trace_id)
        except CapabilityError as exc:
            tracing.mark_run_failed(span, failed_step=exc.failed_step or "ghost", error=exc.message)
            outgoing = _result_message(
                envelope,
                trace_id=trace_id,
                snapshot_hash=work.snapshot_hash,
                status="failed",
                result=None,
                failure=Failure(
                    category=exc.category,
                    retryable=exc.retryable,
                    message=exc.message,
                    failed_step=exc.failed_step,
                ),
                usage=None,
            )
        else:
            outgoing = _result_message(
                envelope,
                trace_id=trace_id,
                snapshot_hash=work.snapshot_hash,
                status="succeeded",
                result=result.model_dump(),
                failure=None,
                usage=Usage(
                    input_tokens=usage.input_tokens,
                    output_tokens=usage.output_tokens,
                    cost_usd=usage.cost_usd,
                ),
            )

    # M3-2: awaited (and, via publisher confirms, acknowledged by the
    # broker) before this handler returns — only then does FastStream's
    # default ack-after-handler-returns behaviour ack the `ghost.requested`
    # delivery.
    await publisher.publish(outgoing, correlation_id=envelope.correlation_id)


def _result_message(
    request_envelope: Envelope,
    *,
    trace_id: str | None,
    snapshot_hash: str,
    status: str,
    result: dict[str, Any] | None,
    failure: Failure | None,
    usage: Usage | None,
) -> dict[str, Any]:
    """Builds the flat `ghost.completed` body: a fresh envelope (echoing
    `correlation_id`, `idempotency_key`, `run_id`, `work_id` and
    `activity_id` from the request per E1-3/M5-2/M5-3) merged with the
    result fields — mirroring Go's `resultMessage` (events/results.go),
    which has no separate `result.trace_id`; the envelope's `trace_id` is
    the wire's only one (E1-2)."""
    message: dict[str, Any] = {
        "event_id": str(uuid4()),
        "event_type": "ghost.completed",
        "schema_version": 1,
        "occurred_at": datetime.now(UTC).isoformat(),
        "work_id": request_envelope.work_id,
        "correlation_id": request_envelope.correlation_id,
        "idempotency_key": request_envelope.idempotency_key,
        "run_id": request_envelope.run_id,
        "activity_id": request_envelope.activity_id,
        "trace_id": trace_id,
        "status": status,
        "snapshot_hash": snapshot_hash,
    }
    if result is not None:
        message["result"] = result
    if failure is not None:
        message["failure"] = failure.model_dump(exclude_none=True)
    if usage is not None:
        message["usage"] = usage.model_dump(exclude_none=True)
    return message
