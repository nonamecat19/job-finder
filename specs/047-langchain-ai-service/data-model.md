# Phase 1 Data Model: Dedicated AI Orchestration Service

**Feature**: 047-langchain-ai-service | **Date**: 2026-08-18

Nothing here is a new database table. This feature adds **message** shapes and **trace**
shapes; the platform's persistent model is unchanged except for one idempotency ledger and the
removal of asynq's Redis keyspace. That is deliberate — FR-008 keeps the orchestration service
out of the database, so every durable entity stays where it already is, owned by Go.

---

## 1. Event envelope

Every message on the broker, work and result alike, carries the same envelope. Generated to
Python via R7; Go is the source of truth.

| Field | Type | Rules |
|---|---|---|
| `event_id` | UUID | Unique per published message. A redelivery keeps the same id — it is not a delivery id |
| `event_type` | string | `<domain>.<action>`, e.g. `match.requested`, `match.completed`. Enumerated; unknown → dead-letter (FR-029) |
| `schema_version` | int | Starts at 1. Consumer rejects a version it does not implement rather than best-effort parsing (FR-029) |
| `occurred_at` | RFC3339 | Set by the publisher at publish time |
| `work_id` | string | Stable identifier of the unit of work — the entity the work concerns (job id, generation run id) (FR-028) |
| `correlation_id` | UUID | Ties a result event to the work event that caused it. Copied verbatim onto the result (FR-028) |
| `idempotency_key` | string | Deterministic for a unit of work: `<work_type>:<work_id>:<run_discriminator>`. The dedupe key at the write (FR-030, R6) |
| `run_id` | UUID | Identifies this attempt. Distinct per retry, unlike `idempotency_key`. Used to discard superseded results (FR-037) |
| `activity_id` | string? | Existing activity-tracking id, carried through so the current heartbeat/stale-sweep machinery keeps working (FR-036) |
| `trace_id` | string? | Langfuse trace id, set by the orchestration service on the result so the backend can link a stored result to its trace (FR-014, FR-017) |

**Invariants**
- `correlation_id` on a result event MUST equal the `correlation_id` of its work event.
- `idempotency_key` MUST be derivable without random input, so a redelivery computes the same
  key.
- No envelope field carries profile or resume content — payloads do, envelopes do not, so logs
  of envelopes are safe to keep beyond the 30-day payload window.

---

## 2. Work events

One per work type. Payloads are the existing `internal/queue` payload structs, which survive
the migration unchanged (that is what makes behaviour comparable before and after).

| Event type | Payload (existing Go type) | Consumer after phase 3 |
|---|---|---|
| `ingest.requested` | `IngestPayload` | Go |
| `enrich.requested` | `EnrichPayload` | Go |
| `match.requested` | `MatchPayload` + profile/job snapshot | Python (`match`) |
| `generate.requested` | `GeneratePayload` + profile/job snapshot | Python (generation graph) |
| `salary.requested` | `SalaryInferPayload` + job snapshot | Python (`salary` loop) |
| `ghost.requested` | `GhostScorePayload` + job snapshot | Python (`ghost`) |

**The snapshot addition is the one substantive payload change.** Today a Go worker receives a
job id and reads the profile and posting from Postgres. FR-008 forbids the Python service from
doing that, so AI work events carry the data the capability needs. Consequences the plan must
carry:

- **Payload size.** A full profile plus a posting is kilobytes, not megabytes; well inside
  RabbitMQ's practical limits. A resume-generation event carrying a whole master profile is the
  largest case and must be measured, with a hard cap that fails the publish explicitly rather
  than producing an oversized message.
- **Staleness.** A snapshot is read at publish time. If the profile changes between publish and
  consume, the run uses the older data. Acceptable — it matches asynq's current behaviour for
  anything already in flight — but the result must record which snapshot it ran against.
- **Grounding (Constitution II).** The snapshot *is* the grounding source. Prompt assembly in
  Python must draw only from it, never from anything the service fetches itself.

