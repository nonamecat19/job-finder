"""The FastStream consumers against a real RabbitMQ broker (M1, M3, M5).

`tests/unit/test_consumers.py` calls the handler functions directly and
`tests/integration/test_tracing.py` drives them with a fake publisher, so
everything the broker itself decides is currently unproven on this side:

* whether the topology `messaging/consumers.py` asserts is *compatible* with
  the one `apps/api/internal/events/topology.go` declares, rather than merely
  spelled the same in two files that drifted apart. RabbitMQ answers this by
  refusing a mismatched passive declare with NOT_FOUND, so the test declares
  the Go side's topology verbatim and then lets the real consumer attach;
* that the consumer creates no topology of its own (M1-1). Every declaration
  in `consumers.py` is passive (`declare=False`), which is not a stylistic
  choice: the `ai_service` broker account is granted `"configure":"^$"`, so
  an active declaration is refused and the service never starts. Against an
  empty broker the consumers must therefore fail loudly and leave the broker
  exactly as empty as they found it — and `wait_for_topology` must be what
  turns "the backend has not declared it *yet*" into a wait instead of a
  crash-loop;
* whether a work message published in the flat wire shape the Go publisher
  emits is actually routed to, decoded by and consumed by the subscriber,
  and whether the result comes back out on `jobfinder.results` under the
  routing key and flat shape `events.resultMessage` parses (M5, E2);
* what happens to a delivery whose handler raised. FastStream's default
  `AckPolicy.REJECT_ON_ERROR` rejects without requeue, which on these queues
  means the broker dead-letters it via `x-dead-letter-exchange` — the
  message is neither lost nor spun forever. That is a claim about queue
  arguments and broker behaviour, so only a broker can prove it.

The LLM boundary is stubbed throughout (`ghost.run`) and so is Langfuse:
this suite is about the broker, and nothing here should reach a gateway, a
provider or a collector.
"""

from __future__ import annotations

import asyncio
import json
from collections.abc import Callable, Generator
from contextlib import contextmanager
from datetime import UTC, datetime
from typing import Any
from uuid import uuid4

import aio_pika
import pytest
from aio_pika.abc import AbstractChannel, AbstractConnection, AbstractIncomingMessage
from aio_pika.exceptions import ChannelNotFoundEntity
from faststream.rabbit.fastapi import RabbitRouter

from jobfinder_ai import tracing
from jobfinder_ai.capabilities.single import ghost
from jobfinder_ai.contracts.usage import Usage
from jobfinder_ai.failures import CapabilityError
from jobfinder_ai.messaging import consumers

# These tests need a Docker daemon. They fail — never skip — without one.
pytestmark = pytest.mark.docker

MESSAGE_TIMEOUT_SECONDS = 20.0

# ---------------------------------------------------------------------------
# The Go side's topology, mirrored from apps/api/internal/events/topology.go
# verbatim (names, types, durability, arguments, bindings). This is the
# fixture under test as much as the consumer is: if `topology.go` changes and
# this copy does not, the compatibility tests below stop meaning anything —
# but they also start failing, because the real consumer would then be
# attaching to a topology the Go side no longer declares.
# ---------------------------------------------------------------------------

WORK_EXCHANGE = "jobfinder.work"
DELAY_EXCHANGE = "jobfinder.delay"
RESULT_EXCHANGE = "jobfinder.results"
DLX = "jobfinder.dlx"
RESULT_QUEUE = "results.backend"

WORK_TYPES = ("ingest", "enrich", "match", "generate", "salary", "ghost")

RETRY_RUNGS: tuple[tuple[str, int], ...] = (
    ("1s", 1000),
    ("10s", 10_000),
    ("1m", 60_000),
    ("10m", 600_000),
)

# The four work types this service actually subscribes to (consumers.py).
CONSUMED_WORK_TYPES = ("ghost", "match", "generate", "salary")


