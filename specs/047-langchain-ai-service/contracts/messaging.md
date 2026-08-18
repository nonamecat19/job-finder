# Contract: Messaging

**Feature**: 047-langchain-ai-service | Binding on `apps/api/internal/events` and
`apps/ai/src/jobfinder_ai/messaging`.

## M1. Topology declaration

- **M1-1**: Every exchange, queue and binding in data-model.md § 4 MUST be declared by the
  publisher at startup, idempotently. A consumer MUST NOT rely on a queue another service
  declared first.
- **M1-2**: Every queue MUST be `durable` and of type `quorum`. A classic or transient queue is
  a defect — FR-033 requires survival of a broker restart.
- **M1-3**: Startup MUST fail loudly if a declaration conflicts with an existing object
  (mismatched type or arguments), naming the object. Silent fallback to the existing definition
  is forbidden — it is how a queue silently stays non-durable.

## M2. Publishing

- **M2-1**: Publishes MUST use publisher confirms and MUST NOT report success to the caller
  until the broker acknowledges (FR-034).
- **M2-2**: Messages MUST be published `persistent` (delivery mode 2).
- **M2-3**: A publish that is nacked, times out, or cannot reach the broker MUST return an
  error to its caller. Enqueue-and-forget is forbidden — an HTTP handler that cannot publish
  MUST return 5xx rather than 202.
- **M2-4**: Publishes MUST be `mandatory` with a returned-message handler; an unroutable
  message MUST be logged as an error and surfaced as a publish failure, never dropped silently.
- **M2-5**: The publisher MUST NOT hold a database transaction open across a publish. Persist
  first, publish after commit; a publish failure after a committed write is reconciled by the
  existing stale-activity sweep (FR-036), not by rolling back.

## M3. Consuming

- **M3-1**: Consumers MUST use manual acknowledgement. Auto-ack is forbidden — it loses work on
  a crash, contradicting FR-033.
- **M3-2**: Ack occurs **after** the unit of work is durably handled — for Go, after the result
  is persisted; for Python, after the result event is confirmed published. Never before.
- **M3-3**: Prefetch MUST equal the work type's configured concurrency, so an idle consumer
  does not hoard messages a peer could serve.
- **M3-4**: A consumer MUST reconnect automatically with bounded exponential backoff and MUST
  resume consuming without operator action (FR-035). Reconnection MUST re-declare topology
  (M1-1) and re-establish prefetch.
- **M3-5**: On shutdown a consumer MUST stop accepting deliveries, finish in-flight work within
  a bounded grace period, and `nack` with `requeue=true` anything it cannot finish.
- **M3-6**: `consumer_timeout` on the broker MUST exceed the longest work type's `MaxDuration`
  with margin. A generation run is minutes long; the RabbitMQ default would kill it.

## M4. Retry and dead-lettering

- **M4-1**: A retryable failure MUST republish to `jobfinder.delay` with `x-attempt`
  incremented, targeting the ladder rung matching the computed backoff, which dead-letters back
  to `jobfinder.work` (data-model.md § 4).
- **M4-2**: Backoff MUST follow the fixed ladder in data-model.md § 6 — `1s → 10s → 1m → 10m`,
  five attempts — with rate-limited failures entering at the `1m` rung. This is a deliberate
  reduction from asynq's inherited default of 25 attempts; the rationale is recorded in
  data-model.md § 6 and MUST appear in the migration notes.
- **M4-3**: A non-retryable failure (`invalid_input`, `credential_rejected`, unknown schema
  version) MUST go straight to the dead-letter queue without consuming retry budget.
- **M4-4**: When `x-attempt` reaches the work type's budget, the message MUST be published to
  `jobfinder.dlx` with `x-first-failure-reason` set (FR-031).
- **M4-5**: A message MUST NOT be retried indefinitely, and a failing message MUST NOT block
  messages behind it — retries leave the work queue, they do not sit at its head.
- **M4-6**: Dead-lettered messages MUST persist with no TTL until an operator acts (FR-032).
- **M4-7**: Re-dispatch from the DLQ MUST reset `x-attempt` to 0 and publish to
  `jobfinder.work`; the operator path for this is the management UI's shovel/move or a
  documented `make` target.

## M5. Ordering, duplication and supersession

- **M5-1**: Delivery is **at-least-once**. No consumer may assume exactly-once, and none may
  assume ordering across messages.
- **M5-2**: Every consumer MUST be idempotent per `idempotency_key` (FR-030). The backend
  enforces this at the write via the idempotency ledger (data-model.md § 7).
- **M5-3**: A result event whose `run_id` does not match the current run recorded for its
  `work_id` MUST be discarded without error, with a counter incremented (FR-037).
- **M5-4**: A result event whose `work_id` no longer exists MUST be discarded without error and
  counted (FR-037). It MUST NOT be dead-lettered — nothing is wrong with the message.

## M6. Schema handling

- **M6-1**: A consumer receiving an `x-schema-version` it does not implement MUST reject to the
  DLQ without deserializing the body (FR-029).
- **M6-2**: An unknown `event_type` MUST be rejected to the DLQ, never ignored.
- **M6-3**: Adding an optional field is a compatible change and MUST NOT bump
  `schema_version`. Removing a field, renaming one, or changing its meaning MUST bump it.

## M7. Security

- **M7-1**: RabbitMQ MUST require authentication; the default `guest/guest` account MUST be
  removed or have its password replaced in every environment including dev.
- **M7-2**: The broker's AMQP and management ports MUST NOT be published outside the stack in
  production (FR-038). The dev compose file may publish the management UI on loopback only.
- **M7-3**: Backend and orchestration service MUST use separate credentials with distinct
  permissions: the orchestration service can consume AI work queues and publish results, and
  nothing else — no access to `ingest`/`enrich` queues, no topology administration.
- **M7-4**: Message bodies carry profile and posting content. TLS is not required inside the
  compose network but MUST be used if the broker is ever reached across a network boundary.

## M8. Observability

- **M8-1**: Queue depth, consumer count, redelivery rate, DLQ depth and publish-confirm
  latency MUST be observable per work type.
- **M8-2**: A non-empty DLQ MUST be visible to an operator without inspecting the broker by
  hand — the health endpoint reports DLQ depth per work type.
- **M8-3**: Broker unavailability MUST be reported by the existing health surface, distinct
  from database or gateway unavailability.