---

## 3. Result events

| Field | Type | Notes |
|---|---|---|
| `status` | `succeeded` \| `failed` | No partial state; FR-003 forbids a partly-parsed success |
| `result` | capability-specific object | Present iff `succeeded`. Schema per capability, contracts/capabilities.md |
| `failure` | Failure (below) | Present iff `failed` |
| `trace_id` | string | The Langfuse trace for this run |
| `snapshot_hash` | string | Hash of the input snapshot the run consumed, so a stored result names its input |
| `usage` | Usage | Token counts and cost, mirrored from the gateway response for cheap querying without Langfuse |

### Failure

| Field | Type | Notes |
|---|---|---|
| `category` | enum | `rate_limited`, `credential_rejected`, `insufficient_credits`, `model_unavailable`, `provider_unavailable`, `invalid_input`, `bound_exceeded`, `timeout`, `internal` (FR-004) |
| `retryable` | bool | Carried, not re-derived by the consumer (FR-004) |
| `message` | string | Operator-facing. MUST NOT embed profile or posting content |
| `failed_step` | string? | The graph node or stage that failed (US3, FR-040) |

The category set is exactly today's sentinel taxonomy from
`internal/platform/llm/infrastructure/shared/errors.go` plus `invalid_input`, `bound_exceeded`
and `internal`. Preserving the names is what lets existing retry decisions carry over unchanged.

---

## 4. Messaging topology

| Object | Name pattern | Properties |
|---|---|---|
| Work exchange | `jobfinder.work` | Direct, durable |
| Work queue | `work.<work_type>` | Quorum, durable, DLX → `jobfinder.dlx` |
| Delay exchange | `jobfinder.delay` | Direct, durable; per-attempt TTL queues dead-letter back to the work exchange (R5) |
| Delay queue | `delay.<work_type>.<rung>` | Quorum, one per ladder rung (`1s`, `10s`, `1m`, `10m`), TTL = the rung, DLX → `jobfinder.work` |
| Result exchange | `jobfinder.results` | Topic, durable, routing key = `event_type` |
| Result queue | `results.backend` | Quorum, durable, consumed by Go only |
| Dead-letter exchange | `jobfinder.dlx` | Direct, durable |
| Dead-letter queue | `dlq.<work_type>` | Quorum, durable, no TTL — items persist until an operator acts (FR-031, FR-032) |

One delay queue per ladder rung rather than one shared delay queue: TTL expiry is evaluated only
at the queue head, so mixing TTLs in one queue head-of-line blocks (research R5). A fixed ladder
keeps that property while bounding the queue count at four per work type.

---

## 5. Message headers

| Header | Purpose |
|---|---|
| `x-attempt` | Retry count. Incremented on each republish; compared against the work type's budget |
| `x-work-type` | Routing and metrics without body parsing |
| `x-schema-version` | Lets a consumer reject early, before deserialization |
| `x-first-failure-reason` | Set when first dead-lettered, so the DLQ shows *why* without opening the body (FR-031) |

---

## 6. Retry budgets

Derived from today's `TaskPolicy` and asynq defaults; the values are the acceptance target, the
mechanism is new (research R5).

| Work type | Max attempts | Backoff ladder | Timeout source |
|---|---|---|---|
| `ingest` | 3 (`IngestMaxRetry` = 2 retries, unchanged) | 1s → 10s | `AITaskTimeoutIngest` |
| `enrich` | 5 | 1s → 10s → 1m → 10m | `AITaskTimeoutEnrich` |
| `match` | 5 | as above; rate-limited enters at 1m | `AITaskTimeoutMatch` |
| `generate` | 5 | as above | `AITaskTimeoutGenerate` |
| `salary` | 5 | as above | `AITaskTimeoutSalary` |
| `ghost` | 5 | as above | `AITaskTimeoutGhost` |