async def _declare_go_topology(channel: AbstractChannel) -> None:
    """`events.DeclareTopology`, port for port."""
    await channel.declare_exchange(WORK_EXCHANGE, aio_pika.ExchangeType.DIRECT, durable=True)
    await channel.declare_exchange(DELAY_EXCHANGE, aio_pika.ExchangeType.DIRECT, durable=True)
    await channel.declare_exchange(RESULT_EXCHANGE, aio_pika.ExchangeType.TOPIC, durable=True)
    await channel.declare_exchange(DLX, aio_pika.ExchangeType.DIRECT, durable=True)

    quorum: dict[str, Any] = {"x-queue-type": "quorum"}

    for work_type in WORK_TYPES:
        work_queue = await channel.declare_queue(
            f"work.{work_type}",
            durable=True,
            arguments={
                "x-queue-type": "quorum",
                "x-dead-letter-exchange": DLX,
                "x-dead-letter-routing-key": work_type,
            },
        )
        await work_queue.bind(WORK_EXCHANGE, routing_key=work_type)

        for rung, ttl in RETRY_RUNGS:
            delay_queue = await channel.declare_queue(
                f"delay.{work_type}.{rung}",
                durable=True,
                arguments={
                    "x-queue-type": "quorum",
                    "x-message-ttl": ttl,
                    "x-dead-letter-exchange": WORK_EXCHANGE,
                    "x-dead-letter-routing-key": work_type,
                },
            )
            await delay_queue.bind(DELAY_EXCHANGE, routing_key=f"{work_type}.{rung}")

        dlq = await channel.declare_queue(f"dlq.{work_type}", durable=True, arguments=quorum)
        await dlq.bind(DLX, routing_key=work_type)

    result_queue = await channel.declare_queue(RESULT_QUEUE, durable=True, arguments=quorum)
    await result_queue.bind(RESULT_EXCHANGE, routing_key="*.completed")


# ---------------------------------------------------------------------------
# Stubs for the two boundaries this suite is not about: the LLM call and the
# Langfuse collector. Both follow the pattern established in
# tests/integration/test_tracing.py.
# ---------------------------------------------------------------------------


class _FakeSpan:
    def __init__(self) -> None:
        self.updates: list[dict[str, Any]] = []

    def update(self, **kwargs: Any) -> None:
        self.updates.append(kwargs)


class _FakeObservation:
    def __enter__(self) -> _FakeSpan:
        return _FakeSpan()

    def __exit__(self, *exc: object) -> bool:
        return False


class _FakeLangfuseClient:
    def start_as_current_observation(self, *, name: str, as_type: str) -> _FakeObservation:
        return _FakeObservation()

    def get_current_trace_id(self) -> str:
        return "trace_broker_test"


def _stub_tracing(monkeypatch: pytest.MonkeyPatch) -> None:
    """No collector, no OTel baggage — nothing in this suite leaves the
    broker connection."""
    monkeypatch.setattr(tracing, "bootstrap", lambda **_kwargs: _FakeLangfuseClient())

    @contextmanager
    def _fake_propagate(**_kwargs: Any) -> Generator[None]:
        yield

    monkeypatch.setattr(tracing, "propagate_attributes", _fake_propagate)
    monkeypatch.setattr(tracing, "callback_handler", lambda: None)


def _stub_ghost_success(monkeypatch: pytest.MonkeyPatch) -> None:
    async def fake_run(
        work: ghost.GhostWork, *, trace_id: str | None = None
    ) -> tuple[ghost.GhostResult, Usage]:
        return (
            ghost.GhostResult(
                score=20.0,
                confidence=0.9,
                explanation="Signals look normal.",
                topSignals=["repost count is low"],
            ),
            Usage(input_tokens=100, output_tokens=20),
        )

    monkeypatch.setattr(ghost, "run", fake_run)


def _stub_ghost_classified_failure(monkeypatch: pytest.MonkeyPatch) -> None:
    async def fake_run(
        work: ghost.GhostWork, *, trace_id: str | None = None
    ) -> tuple[ghost.GhostResult, Usage]:
        raise CapabilityError(
            category="provider_unavailable",
            message="ghost: gateway unreachable",
            failed_step="ghost",
        )

    monkeypatch.setattr(ghost, "run", fake_run)


# ---------------------------------------------------------------------------
# Wire shapes, built the way the Go publisher builds them: the envelope's
# fields merged flat with the payload's, never nested (E2, consumers.py's
# module docstring).
# ---------------------------------------------------------------------------


