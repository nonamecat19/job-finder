# Phase 0 Research: Dedicated AI Orchestration Service

**Feature**: 047-langchain-ai-service | **Date**: 2026-08-18

Version numbers below were resolved from PyPI, the Go module proxy and Docker Hub on
2026-08-18. They are the pins the plan proposes; re-resolve at implementation time and record
any drift in the lockfiles rather than here.

---

## R1. Framework layering — LangChain vs LangGraph per capability

**Decision**: `langchain` 1.3.15 for single-call capabilities; `langgraph` 1.2.11 for the
three multi-call capabilities. Both, layered — not a choice between them.

**Rationale**: LangChain's agent construction sits on the LangGraph runtime, so LangGraph is
already present transitively. What varies is whether a capability declares a graph. The spec
makes this mechanical: FR-039 forbids a graph for a single model call, FR-040 requires one
for every capability with more than one, and FR-041 requires bounds enforced by the runtime.
LangGraph's `recursion_limit` and per-node retry policy satisfy FR-041 without
application-level counting; a hand-rolled loop would re-implement exactly the machinery the
existing Go `toolloop` package already had to.

The mapping is fixed by spec.md § *Which layer serves which capability*:

| Layer | Capabilities |
|---|---|
| LangChain only | `match`, `ghost`, `rephrase`, `recruiter`, `outreach`, `generation-summary{,-premium,-fast}`, `embed` |
| LangGraph agent loop | `salary` (the only tool loop in the codebase today) |
| LangGraph state graph | `generation`, `generation-analyze`, `generation-select`, `generation-select-premium` |

**Alternatives considered**:
- *Everything as a graph, for uniformity* — rejected: eight one-node graphs cost indirection
  and give nothing back. FR-039 exists to prevent it.
- *Everything as chains, no LangGraph* — rejected: loses per-node retry, per-node span
  boundaries and stage-level failure identification, all of which US3 and FR-012 require.
- *No framework, plain HTTP + a hand-written loop* — rejected: this is what exists in Go
  today; porting it to Python would carry the cost of a new service with none of the benefit.

**Python floor**: every pinned library requires `>=3.10`; the plan targets **3.13** to stay
inside the support window of all four for the life of the feature.

---

## R2. Structured output

**Decision**: Pydantic 2.13.4 models per capability, bound via LangChain's structured-output
interface, with the gateway request carrying `response_format: {"type":"json_object"}` as it
does today.

**Rationale**: FR-003 demands a schema-valid result or an explicit failure. Pydantic gives
validation and the JSON Schema needed for R7's cross-language contract generation from one
declaration. The existing Go path already sends `response_format` and already depends on every
model in a JSON-consuming chain supporting it — the *capability trap* recorded in
`specs/domains/llm-routing.md` § 2.1 — so nothing about model requirements changes.

**Alternatives considered**: native tool-calling for structured extraction (rejected: not
every tier in every chain is tool-capable, and the chains are unchanged by FR-010); free-text
plus a parser (rejected outright by FR-003).

---

## R3. Reaching the gateway from Python

**Decision**: `langchain-openai` 1.5.1's chat model, configured with `base_url` pointing at
`GATEWAY_URL` and `api_key` set to `LITELLM_MASTER_KEY`, with `model` set to the **task key**.

**Rationale**: FR-009 requires calls by task key through the existing gateway; FR-010a forbids
any bypass. LiteLLM presents an OpenAI-compatible endpoint (029-FR-011), so the OpenAI client
is the correct adapter and no provider-specific LangChain integration is installed at all.
This is what keeps FR-011 true by construction: the service's dependency tree contains no
provider SDK, so there is no code path that *could* read a provider credential.

Embeddings use the same package's embeddings client against the `embed` key.

**Consequences to carry into the plan**:
- Retry/failover belongs to the gateway (FR-010). The client must therefore be configured with
  its own retries **disabled** (`max_retries=0`), or a single gateway chain exhaustion becomes
  N chain exhaustions and N times the spend.
- Per-request timeout must stay above the gateway's own `request_timeout: 110` so the client
  does not abandon a call the gateway is still serving.

**Alternatives considered**: raw `httpx` calls (rejected: re-implements message
serialization, tool-call parsing and streaming for no gain); LangChain provider integrations
per provider (rejected: violates FR-009/FR-011 and duplicates routing).

---

## R4. Message broker client — Go side and Python side

**Decision**: Go publishes and consumes via `github.com/rabbitmq/amqp091-go` v1.13.0. Python
consumes and publishes via **FastStream** 0.7.4 (RabbitMQ backend, which wraps `aio-pika`).
Broker image `rabbitmq:4.3.4-management-alpine`.

**Rationale**: `amqp091-go` is the RabbitMQ team's own maintained client and the lowest-risk
choice for the side that owns persistence and must control ack timing precisely. On the Python
side FastStream supplies what would otherwise be hand-written: declarative consumers, Pydantic
payload validation at the edge, graceful shutdown, and a `TestRabbitBroker` in-memory harness
that makes consumer logic testable without a container.

