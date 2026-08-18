"""FastStream consumers for AI work events (T064, research R4/R9).

`register(router)` attaches every queued capability's subscriber and result
publisher to the shared `RabbitRouter` `main.py` owns — this module does not
construct its own broker, so there is exactly one connection and one place
health checks (`/health/ready`) look at (research R9).

Topology (exchange/queue names, routing keys, arguments) mirrors
`apps/api/internal/events/topology.go`'s `DeclareTopology` exactly — same
`jobfinder.work` exchange, `work.<work_type>` quorum queue with its DLX
arguments, and `jobfinder.results` topic exchange routed by `event_type`
(data-model.md § 4). Every declaration here is PASSIVE (`declare=False`): this
side asserts the topology the Go side owns rather than creating any of it,
which is what M1-1 reserves to the publisher and what the AI service's own
broker account permits — it is granted `"configure":"^$"`, so an active
declaration is refused and the service never finishes starting. The
consequence is an ordering requirement: the backend must have declared the
topology before this service connects, and `main.py` waits for that rather
than exiting on the first attempt (M1-1, M1-3).

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

import asyncio
import logging
import time
from datetime import UTC, datetime
from typing import Any
from uuid import uuid4

import aio_pika
from faststream.rabbit import ExchangeType, RabbitExchange, RabbitQueue
from faststream.rabbit.fastapi import RabbitRouter
from faststream.rabbit.publisher.usecase import RabbitPublisher
from faststream.rabbit.schemas.queue import QueueType

from jobfinder_ai import tracing
from jobfinder_ai.capabilities.graphs import generation, salary
from jobfinder_ai.capabilities.single import ghost, match
from jobfinder_ai.contracts.envelope import Envelope
from jobfinder_ai.contracts.generate_work import GenerateWork
from jobfinder_ai.contracts.ghost_work import GhostWork
from jobfinder_ai.contracts.match_work import MatchWork
from jobfinder_ai.contracts.result import Failure, Usage
from jobfinder_ai.contracts.salary_work import SalaryWork
from jobfinder_ai.failures import CapabilityError

logger = logging.getLogger(__name__)

# `declare=False` throughout this module, on every exchange and every queue.
# FastStream turns that into a PASSIVE declaration: it asserts the object
# already exists and never creates one, which is what M1-1 means by reserving
# topology to the publisher. It is also what makes the service able to run at
# all under the account docker/rabbitmq/init-ai-user.sh creates — that account
# is granted `"configure":"^$"`, so an active declaration is refused by the
# broker and the service exits during startup.
WORK_EXCHANGE = RabbitExchange(
    "jobfinder.work", type=ExchangeType.DIRECT, durable=True, declare=False
)
RESULTS_EXCHANGE = RabbitExchange(
    "jobfinder.results", type=ExchangeType.TOPIC, durable=True, declare=False
)

GHOST_WORK_QUEUE = RabbitQueue(
    "work.ghost",
    queue_type=QueueType.QUORUM,
    durable=True,
    declare=False,
    routing_key="ghost",
    arguments={
        "x-dead-letter-exchange": "jobfinder.dlx",
        "x-dead-letter-routing-key": "ghost",
    },
)

GENERATE_WORK_QUEUE = RabbitQueue(
    "work.generate",
    queue_type=QueueType.QUORUM,
    durable=True,
    declare=False,
    routing_key="generate",
    arguments={
        "x-dead-letter-exchange": "jobfinder.dlx",
        "x-dead-letter-routing-key": "generate",
    },
)

MATCH_WORK_QUEUE = RabbitQueue(
    "work.match",
    queue_type=QueueType.QUORUM,
    durable=True,
    declare=False,
    routing_key="match",
    arguments={
        "x-dead-letter-exchange": "jobfinder.dlx",
        "x-dead-letter-routing-key": "match",
    },
)

SALARY_WORK_QUEUE = RabbitQueue(
    "work.salary",
    queue_type=QueueType.QUORUM,
    durable=True,
    declare=False,
    routing_key="salary",
    arguments={
        "x-dead-letter-exchange": "jobfinder.dlx",
        "x-dead-letter-routing-key": "salary",
    },
)

# The queue wait_for_topology probes. Any of the four would do — the backend
# declares them in one call — and this one is consumed by the capability that
# is routed to this service first (ghost, 048).
_TOPOLOGY_PROBE_QUEUE = "work.ghost"


async def wait_for_topology(
    rabbitmq_url: str,
    *,
    timeout: float = 120.0,
    poll_interval: float = 2.0,
) -> None:
    """Block until the backend has declared `jobfinder.work`, or give up.

    Every declaration in this module is passive (see the module docstring), so
    this service can consume only topology that already exists — and it has no
    permission to create any if it does not. Compose starts this service as
    soon as the broker is healthy, which says nothing about whether the
    backend has run `DeclareTopology` yet, so on a cold stack the first
    connection attempt legitimately finds nothing there.

    Waiting is the difference between "the backend has not started yet", which
    resolves on its own in seconds, and "the topology is wrong", which does
    not. Exiting immediately would make the first indistinguishable from the
    second and turn every cold start into a crash-loop.

    The probe is a passive declare of a WORK QUEUE, not of the work exchange,
    and that detail is load-bearing. A passive declare is not
    permission-exempt: RabbitMQ still requires the user to hold configure,
    write or read on the resource. This service's account holds none of the
    three on `jobfinder.work` (`configure "^$"`, `write "^jobfinder\.results$"`,
    `read "^work\.(match|generate|salary|ghost)$"`), so probing the exchange
    is refused permanently — indistinguishable, to a retry loop, from a
    backend that never starts. The work queues are inside the read regex by
    design, are created by the same `DeclareTopology` call, and are what this
    service actually consumes, so their presence is both a permitted and a
    stricter signal.

    Raises TimeoutError once `timeout` elapses, which is a real failure: at
    that point the backend is not coming up and this service should not
    pretend to be ready.
    """
    deadline = time.monotonic() + timeout
    attempt = 0
    last_error: Exception | None = None

    while True:
        attempt += 1
        try:
            connection = await aio_pika.connect_robust(rabbitmq_url)
        except Exception as exc:  # noqa: BLE001 — broker not up yet is one of many
            last_error = exc
        else:
            try:
                channel = await connection.channel()
                # Passive: asserts the queue and creates nothing, the same
                # shape the subscribers use once they connect.
                await channel.declare_queue(_TOPOLOGY_PROBE_QUEUE, passive=True)
                logger.info(
                    "messaging: topology present after %d attempt(s); starting consumers", attempt
                )
                return
            except Exception as exc:  # noqa: BLE001 — absent topology is a channel error
                last_error = exc
            finally:
                await connection.close()

        if time.monotonic() >= deadline:
            raise TimeoutError(
                f"messaging: queue {_TOPOLOGY_PROBE_QUEUE!r} was not declared within "
                f"{timeout:.0f}s ({attempt} attempts); the backend declares the topology "
                f"this service consumes (M1-1). Last error: {last_error}"
            )
        logger.warning(
            "messaging: topology not declared yet (attempt %d): %s; retrying in %.0fs",
            attempt,
            last_error,
            poll_interval,
        )
        await asyncio.sleep(poll_interval)


_ENVELOPE_FIELDS = frozenset(Envelope.model_fields)
_GHOST_WORK_FIELDS = frozenset(GhostWork.model_fields)
_GENERATE_WORK_FIELDS = frozenset(GenerateWork.model_fields)
_SALARY_WORK_FIELDS = frozenset(SalaryWork.model_fields)
_MATCH_WORK_FIELDS = frozenset(MatchWork.model_fields)


def _split_ghost_requested(body: dict[str, Any]) -> tuple[Envelope, GhostWork]:
    """Splits a flat `ghost.requested` body into its envelope and payload
    halves by field name — the inverse of the Go publisher's struct
    embedding, since neither generated model accepts the other's fields
    (`extra="forbid"`, C3-1)."""
    envelope = Envelope.model_validate({k: v for k, v in body.items() if k in _ENVELOPE_FIELDS})
    work = GhostWork.model_validate({k: v for k, v in body.items() if k in _GHOST_WORK_FIELDS})
    return envelope, work


def _split_generate_requested(body: dict[str, Any]) -> tuple[Envelope, GenerateWork]:
    """`_split_ghost_requested`'s counterpart for `generate.requested`."""
    envelope = Envelope.model_validate({k: v for k, v in body.items() if k in _ENVELOPE_FIELDS})
    work = GenerateWork.model_validate(
        {k: v for k, v in body.items() if k in _GENERATE_WORK_FIELDS}
    )
    return envelope, work