def _ghost_requested_body(**overrides: Any) -> dict[str, Any]:
    body: dict[str, Any] = {
        "event_id": str(uuid4()),
        "event_type": "ghost.requested",
        "schema_version": 1,
        "occurred_at": datetime.now(UTC).isoformat(),
        "work_id": "job_42",
        "correlation_id": "corr_99",
        "idempotency_key": "ghost:job_42:corr_99",
        "run_id": "run_77",
        "activity_id": None,
        "trace_id": None,
        "jobId": "job_42",
        "activityId": None,
        "snapshot": {
            "job_id": "job_42",
            "title": "Senior Backend Engineer",
            "company": "Acme Corp",
            "repost_count": 1,
            "days_open": 10,
            "cross_board_count": 0,
            "always_hiring_count": 1,
        },
        "snapshot_hash": "sha256:abc",
    }
    body.update(overrides)
    return body


# ---------------------------------------------------------------------------
# Broker plumbing.
# ---------------------------------------------------------------------------


async def _publish_work(channel: AbstractChannel, body: dict[str, Any], routing_key: str) -> None:
    exchange = await channel.get_exchange(WORK_EXCHANGE)
    await exchange.publish(
        aio_pika.Message(
            body=json.dumps(body).encode(),
            content_type="application/json",
            delivery_mode=aio_pika.DeliveryMode.PERSISTENT,
            correlation_id=body["correlation_id"],
        ),
        routing_key=routing_key,
    )


async def _await_message(
    channel: AbstractChannel, queue_name: str, *, timeout: float = MESSAGE_TIMEOUT_SECONDS
) -> AbstractIncomingMessage:
    queue = await channel.get_queue(queue_name)
    loop = asyncio.get_running_loop()
    deadline = loop.time() + timeout
    while loop.time() < deadline:
        message = await queue.get(no_ack=True, fail=False)
        if message is not None:
            return message
        await asyncio.sleep(0.1)
    raise AssertionError(f"no message arrived on {queue_name} within {timeout}s")


async def _queue_depth(channel: AbstractChannel, queue_name: str) -> int:
    queue = await channel.declare_queue(queue_name, passive=True)
    return int(queue.declaration_result.message_count or 0)


async def _consumer_count(channel: AbstractChannel, queue_name: str) -> int:
    queue = await channel.declare_queue(queue_name, passive=True)
    return int(queue.declaration_result.consumer_count or 0)


async def _start_consumers(url: str) -> Any:
    """Builds a router exactly the way `main.py` does — same
    `consumers.register`, same real subscribers and publishers — pointed at
    the test broker, and starts it."""
    router = RabbitRouter(url)
    consumers.register(router)
    broker = router.broker
    await broker.connect()
    await broker.start()
    return broker


async def _start_consumers_expecting_failure(url: str) -> tuple[BaseException | None, Any]:
    """`_start_consumers`, but returning whatever `start()` raised instead
    of propagating it, so the test can inspect the failure *and* go on to
    inspect the broker afterwards."""
    router = RabbitRouter(url)
    consumers.register(router)
    broker = router.broker
    failure: BaseException | None = None
    try:
        await broker.connect()
        await broker.start()
    except Exception as exc:  # noqa: BLE001 — the failure itself is the assertion
        failure = exc
    try:
        await broker.stop()
    except Exception:  # noqa: BLE001, S110 — a half-started broker may not stop cleanly
        pass
    return failure, broker


async def _exchange_exists(connection: AbstractConnection, name: str) -> bool:
    """A passive declare on a channel of its own — a NOT_FOUND closes the
    channel it was issued on, so one channel cannot answer two questions."""
    channel = await connection.channel()
    try:
        await channel.declare_exchange(
            name, aio_pika.ExchangeType.DIRECT, durable=True, passive=True
        )
    except ChannelNotFoundEntity:
        return False
    else:
        return True


async def _queue_exists(connection: AbstractConnection, name: str) -> bool:
    channel = await connection.channel()
    try:
        await channel.declare_queue(name, passive=True)
    except ChannelNotFoundEntity:
        return False
    else:
        return True


# ---------------------------------------------------------------------------
# M1-1: the Go side owns the topology; this side only ever asserts it.
# ---------------------------------------------------------------------------