The management image is chosen over plain `rabbitmq:4.3.4` deliberately — FR-032 requires an
operator to list, inspect and re-dispatch dead-lettered work, and the management UI plus its
HTTP API deliver that without building a bespoke tool. It replaces `asynqmon`, which the
compose stack runs today for exactly this purpose and which goes away with asynq.

**Alternatives considered**:
- `wagslane/go-rabbitmq` v0.16.1 (nice reconnect ergonomics, but a wrapper over the same
  client and a thinner maintenance base — reconnection is worth writing once against the
  official client).
- `aio-pika` directly in Python (rejected: FastStream gives testing and validation on top of
  it for the same dependency).
- Keeping asynq for non-AI work and adding RabbitMQ only for AI (rejected by FR-026 — two
  async mechanisms is worse than either).
- Kafka/NATS (rejected: no ordering or replay requirement here; RabbitMQ was named).

---

## R5. Preserving asynq's semantics on RabbitMQ

**Decision**: quorum queues, publisher confirms, manual ack, per-work-type retry via a
delay-exchange, and a dead-letter queue per work type.

The behaviours that must survive, and what replaces each:

| asynq behaviour today | RabbitMQ replacement |
|---|---|
| One queue per task type, own concurrency (`policy.go`) | One durable quorum queue per work type; concurrency = consumer prefetch × consumer count |
| `MaxRetry`, exponential backoff | Retry count in a message header; republish to a per-work-type delay exchange with a computed TTL, dead-lettering back to the work queue |
| `RateLimitRetryDelay` — longer backoff on `ErrRateLimited` | Same function, same doubling, feeding the delay TTL instead of asynq's `RetryDelayFunc` |
| Retries exhausted → asynq archive | Dead-letter queue per work type, inspectable in the management UI (FR-031, FR-032) |
| `MaxDuration` per policy, deadline middleware | Consumer-side context deadline, unchanged in value; plus `consumer_timeout` raised above the longest `MaxDuration` |
| Stuck-run detection (`activity` heartbeat + sweeper) | Unchanged — it is already a database-level mechanism, not an asynq one (FR-036) |

**Rationale**: RabbitMQ has no native per-message retry-with-backoff, so the delay exchange is
the standard construction. Quorum queues are chosen over classic mirrored queues because they
are the supported durable type in RabbitMQ 4.x and survive broker restart, which FR-033
requires. Publisher confirms are what make FR-034 enforceable: without them a publish returns
success before the broker has accepted the message.

**Alternatives considered**: the delayed-message-exchange plugin (rejected: a community plugin
in the critical path of every retry, and it does not survive a broker restart as reliably as
queue-based delays); per-message TTL on a single shared delay queue (rejected: head-of-line
blocking — TTL expiry is evaluated only at the queue head).

---

## R6. Idempotency

**Decision**: A deterministic idempotency key per unit of work, carried on the event, enforced
by the backend at the write, not by the broker.

**Rationale**: FR-030 requires at-least-once delivery with idempotent consumers, and
at-least-once is what publisher confirms plus manual ack actually give you — exactly-once does
not exist here. The write side is the only place idempotency can be enforced honestly, and the
backend already owns every write (FR-027).

Two distinct cases, which need different treatment:
- **Work events** (backend → consumer): re-delivery must not produce a second stored result.
  Enforced by a unique constraint on `(work_type, idempotency_key)` at the point of persistence.
- **Result events** (orchestration service → backend): a result for a superseded run must be
  discarded (FR-037). Enforced by comparing the event's `run_id` against the current run
  recorded for that unit of work, and dropping non-matches with a counter incremented.

**Cost note worth stating**: a redelivered AI work event that has not yet been persisted *will*
re-run the model and spend tokens again. FR-030 bounds this to one retry budget rather than
eliminating it; the dedupe is on stored state, not on spend.

---

## R7. Typed contracts across Go ↔ Python (Constitution III)

**Decision**: JSON Schema as the interchange, generated from the Go event structs, with Python
Pydantic models generated from those schemas. Both generated artifacts are checked in, and CI
regenerates and diffs them (`make contracts-check`), exactly as `sqlc-check` and `tygo-check`
work today.

**Rationale**: Principle III forbids hand-maintained duplicate types across a language
boundary, and this feature adds a third language to a repository whose existing answer is
generate-don't-duplicate. Go stays the source of truth because the backend owns persistence
and already generates the TypeScript side from `internal/dto` via tygo — making Python a third
generated target keeps one direction of authority rather than introducing a second.

**Alternatives considered**:
- Protobuf/Avro as the source of truth (rejected: would make Go a generated target too,
  inverting the existing tygo direction and rewriting DTO ownership for a payload format that
  is JSON on the wire regardless).
- Pydantic as the source of truth, Go generated from it (rejected: same inversion, and the Go
  structs already exist).
- Hand-written models on both sides with a shared schema doc (rejected outright by III).

**Schema versioning** (FR-028, FR-029): every event carries `schema_version`. A consumer that
receives a version it does not understand rejects the message to the dead-letter queue rather
than best-effort parsing it.

---

## R8. Langfuse integration

**Decision**: `langfuse` 4.14.4 via its LangChain callback handler, attached per run, with the
existing LiteLLM `success_callback`/`failure_callback` left in place.

