# Implementation Plan: Dedicated AI Orchestration Service

**Branch**: `047-langchain-ai-service` | **Date**: 2026-08-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/047-langchain-ai-service/spec.md`

## Summary

Two migrations, deliberately sequenced as one feature because the second depends on the first.

**Messaging.** Every asynchronous work type moves off asynq/Redis onto RabbitMQ, and the
platform becomes event-driven: the backend publishes work events, consumers publish result
events, the backend persists. asynq and `asynqmon` are removed (FR-026, SC-013).

**AI orchestration.** A Python service built on LangChain and LangGraph takes over prompt
assembly, step sequencing, tool loops and output validation for all fourteen task keys. It
reaches models only through the existing LiteLLM gateway, by task key (FR-009 – FR-011), and
emits a Langfuse trace per run with one span per step (FR-012). Go keeps every database write,
every authorization decision and all non-AI logic.

The approach, in one line each: **layer by need** — LangChain alone for the eight single-call
capabilities, LangGraph for the two that loop or branch — `salary` and the generation pipeline
(FR-039 – FR-041); **transport by
nature of the work** — events for the six queued work types, HTTP for the interactive ones
(research R9); **migrate in three ordered phases** — broker on non-AI work, then remaining
queues and asynq removal, then AI capabilities one at a time behind a per-capability switch
(research R11).

## Technical Context

**Language/Version**: Go 1.25 (`apps/api`, existing) · Python 3.13 (`apps/ai`, new) ·
TypeScript (`apps/dashboard`, unchanged)

**Primary Dependencies**:
- New Python service: `langchain` 1.3.15, `langgraph` 1.2.11, `langchain-openai` 1.5.1
  (gateway adapter only — no provider SDKs), `langfuse` 4.14.4, `fastapi` 0.141.1,
  `faststream[rabbit]` 0.7.4, `pydantic` 2.13.4
- Go: `github.com/rabbitmq/amqp091-go` v1.13.0 added; `github.com/hibiken/asynq` removed
- Infrastructure: `rabbitmq:4.3.4-management-alpine` added; `asynqmon` removed

**Storage**: Postgres + pgvector (unchanged, Go-only access) · RabbitMQ quorum queues for work
and results · ClickHouse via the already-deployed Langfuse group for traces · Redis retained
for caching and rate-limit state only, no longer a queue backend

**Testing**: `go test` (existing) · `vitest` (existing) · `pytest` 9.1.1 + `ruff` 0.16.3 +
`mypy` 2.3.1 strict (new, joined to `make test-lint` as `lint-py` / `test-py`) ·
`TestRabbitBroker` for consumer unit tests · Docker-backed integration tests against real
RabbitMQ and Postgres

**Target Platform**: Linux containers, Docker Compose (`docker-compose.yml` dev,
`docker-compose.prod.yml` prod)

**Project Type**: Multi-service monorepo — Go API, React dashboard, new Python AI service

**Performance Goals**: Median end-to-end completion per capability within 110% of its
pre-migration measurement (SC-005) · gateway latency budget unchanged (029-SC-006, ≤200 ms
median overhead) · backend worker occupancy for AI work drops to publish + persist time
(SC-014)

**Constraints**: No provider credentials in the Python service (FR-011) · no primary-database
credentials in it (FR-008) · no bypass of the gateway under any condition (FR-010a) · tracing
strictly off the critical path (FR-016) · trace payloads purged at 30 days (FR-018) · prompts
and workflows in-repo, never fetched at runtime (FR-015a)

**Scale/Scope**: 14 task keys · 6 queued work types + 4 interactive capabilities (`rephrase`,
`recruiter`, `outreach`, `embed`) · 2 LangGraph workflows, 9 single-call capabilities ·
single-operator deployment, no multi-tenant scaling requirement

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

Constitution version 2.0.0.

| Principle | Verdict | Reasoning |
|---|---|---|
| **I. No Auto-Apply, Ever** | ✅ Pass | Nothing here touches submission. The orchestration service drafts and scores; it holds no outbound-action capability and no credentials to act with. Human review gates are in the backend and untouched. |
| **II. Grounded Generation** | ✅ Pass, with a requirement | Generation input still comes from the master profile and the posting — now delivered in the work event rather than read from the DB (FR-008). Traceability *improves*: FR-012 records each stage's exact input and output, making "trace a generated claim to its source field" mechanical rather than inferential. The plan must not let prompt assembly in Python introduce content the event did not carry. |
| **III. Typed Contracts Across Service Boundaries** | ⚠️ Pass only via new generation | A third language crosses the boundary. Handled by R7: Go structs → JSON Schema → generated Pydantic models, both checked in, both diffed in CI (`make contracts-check`), mirroring `sqlc-check`/`tygo-check`. Hand-written Python mirrors of Go event types would violate this outright. |
| **IV. Test Discipline Per Language** | ⚠️ Requires amendment | The principle names `go test` and `vitest` only. A Python suite must join `make test-lint` (FR-025) and integration tests must exercise real RabbitMQ via Compose. The principle's *spirit* is satisfied; its *text* enumerates two toolchains and needs a third. |
| **V. Self-Hosted Control Plane, Single Inference Path** | ✅ Pass, strengthened | Every model call still goes through the self-hosted gateway by task key. FR-011 keeps provider credentials out of the new service — and R3 makes it structural: no provider SDK is installed, so no code path exists that could use one. RabbitMQ is self-hosted, in-stack, not externally reachable (FR-038). |

**Both amendments are DONE — constitution 2.1.0, amended 2026-08-18**, ahead of implementation
rather than alongside it, so no task in this plan begins by contradicting the constitution:

1. **Async mechanism** — § *Technology & Architecture Constraints* now names RabbitMQ, one
   durable queue per work type plus a dead-letter queue per work type, Redis demoted to caching
   and rate-limit state, and services communicating by events. The one-queue-per-work-type rule
   survived the change; only its implementation changed.
2. **Runtimes and test discipline** — `apps/ai` (Python, LangChain/LangGraph, Langfuse) is
   admitted as a third runtime with its boundaries stated in the constraints themselves, and
   Principle IV now names `pytest` alongside `go test` and `vitest`, requires a real message
   broker in integration paths, and requires `make audit` to cover every language.

**Still outstanding, and deliberately deferred**: `AGENTS.md` ("No Python is in this
repository", the `test-lint` description, the `apps/api` "asynq workers" line), `README.md`,
`docs/docs/async/*`, `docs/docs/architecture/*` and the asynq references in
`specs/domains/*.md`. Every one of them describes the system **as it currently runs**, and asynq
is still the live dispatch mechanism. They become false when phase 2 removes asynq and are
corrected in that change (contracts/configuration.md K6). Correcting them now would leave the
repository documenting a system nobody has built.

## Project Structure

### Documentation (this feature)

```text
specs/047-langchain-ai-service/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── events.md            # Event envelope, work/result payloads, routing topology
│   ├── capabilities.md      # Per-capability input/output contracts, layer, bounds
│   ├── http.md              # Interactive HTTP surface of the AI service
│   ├── messaging.md         # Exchanges, queues, retry/DLQ, ack and confirm semantics
│   └── configuration.md     # Environment variables, secrets boundaries, compose wiring
├── checklists/
│   └── requirements.md  # Spec quality checklist (complete)
└── tasks.md             # Phase 2 output (/speckit.tasks — NOT created here)
```

### Source Code (repository root)

```text
apps/api/                                   # Go backend — loses AI orchestration, keeps everything else
├── cmd/server/
│   ├── compose.go                          # llmHandles removed; event publisher + result consumers wired
│   └── servers.go                          # asynq.Server workers → AMQP consumers
├── internal/
│   ├── events/                             # NEW — the messaging layer
│   │   ├── envelope.go                     # Event envelope: type, ids, schema_version (FR-028)
│   │   ├── publisher.go                    # Publisher confirms, fail-loud publish (FR-034)
│   │   ├── consumer.go                     # Manual ack, prefetch, reconnection (FR-035)
│   │   ├── retry.go                        # Delay-exchange backoff, DLQ routing (FR-031)
│   │   ├── topology.go                     # Exchange/queue/binding declaration
│   │   └── schema/                         # Generated JSON Schema, source for Python models (R7)
│   ├── queue/                              # Payload types + policies retained; asynq bindings removed
│   ├── aiclient/                           # NEW — HTTP client for interactive AI capabilities
│   └── <domain>/                           # matching, generation, salary, ghost, keyword, recruiter,
│                                           # outreach, profile: LLM orchestration deleted per
│                                           # capability at its cutover. coach keeps its handler and
│                                           # repoints at the `rephrase` capability; companyintel and
│                                           # interviewprep make no model calls and are untouched
└── internal/platform/llm/                  # Removed entirely at the end of phase 3 (FR-019a)

apps/ai/                                    # NEW — Python orchestration service
├── pyproject.toml                          # uv-managed, locked
├── src/jobfinder_ai/
│   ├── main.py                             # FastAPI app + FastStream consumers in one process (R9)
│   ├── settings.py                         # Config, startup validation (FR-007)
│   ├── gateway.py                          # LiteLLM-backed chat/embeddings clients, retries disabled (R3)
│   ├── tracing.py                          # Langfuse handler, non-blocking flush (FR-016)
│   ├── contracts/                          # GENERATED Pydantic models — do not hand-edit (R7)
│   ├── capabilities/
│   │   ├── registry.py                     # name → capability, validated at startup (FR-007)
│   │   ├── single/                         # match, ghost, rephrase, recruiter, outreach,
│   │   │                                   # summary×3, embed — LangChain only (FR-039)
│   │   └── graphs/
│   │       ├── salary.py                   # LangGraph bounded tool loop
│   │       └── generation.py               # LangGraph state graph: analyze → select → summarize → assemble
│   ├── prompts/                            # In-repo prompt text, versioned by commit (FR-015a)
│   └── messaging/                          # FastStream consumers, result publication
└── tests/
    ├── unit/                               # Capability logic, TestRabbitBroker
    ├── contract/                           # Generated models match checked-in schemas
    └── integration/                        # Real RabbitMQ + gateway stub

gateway/config.yaml                          # UNCHANGED — chains, task keys, callbacks all stay (FR-010)
docker-compose.yml                           # + rabbitmq; − asynqmon; + ai service
docker-compose.prod.yml                      # same
Makefile                                     # + lint-py, test-py, vuln-py, contracts-generate/-check
.specify/memory/constitution.md              # 2.0.0 → 2.1.0 (two amendments above)
AGENTS.md                                    # Python claims corrected
```

**Structure Decision**: A third top-level app, `apps/ai`, alongside `apps/api` and
`apps/dashboard` — the repository's existing convention for a deployable unit. The Python
service is one process serving two entry points (HTTP for interactive capabilities, AMQP
consumers for queued ones, per R9) rather than two deployables, because they share the
capability registry, the prompts and the gateway client entirely.

`internal/events/` is a new Go package rather than a rewrite of `internal/queue/`: the payload
types and `TaskPolicy` values in `queue` describe *work*, survive the migration unchanged, and
are what let the replacement be checked against the original behaviour. Only the asynq bindings
inside it are deleted.

## Phase sequencing

Each phase is independently revertible; none begins before its predecessor is confirmed in
production. Detail in research.md R11.

| Phase | Scope | Done when |
|---|---|---|
| **0. Amendments** | Constitution 2.1.0 | ✅ Done 2026-08-18. Doc corrections deferred to the phases that make them true |
| **1. Broker on non-AI work** | RabbitMQ in stack; `ingest`, `enrich` migrated; retry/DLQ/reconnect proven | US5 acceptance scenarios pass; no AI change made |
| **2. Remaining queues, asynq removed** | `match`, `generate`, `salary`, `ghost` on RabbitMQ, still Go-handled; asynq + asynqmon deleted | SC-011, SC-013 verified |
| **3. AI cutover, capability by capability** | `apps/ai` ships; capabilities move in order: `ghost` → other single-call → `salary` → generation pipeline → `embed` | Per capability: baseline within tolerance (FR-021, SC-004), Go path deleted (FR-023) |
| **4. Close-out** | `internal/platform/llm` removed; domain docs folded into `specs/domains/` | SC-010, SC-013 hold; feature directory deleted per repo convention |

Baselines (FR-021) are captured **before** each capability's phase-3 task, not during it.
Matching and routing baselines already exist under `specs/044-litellm-only-routing/`; the other
twelve capabilities need theirs recorded first.

## Post-Design Constitution Re-check

Re-evaluated after Phase 1. No new violations; two findings the design changed rather than
merely restated.

- **Principle II (Grounded Generation) got sharper, not weaker.** The design forces AI work
  events to carry an input snapshot (data-model.md § 3, E3-2) because FR-008 denies the service
  database access. That snapshot *is* the grounding source, and C6-3 plus C5-2 forbid the
  service fetching anything else — so "generated content traces to profile or posting" becomes
  structurally true rather than a prompt-discipline hope. E3-4's `snapshot_hash`, echoed onto
  every result, names the exact input a stored result came from.
- **Principle III is satisfied only if E7-2 is enforced.** The whole cross-language typing story
  rests on `make contracts-check` running in CI. Without it, generated Pydantic models drift
  from the Go structs silently and the principle is violated in the least visible way possible.
  This is a task, not an aspiration.
- **Principle V is strengthened structurally.** C7-3 makes "no provider credentials" checkable
  in CI by asserting no provider SDK is in the dependency tree — a stronger guarantee than
  withholding environment variables, since it removes the code path rather than the key.
- **Principle IV**: unchanged finding. The Python suite (`lint-py`, `test-py`) and the Python
  supply-chain leg (`vuln-py`) must join `test-lint` and `audit`. Still requires the amendment.

The two amendments named in § Constitution Check remain prerequisites. Nothing in Phase 1
design removed the need for either.

**Agent context**: `.specify/scripts/bash/update-agent-context.sh` does not exist in this
repository — agent context lives in `AGENTS.md`, maintained by hand (feature 024). Its Python
and `test-lint` claims are corrected as part of implementation, recorded as an obligation in
contracts/configuration.md K6-2 rather than edited here, since the claims stay true until the
service actually lands.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Third runtime (Python) in a Go+TS repo | LangChain and LangGraph are the mandated orchestration layer (spec § Mandated Technologies) and are Python-first. A new runtime brings a new dependency surface, a new CI leg, and a second place to deploy | Go LLM libraries exist but not LangGraph's graph runtime; keeping orchestration in Go is the status quo the feature exists to replace. Rejected by the maintainer's explicit choice of stack |
| Two migrations in one feature | The orchestration service consumes work events, so the broker must exist first. Splitting them would leave phase 3 blocked on another feature's completion | Sequencing inside one feature (phases 1–2 before 3, each revertible) gets the ordering benefit without the coordination cost. Flagged in checklists/requirements.md: if `/speckit.tasks` shows the task count skewed to messaging, splitting is still the better call |
| Two transports to one service (AMQP + HTTP) | Queued work must not hold a Go worker open (FR-027, SC-014); interactive work must not round-trip through a broker while a user waits (R9) | All-AMQP needs reply-queue correlation for user-facing requests; all-HTTP re-introduces the blocked worker SC-014 measures against. One process, two entry points, one capability registry |
| Generated Pydantic models from Go structs | Principle III forbids hand-maintained duplicate types across a boundary | Hand-written models drift silently — the exact failure III exists to prevent. Protobuf would invert the existing tygo authority direction for no wire-format gain |