def _split_salary_requested(body: dict[str, Any]) -> tuple[Envelope, SalaryWork]:
    """`_split_ghost_requested`'s counterpart for `salary.requested`."""
    envelope = Envelope.model_validate({k: v for k, v in body.items() if k in _ENVELOPE_FIELDS})
    work = SalaryWork.model_validate({k: v for k, v in body.items() if k in _SALARY_WORK_FIELDS})
    return envelope, work


def _split_match_requested(body: dict[str, Any]) -> tuple[Envelope, MatchWork]:
    """`_split_ghost_requested`'s counterpart for `match.requested`."""
    envelope = Envelope.model_validate({k: v for k, v in body.items() if k in _ENVELOPE_FIELDS})
    work = MatchWork.model_validate({k: v for k, v in body.items() if k in _MATCH_WORK_FIELDS})
    return envelope, work


def register(router: RabbitRouter) -> None:
    """Attaches the `ghost`, `match`, `generate` and `salary` consumers (and
    their result publishers) to `router`. Called once from `main.py` at
    import time, alongside capability registration (C1-4)."""
    ghost_results = router.publisher(exchange=RESULTS_EXCHANGE, routing_key="ghost.completed")

    @router.subscriber(GHOST_WORK_QUEUE, WORK_EXCHANGE)
    async def handle_ghost_requested(body: dict[str, Any]) -> None:
        envelope, work = _split_ghost_requested(body)
        await _handle_ghost(envelope, work, ghost_results)

    match_results = router.publisher(exchange=RESULTS_EXCHANGE, routing_key="match.completed")

    @router.subscriber(MATCH_WORK_QUEUE, WORK_EXCHANGE)
    async def handle_match_requested(body: dict[str, Any]) -> None:
        envelope, work = _split_match_requested(body)
        await _handle_match(envelope, work, match_results)

    generate_results = router.publisher(exchange=RESULTS_EXCHANGE, routing_key="generate.completed")

    @router.subscriber(GENERATE_WORK_QUEUE, WORK_EXCHANGE)
    async def handle_generate_requested(body: dict[str, Any]) -> None:
        envelope, work = _split_generate_requested(body)
        await _handle_generate(envelope, work, generate_results)

    salary_results = router.publisher(exchange=RESULTS_EXCHANGE, routing_key="salary.completed")

    @router.subscriber(SALARY_WORK_QUEUE, WORK_EXCHANGE)
    async def handle_salary_requested(body: dict[str, Any]) -> None:
        envelope, work = _split_salary_requested(body)
        await _handle_salary(envelope, work, salary_results)


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