**Rationale**: FR-017 requires both records and requires them correlatable. The SDK is
OpenTelemetry-based, so spans nest naturally: a LangGraph node becomes a span, and the model
call inside it becomes a child span. Correlation with the gateway's own records is achieved by
sending the trace id as request metadata on the gateway call, which LiteLLM already forwards
into its Langfuse record.

**Non-blocking requirement (FR-016)**: the SDK batches and flushes on a background thread;
the plan must additionally ensure the service never blocks shutdown on a flush beyond a short
bounded timeout, and that a collector returning errors is logged once and otherwise ignored.
This is worth an explicit test with the collector stopped (SC-006).

**Retention (FR-018/FR-018a)**: 30-day payload purge is *not* a client-side concern. Langfuse
data lives in ClickHouse; the purge runs as a scheduled job against Langfuse's own data
retention configuration. Metrics survive because the purge targets input/output payload
columns rather than observation rows.

**Alternatives considered**: OpenTelemetry SDK directly against the Langfuse OTLP endpoint
(rejected: loses generation-level semantics — token counts, cost, model — that the native
integration extracts automatically); self-hosted trace tables in Postgres (rejected: Langfuse
is already deployed and already holds the call-level half).

---

## R9. Service shape and transport split

**Decision**: One Python service exposing **two** entry points — a FastAPI 0.141.1 HTTP app for
interactive capabilities, and FastStream consumers for queued capabilities. Both run in one
process, sharing the capability registry.

**Rationale**: The spec's event-driven mandate (FR-027) is scoped to *queued work*: "The
backend MUST NOT call the orchestration service synchronously **for queued work**". Six work
types are queued (ingest, match, generate, enrich, salary, ghost). But several AI capabilities
today run inside HTTP handlers where a user is waiting — keyword rephrase, recruiter
extraction, outreach drafting, coach chat (which calls `rephrase`), and the profile embedding
path. Routing those through a broker would mean a request/reply-over-AMQP round trip
to serve a synchronous user request: more moving parts, worse latency, no durability benefit
(nobody wants a user's abandoned request replayed an hour later).

So: **queued work is events; interactive work is HTTP**. Both go to the same service, run the
same capability code, and produce the same traces.

**Alternatives considered**: two separate Python services (rejected: same code, same
dependencies, twice the deployment); everything over AMQP including interactive
(rejected: reply-queue correlation for a user-facing request is complexity with no payoff);
everything over HTTP including queued work (rejected by FR-027 — it holds a Go worker open for
the minutes a generation run takes, which SC-014 explicitly measures against).

---

## R10. Python toolchain and the merge gate

**Decision**: `uv` 0.12.5 for dependency management and locking, `pytest` 9.1.1, `ruff` 0.16.3
for lint and format, `mypy` 2.3.1 in strict mode. New Makefile targets `lint-py` and `test-py`,
both added to `test-lint`.

**Rationale**: FR-025 requires the service's suite in the merge gate, and Principle IV requires
each app to test in its native toolchain. `AGENTS.md` currently states `test-lint` "never
claimed to check" Python because none existed — that sentence and the constitution's
technology constraints both need editing in this feature, not quietly contradicting.

`uv` is chosen over pip/poetry for lockfile determinism and speed; a locked, hash-pinned
dependency set also matters for `make audit`, which gains a Python leg (`vuln-py`) so the new
dependency surface is not invisible to the supply-chain gate that spec 039 established.

---

## R11. Migration sequencing

**Decision**: Three phases, strictly ordered, each independently revertible.

1. **Broker first, on non-AI work.** RabbitMQ enters the stack; `ingest` and `enrich` migrate
   off asynq. No Python, no AI change. This proves durability, retry, dead-lettering and
   reconnection against work whose correctness is easy to check (US5 is independently testable
   for exactly this reason).
2. **Remaining queues, then asynq removal.** `match`, `generate`, `salary`, `ghost` move to
   RabbitMQ still handled in Go. asynq and `asynqmon` are removed; SC-013 becomes checkable.
3. **Capability-by-capability AI cutover.** The Python service appears, and capabilities move
   one at a time behind a per-capability switch (FR-020), each gated on its recorded baseline
   (FR-021), each removing its Go path once confirmed (FR-023). Order within this phase:
   `ghost` first (smallest, single-call, low blast radius), then the other single-call
   capabilities, then `salary` (first LangGraph loop), then the generation pipeline (largest),
   then `embed` last (highest call volume, least benefit, so it moves once the path is proven).

**Rationale**: The broker migration and the AI migration fail in completely different ways.
Interleaving them means a failure in production is ambiguous between the two. This ordering
also front-loads the risk that is cheapest to reverse.

---

## Open items for `/speckit.tasks`

- **Baseline capture (FR-021) must precede phase 3** for every capability. Baselines exist for
  matching and routing (`specs/044-litellm-only-routing/baseline-*`); the remaining twelve
  capabilities need theirs recorded before their cutover task can be written.
- **Two constitution amendments** are prerequisites, not follow-ups — see plan.md
  § *Constitution Check*.