def test_consumers_attach_to_the_topology_the_go_side_declared_first(broker_url: str) -> None:
    """The production order: the Go publisher declares topology at startup
    (M1-1), `wait_for_topology` sees it, and the consumers attach. Every
    declaration on this side is passive, so "attached cleanly" means the
    named exchanges and queues all exist as the Go side created them — a
    passive declare of anything absent is NOT_FOUND and takes the whole
    startup down (the test below).

    Passive declares assert existence, not arguments: the broker will not
    catch a drifted `arguments=` dict in `consumers.py`, which is why the
    management-API test below compares those explicitly.
    """

    async def scenario() -> dict[str, int]:
        connection = await aio_pika.connect_robust(broker_url)
        async with connection:
            channel = await connection.channel()
            await _declare_go_topology(channel)

        # Exactly what main.py's lifespan does before the broker starts.
        await consumers.wait_for_topology(broker_url, timeout=10.0, poll_interval=0.2)

        broker = await _start_consumers(broker_url)
        try:
            verify = await aio_pika.connect_robust(broker_url)
            async with verify:
                channel = await verify.channel()
                # Idempotent from the Go side's point of view, still.
                await _declare_go_topology(channel)
                return {
                    f"work.{work_type}": await _consumer_count(channel, f"work.{work_type}")
                    for work_type in CONSUMED_WORK_TYPES
                }
        finally:
            await broker.stop()

    consumer_counts = asyncio.run(scenario())

    # Attached to the Go-declared queues themselves, not to look-alikes.
    assert consumer_counts == {f"work.{work_type}": 1 for work_type in CONSUMED_WORK_TYPES}


def test_starting_the_consumers_against_an_empty_broker_creates_no_topology(
    broker_url: str,
) -> None:
    """The inverse of the test above, and the one that actually pins M1-1:
    on a broker where the backend has declared nothing, starting the
    consumers must not bring a single exchange or queue into existence.

    It must also not succeed. The observed behaviour (not an assumed one —
    this was run before it was asserted) is that `broker.start()` raises
    `aiormq.exceptions.ChannelNotFoundEntity`, a NOT_FOUND naming the first
    object of the topology it could not find. Which object that is depends
    on subscriber start order, so the assertion pins the error class and the
    NOT_FOUND, not the name.

    A missing object also kills the channel it was asserted on, which is why
    every existence check below uses a channel of its own.
    """

    async def scenario() -> tuple[BaseException | None, dict[str, bool]]:
        broker = await _start_consumers_expecting_failure(broker_url)
        failure, _broker = broker

        connection = await aio_pika.connect_robust(broker_url)
        async with connection:
            existence: dict[str, bool] = {}
            for exchange in (WORK_EXCHANGE, RESULT_EXCHANGE, DLX):
                existence[exchange] = await _exchange_exists(connection, exchange)
            for work_type in CONSUMED_WORK_TYPES:
                name = f"work.{work_type}"
                existence[name] = await _queue_exists(connection, name)
            existence[RESULT_QUEUE] = await _queue_exists(connection, RESULT_QUEUE)
        return failure, existence

    failure, existence = asyncio.run(scenario())

    # Loud, not silent: the service refuses to run without the topology.
    assert isinstance(failure, ChannelNotFoundEntity), f"expected NOT_FOUND, got {failure!r}"
    assert "NOT_FOUND" in str(failure)

    # And nothing at all was created on the way to failing.
    assert existence == dict.fromkeys(existence, False), (
        f"the consumers created topology they must never create: "
        f"{[name for name, exists in existence.items() if exists]}"
    )


def test_wait_for_topology_returns_once_the_go_side_has_declared_it(broker_url: str) -> None:
    """`main.py` awaits this before the broker lifespan, so its success path
    has to be prompt: once the backend's `DeclareTopology` has run, the very
    first poll must satisfy it rather than costing a `poll_interval`."""

    async def scenario() -> float:
        connection = await aio_pika.connect_robust(broker_url)
        async with connection:
            channel = await connection.channel()
            await _declare_go_topology(channel)

        loop = asyncio.get_running_loop()
        started = loop.time()
        await consumers.wait_for_topology(broker_url, timeout=10.0, poll_interval=5.0)
        return loop.time() - started

    # Well under one poll_interval: it returned on the first attempt.
    assert asyncio.run(scenario()) < 5.0