async def _handle_match(envelope: Envelope, work: MatchWork, publisher: RabbitPublisher) -> None:
    """`_handle_ghost`'s exact pattern for `match.requested`: run the
    `match` capability and publish `match.completed`, acking only after the
    publish is awaited and confirmed (M3-2)."""
    with tracing.run_trace(
        name="match",
        user_id=envelope.work_id,
        session_id=envelope.work_id,
        metadata={
            "work_type": "match",
            "run_id": envelope.run_id,
            "correlation_id": envelope.correlation_id,
            "task_key": match.TASK_KEY,
            "workflow_version": tracing.resolve_workflow_version(),
            "snapshot_hash": work.snapshot_hash,
        },
        tags=["match", "match"],
    ) as span:
        trace_id = tracing.bootstrap().get_current_trace_id()
        try:
            result, usage = await match.run(work, trace_id=trace_id)
        except CapabilityError as exc:
            tracing.mark_run_failed(span, failed_step=exc.failed_step or "match", error=exc.message)
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
                event_type="match.completed",
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
                event_type="match.completed",
            )

    await publisher.publish(outgoing, correlation_id=envelope.correlation_id)


async def _handle_generate(
    envelope: Envelope, work: GenerateWork, publisher: RabbitPublisher
) -> None:
    """`_handle_ghost`'s exact pattern for `generate.requested`: run the
    `generation` graph, publish `generate.completed`, and only ack after
    the publish is awaited and confirmed (M3-2). A stage failure — including
    a bound hit — names its own stage on the trace and on the result's
    `failed_step` (E5-4, FR-040, US3 scenario 1) via
    `generation.node`'s own span, not this function."""
    with tracing.run_trace(
        name="generation",
        user_id=envelope.work_id,
        session_id=envelope.work_id,
        metadata={
            "work_type": "generate",
            "run_id": envelope.run_id,
            "correlation_id": envelope.correlation_id,
            "task_key": generation.CAPABILITY.task_key,
            "workflow_version": tracing.resolve_workflow_version(),
            "snapshot_hash": work.snapshot_hash,
            **generation.trace_bounds_metadata(),
        },
        tags=["generation", "generate"],
    ) as span:
        trace_id = tracing.bootstrap().get_current_trace_id()
        try:
            result, usage = await generation.run(work, trace_id=trace_id)
        except CapabilityError as exc:
            tracing.mark_run_failed(
                span, failed_step=exc.failed_step or "generation", error=exc.message
            )
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
                event_type="generate.completed",
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
                event_type="generate.completed",
            )

    await publisher.publish(outgoing, correlation_id=envelope.correlation_id)