**These numbers are a deliberate correction, not a port.** asynq's default `MaxRetry` is **25**,
which is what five of these work types inherit today. Carried over literally it would mean up to
25 attempts against a paid LLM gateway for a request that has already exhausted its provider
chain, and — with a delay queue per attempt (§ 4) — roughly 150 queues to declare. Neither is
worth preserving. Five attempts across a four-rung ladder covers the failure that retries
actually fix (a transient provider outage or a rate-limit window) and stops well short of
burning budget on one that they do not.

**The ladder is fixed, not per-attempt.** Attempts map onto four delay queues per work type
(`delay.<work_type>.{1s,10s,1m,10m}`), so the topology is 24 delay queues in total rather than
one per attempt. An attempt beyond the ladder's length reuses its longest rung.

This is a behaviour change, and the one place where "preserve today's semantics" is knowingly
broken. It MUST be called out in the migration notes rather than discovered from a queue count.

`RateLimitRetryDelay` moves from `internal/queue/policy.go` into `internal/events/retry.go`
with its logic intact — same doubling, same ceiling — now computing a delay-queue TTL instead
of an asynq `RetryDelayFunc` return.

---

## 7. Idempotency ledger (the one new table)

| Column | Type | Notes |
|---|---|---|
| `idempotency_key` | text | Primary key |
| `work_type` | text | |
| `run_id` | uuid | The run whose result was accepted |
| `accepted_at` | timestamptz | |

Written in the same transaction as the result it admits, so a redelivered result event hits the
primary key and is discarded (FR-030). A result whose `run_id` differs from the row's is a
superseded result and is dropped with a counter incremented (FR-037).

Retention: rows older than the longest retry budget plus a margin are pruned — the ledger is a
short-horizon dedupe window, not an audit log.

Goose migration, sequential version number per the constitution's migration rule.

---

## 8. Capability contracts

Each capability declares a typed input and output, generated to Python from Go (R7). Full table
in contracts/capabilities.md; the shape is:

| Capability | Input | Output | Layer | Bounds |
|---|---|---|---|---|
| `match` | profile snapshot + job posting | score, reasons, matched/missing skills | LangChain | 1 call |
| `ghost` | job posting | score, signals | LangChain | 1 call |
| `salary` | job posting + tools | salary band | LangGraph loop | max tool rounds |
| generation | profile snapshot + posting + section config | resume/cover-letter sections | LangGraph graph | max nodes, per-stage timeout |

Bounds are configuration per capability, enforced by the runtime (FR-041), and are recorded in
the trace so an exceeded bound is visible as such rather than as a generic failure.

---

## 9. Trace structure

Not persisted by this platform — held by Langfuse — but its shape is a contract (FR-012 – FR-015).

```
trace (one per run)
├── name          = capability name
├── user_id       = the user the work ran for
├── session_id    = work_id (all attempts for a unit of work group together)
├── metadata      = { work_type, run_id, correlation_id, task_key,
│                     workflow_version = service git revision,
│                     snapshot_hash, activity_id }
├── tags          = [capability, work_type]
└── spans (one per orchestration step, FR-040)
    ├── input / output       ← purged at 30 days (FR-018)
    ├── model tier, tokens, cost, latency   ← retained indefinitely
    └── status + error       ← on failure, names the step (US3)
```

`workflow_version` resolves to the orchestration service's committed revision, which is what
makes FR-015 checkable: prompts live in-repo (FR-015a), so a revision identifies exact prompt
text.

---

## 10. What is removed

| Removed | Replaced by |
|---|---|
| asynq task/queue/server bindings | `internal/events` publisher + consumers |
| `asynqmon` service | RabbitMQ management UI (FR-032) |
| asynq's Redis keyspace | RabbitMQ durable queues (Redis stays, for caching only) |
| `internal/platform/llm/**` | `apps/ai` capability registry (end of phase 3) |
| `internal/platform/llm/application/toolloop` | LangGraph bounded loops |
| Per-domain prompt assembly in Go | `apps/ai/src/jobfinder_ai/prompts/` |