def test_wait_for_topology_times_out_when_the_backend_never_declares_anything(
    broker_url: str,
) -> None:
    """The failure path is a real failure, not an indefinite hang: waiting
    covers "the backend has not got there yet", and the timeout is where
    that stops being a plausible explanation. The message has to name what
    was missing, since it is all an operator gets from a container that
    would otherwise just be dead."""

    async def scenario() -> tuple[float, str]:
        loop = asyncio.get_running_loop()
        started = loop.time()
        try:
            await consumers.wait_for_topology(broker_url, timeout=1.0, poll_interval=0.2)
        except TimeoutError as exc:
            return loop.time() - started, str(exc)
        raise AssertionError("wait_for_topology returned even though no topology exists")

    elapsed, message = asyncio.run(scenario())

    assert elapsed >= 1.0  # it really waited out its deadline
    assert elapsed < 10.0  # and did not run away past it
    # The probe is a work queue, not the work exchange: a passive declare is
    # not permission-exempt, and the AI account holds no permission at all on
    # jobfinder.work, so probing it would be refused forever rather than
    # resolving when the backend declares.
    assert consumers._TOPOLOGY_PROBE_QUEUE in message
    assert "NOT_FOUND" in message


def test_consumer_queue_arguments_still_match_the_ones_the_go_side_created(
    broker_url: str, queue_arguments: Callable[[str], dict[str, Any]]
) -> None:
    """What went missing when the declarations became passive.

    An active declare made RabbitMQ itself compare `consumers.py`'s
    `arguments=` against the Go side's and refuse a mismatch. A passive one
    asserts existence only, so the constants in `consumers.py` — quorum
    type, DLX, dead-letter routing key — are no longer checked by anything
    at runtime, and could rot into a lie about the queues this service reads
    without a single test noticing. This reads the arguments RabbitMQ
    actually recorded for the Go-declared queues (management API) and
    compares them to what `consumers.py` claims.
    """
    queues = (
        consumers.GHOST_WORK_QUEUE,
        consumers.MATCH_WORK_QUEUE,
        consumers.GENERATE_WORK_QUEUE,
        consumers.SALARY_WORK_QUEUE,
    )

    async def scenario() -> None:
        connection = await aio_pika.connect_robust(broker_url)
        async with connection:
            channel = await connection.channel()
            await _declare_go_topology(channel)

    asyncio.run(scenario())

    for queue in queues:
        actual = queue_arguments(queue.name)
        # `RabbitQueue.arguments` already carries `x-queue-type` for a
        # `queue_type=QueueType.QUORUM` queue, alongside the DLX pair.
        expected = dict(queue.arguments or {})
        assert actual == expected, (
            f"{queue.name}: broker has {actual}, consumers.py claims {expected}"
        )


# ---------------------------------------------------------------------------
# M5/E2: a real work delivery in, a real result out.
# ---------------------------------------------------------------------------