async def _handle_salary(envelope: Envelope, work: SalaryWork, publisher: RabbitPublisher) -> None:
    """`_handle_ghost`'s exact pattern for `salary.requested`: run the
    `salary` tool-call loop, publish `salary.completed`, and only ack after
    the publish is awaited and confirmed (M3-2)."""
    with tracing.run_trace(
        name="salary",
        user_id=envelope.work_id,
        session_id=envelope.work_id,
        metadata={
            "work_type": "salary",
            "run_id": envelope.run_id,
            "correlation_id": envelope.correlation_id,
            "task_key": salary.TASK_KEY,
            "workflow_version": tracing.resolve_workflow_version(),
            "snapshot_hash": work.snapshot_hash,
            "max_tool_rounds": salary.MAX_TOOL_ROUNDS,
            "max_nodes": salary.MAX_NODES,
        },
        tags=["salary", "salary"],
    ) as span:
        trace_id = tracing.bootstrap().get_current_trace_id()
        try:
            result, usage = await salary.run(work, trace_id=trace_id)
        except CapabilityError as exc:
            tracing.mark_run_failed(
                span, failed_step=exc.failed_step or "salary", error=exc.message
            )
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
                event_type="salary.completed",
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
                event_type="salary.completed",
            )

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
    event_type: str = "ghost.completed",
) -> dict[str, Any]:
    """Builds the flat `<event_type>` body: a fresh envelope (echoing
    `correlation_id`, `idempotency_key`, `run_id`, `work_id` and
    `activity_id` from the request per E1-3/M5-2/M5-3) merged with the
    result fields — mirroring Go's `resultMessage` (events/results.go),
    which has no separate `result.trace_id`; the envelope's `trace_id` is
    the wire's only one (E1-2)."""
    message: dict[str, Any] = {
        "event_id": str(uuid4()),
        "event_type": event_type,
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