def test_flat_work_message_is_consumed_and_a_result_reaches_the_backend_queue(
    broker_url: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    """End to end over the broker: a `ghost.requested` body in the flat wire
    shape the Go publisher emits is routed to `work.ghost`, decoded and
    handled, and the result is published to `jobfinder.results` under
    `ghost.completed` — which is what binds it to `results.backend`
    (`*.completed`). The body is asserted field by field against
    `events.resultMessage`'s JSON tags, since that struct is what parses it.
    """
    _stub_tracing(monkeypatch)
    _stub_ghost_success(monkeypatch)

    body = _ghost_requested_body()

    async def scenario() -> tuple[dict[str, Any], str, int]:
        connection = await aio_pika.connect_robust(broker_url)
        async with connection:
            channel = await connection.channel()
            await _declare_go_topology(channel)

            broker = await _start_consumers(broker_url)
            try:
                await _publish_work(channel, body, routing_key="ghost")
                message = await _await_message(channel, RESULT_QUEUE)
                payload = json.loads(message.body)
                assert isinstance(payload, dict)
                work_depth = await _queue_depth(channel, "work.ghost")
                return payload, message.routing_key or "", work_depth
            finally:
                await broker.stop()

    result, routing_key, work_depth = asyncio.run(scenario())

    assert routing_key == "ghost.completed"

    # Flat, not nested: no "envelope"/"payload" wrapper anywhere (E2).
    assert "envelope" not in result
    assert "payload" not in result

    assert result["event_type"] == "ghost.completed"
    assert result["schema_version"] == 1
    assert result["status"] == "succeeded"
    assert result["snapshot_hash"] == "sha256:abc"
    # Correlation identifiers echoed from the request (E1-3, M5-2, M5-3).
    assert result["work_id"] == body["work_id"]
    assert result["correlation_id"] == body["correlation_id"]
    assert result["idempotency_key"] == body["idempotency_key"]
    assert result["run_id"] == body["run_id"]
    assert result["trace_id"] == "trace_broker_test"
    assert result["result"] == {
        "score": 20.0,
        "confidence": 0.9,
        "explanation": "Signals look normal.",
        "topSignals": ["repost count is low"],
    }
    assert result["usage"] == {"input_tokens": 100, "output_tokens": 20}
    assert "failure" not in result

    # The work delivery was acked, not left unacknowledged or requeued.
    assert work_depth == 0


def test_classified_capability_failure_is_acked_and_reported_as_a_failed_result(
    broker_url: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    """A failure the capability classified (E5) is an outcome, not a broker
    error: the delivery is acked, nothing is dead-lettered, and the backend
    still gets a `ghost.completed` carrying `status=failed` (E4-1). Only a
    real broker can show the difference between this path and the redelivery
    path below.
    """
    _stub_tracing(monkeypatch)
    _stub_ghost_classified_failure(monkeypatch)

    async def scenario() -> tuple[dict[str, Any], int]:
        connection = await aio_pika.connect_robust(broker_url)
        async with connection:
            channel = await connection.channel()
            await _declare_go_topology(channel)

            broker = await _start_consumers(broker_url)
            try:
                await _publish_work(channel, _ghost_requested_body(), routing_key="ghost")
                message = await _await_message(channel, RESULT_QUEUE)
                payload = json.loads(message.body)
                assert isinstance(payload, dict)
                return payload, await _queue_depth(channel, "dlq.ghost")
            finally:
                await broker.stop()

    result, dlq_depth = asyncio.run(scenario())

    assert result["event_type"] == "ghost.completed"
    assert result["status"] == "failed"
    assert result["failure"]["category"] == "provider_unavailable"
    assert result["failure"]["retryable"] is True
    assert result["failure"]["failed_step"] == "ghost"
    assert "result" not in result
    assert dlq_depth == 0


# ---------------------------------------------------------------------------
# M3: a handler that raises never loses the message.
# ---------------------------------------------------------------------------


def test_a_raising_handler_dead_letters_the_delivery_instead_of_losing_it(
    broker_url: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    """A body the generated models reject (here: `snapshot_hash` missing)
    raises out of the handler, which is exactly the unclassified-bug path
    `_handle_ghost`'s docstring describes. FastStream's default
    `AckPolicy.REJECT_ON_ERROR` rejects without requeue; because
    `work.ghost` carries `x-dead-letter-exchange: jobfinder.dlx` and
    `x-dead-letter-routing-key: ghost`, the broker moves the delivery —
    unchanged — to `dlq.ghost`, where the Go side's operators can see it.

    The two assertions that matter are that the message is *somewhere*
    (never silently dropped) and that `work.ghost` is empty (never spinning
    through infinite redelivery against a body that can never parse).
    """
    _stub_tracing(monkeypatch)
    _stub_ghost_success(monkeypatch)

    body = _ghost_requested_body()
    del body["snapshot_hash"]

    async def scenario() -> tuple[dict[str, Any], int, int]:
        connection = await aio_pika.connect_robust(broker_url)
        async with connection:
            channel = await connection.channel()
            await _declare_go_topology(channel)

            broker = await _start_consumers(broker_url)
            try:
                await _publish_work(channel, body, routing_key="ghost")
                dead_lettered = await _await_message(channel, "dlq.ghost")
                payload = json.loads(dead_lettered.body)
                assert isinstance(payload, dict)
                # Give a redelivery loop, if there were one, a chance to show.
                await asyncio.sleep(1.0)
                return (
                    payload,
                    await _queue_depth(channel, "work.ghost"),
                    await _queue_depth(channel, RESULT_QUEUE),
                )
            finally:
                await broker.stop()

    dead_lettered, work_depth, result_depth = asyncio.run(scenario())

    assert dead_lettered == body  # byte-for-byte the message that was published
    assert work_depth == 0
    assert result_depth == 0  # no result was invented for a message never handled
