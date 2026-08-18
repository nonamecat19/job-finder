---
description: "Task list for 047-langchain-ai-service"
---

# Tasks: Dedicated AI Orchestration Service

**Input**: Design documents from `/specs/047-langchain-ai-service/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Test tasks are included. FR-025 requires the new service's suite in the merge gate,
Principle IV requires per-language suites, and the spec's acceptance scenarios (US5 especially)
are only checkable by running them. This is not TDD-by-preference — it is a stated requirement.

**Organization**: Grouped by user story. Execution order departs from raw priority in one
place: **US5 (P1, messaging) runs first**, because every other story consumes work events that
do not exist until it lands. Within that constraint, priority order holds.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Parallelizable — different files, no dependency on an incomplete task
- **[Story]**: US1–US5 from spec.md
- All paths are repository-relative
- **Task IDs are stable identifiers, not execution order.** Execution order is phase order, then
  position within the phase. T128–T133 were added after `/speckit.analyze` and sit in the phases
  where they belong rather than at the end; renumbering 127 existing tasks would churn every
  cross-reference for no gain

## Path Conventions

Multi-service monorepo per plan.md § Project Structure: `apps/api/` (Go), `apps/ai/` (Python,
new), `apps/dashboard/` (unchanged by this feature).

---

## Phase 0: Constitution ✅ COMPLETE

Constitution amended to **2.1.0** on 2026-08-18 — RabbitMQ replaces asynq in the data-layer
constraint, `apps/ai` admitted as a third runtime, Principle IV extended to `pytest`. No task
here; recorded so the sequence is legible.

Doc corrections (`AGENTS.md`, `README.md`, `docs/docs/async/*`, `specs/domains/*`) are
deliberately **not** here — they describe the live system and become false only when asynq is
removed. They land in Phase 3 (T045–T047) and Phase 8.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Broker in the stack, Python service scaffolded, gates extended. No behaviour
changes yet.

- [X] T001 Add `rabbitmq:4.3.4-management-alpine` service to `docker-compose.yml` with named volume, `rabbitmq-diagnostics` healthcheck, and management UI published on loopback only (contracts/configuration.md K4-1, K4-4)
- [X] T002 Add the same `rabbitmq` service to `docker-compose.prod.yml` with **no** published ports — authenticated, in-stack, not reachable from outside it (FR-038, K4-4, M7-2)
- [X] T003 [P] Add `RABBITMQ_URL`, `RABBITMQ_DEFAULT_USER`, `RABBITMQ_DEFAULT_PASS`, `AI_SERVICE_URL`, `AI_SERVICE_TOKEN`, `AI_CAPABILITY_ROUTING`, `LANGFUSE_PAYLOAD_RETENTION_DAYS` to `.env.example` with the container-network-address warning `GATEWAY_URL` already carries (K1, K6-1)
- [X] T004 [P] Remove the default `guest` account or replace its password in every environment, including dev, in `docker-compose.yml` and `docker-compose.prod.yml` (M7-1)
- [X] T005 [P] Create separate broker credentials for backend and AI service with distinct permissions in `docker-compose.yml` — AI service may consume AI work queues and publish results only (M7-3)
- [X] T006 Scaffold `apps/ai/` with `pyproject.toml` (uv-managed, Python 3.13), locked dependencies: langchain 1.3.15, langgraph 1.2.11, langchain-openai 1.5.1, langfuse 4.14.4, fastapi 0.141.1, faststream[rabbit] 0.7.4, pydantic 2.13.4
- [X] T007 [P] Configure `ruff` 0.16.3 (lint + format) and `mypy` 2.3.1 strict in `apps/ai/pyproject.toml`
- [X] T008 [P] Configure `pytest` 9.1.1 with `apps/ai/tests/{unit,contract,integration}/` layout in `apps/ai/pyproject.toml`
- [X] T009 Add `lint-py` and `test-py` targets to `Makefile` and wire both into `test-lint` (FR-025, Principle IV)
- [X] T010 [P] Add `vuln-py` target to `Makefile` and wire into `audit`, so the new dependency surface is covered by the supply-chain gate (Principle IV, spec 039)
- [X] T011 [P] Add `apps/ai` Dockerfile and an `ai` service to both compose files, depending on `rabbitmq` healthy and `litellm` started, and explicitly **not** on `langfuse-web` (K4-2). `api` MUST NOT depend on `ai` (K4-5)
- [X] T132 [P] Test that the backend starts and serves every non-AI path with the `ai` service stopped (FR-024, K4-5)
- [X] T012 Extend `.github/workflows/` to run `lint-py`, `test-py` and `vuln-py` on every PR

**Checkpoint**: `make up` brings up RabbitMQ; `make test-lint` runs a (trivially passing) Python suite.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The Go messaging layer, the cross-language contract pipeline, and the Python
service skeleton. Everything downstream depends on these.

**⚠️ CRITICAL**: No user story work begins until this phase completes.

### Event contracts (blocks everything that publishes or consumes)

- [X] T013 Create `apps/api/internal/events/envelope.go` with the envelope struct from data-model.md § 1 — `event_id`, `event_type`, `schema_version`, `occurred_at`, `work_id`, `correlation_id`, `idempotency_key`, `run_id`, `activity_id`, `trace_id` (FR-028, E1)
- [X] T014 Add the closed `event_type` registry and `Failure` type with the nine categories to `apps/api/internal/events/envelope.go`, keeping the five existing sentinel names from `internal/platform/llm/infrastructure/shared/errors.go` (E2, E5-1)
- [X] T015 [P] Define work and result payload structs in `apps/api/internal/events/payloads.go`, reusing the existing `internal/queue` payload types unchanged and adding the input-snapshot and `snapshot_hash` fields (E3-1, E3-2, E3-4)
- [X] T016 Write the Go→JSON Schema generator and `contracts-generate` / `contracts-check` Makefile targets, emitting to `apps/api/internal/events/schema/` (E7-1, E7-2, Principle III)
- [X] T017 Generate Pydantic models into `apps/ai/src/jobfinder_ai/contracts/` with a do-not-edit header, wired into `contracts-generate` (E7-3)
- [X] T018 [P] Contract test in `apps/ai/tests/contract/test_generated_models.py` asserting generated models round-trip every schema fixture
- [X] T019 [P] Go test in `apps/api/internal/events/envelope_test.go` asserting envelope invariants: required fields, `correlation_id` echo, and that no envelope field carries profile content (E1-1, E1-4)

### Go messaging layer

- [X] T020 Implement topology declaration in `apps/api/internal/events/topology.go` — exchanges, quorum queues, delay queues, DLQs per data-model.md § 4, failing loudly on a conflicting declaration (M1-1, M1-2, M1-3)
- [X] T021 Implement the publisher in `apps/api/internal/events/publisher.go` with publisher confirms, `persistent` delivery, `mandatory` + returned-message handler, and error return on nack or timeout (M2-1 – M2-4, FR-034)
- [X] T022 Implement the consumer in `apps/api/internal/events/consumer.go` with manual ack, prefetch = configured concurrency, ack-after-durable-handling, and bounded-grace shutdown that nacks with requeue (M3-1 – M3-3, M3-5)
- [X] T023 Implement automatic reconnection with bounded exponential backoff in `apps/api/internal/events/consumer.go`, re-declaring topology and re-establishing prefetch on reconnect (M3-4, FR-035)
- [X] T024 Port `RateLimitRetryDelay` from `apps/api/internal/queue/policy.go` into `apps/api/internal/events/retry.go`, computing a delay-queue TTL instead of an asynq `RetryDelayFunc`, preserving the doubling and ceiling exactly (M4-2, data-model.md § 6)
- [X] T025 Implement retry republish and dead-lettering in `apps/api/internal/events/retry.go` — `x-attempt` increment, delay-exchange routing, straight-to-DLQ for non-retryable categories, `x-first-failure-reason` on dead-letter (M4-1, M4-3, M4-4, FR-031)
- [X] T026 Set broker `consumer_timeout` above the longest work type's `MaxDuration` in `docker-compose.yml` and `docker-compose.prod.yml` (M3-6)
- [X] T027 [P] Unit tests in `apps/api/internal/events/retry_test.go` covering the fixed ladder (`1s → 10s → 1m → 10m`, five attempts), rate-limited entry at the `1m` rung, budget exhaustion → DLQ, and that `retryable` is read from the failure rather than re-derived (E5-2, FR-004)
- [X] T028 [P] Unit tests in `apps/api/internal/events/publisher_test.go` asserting a nacked or unroutable publish returns an error and never reports success (M2-3, M2-4)

### Idempotency

- [X] T029 Add goose migration creating the idempotency ledger table per data-model.md § 7, with a unique primary key on `idempotency_key` and a sequential version number
- [X] T030 Implement ledger write-in-same-transaction and supersession check in `apps/api/internal/events/idempotency.go` — duplicate key discards, `run_id` mismatch discards with a counter (M5-2, M5-3, FR-030, FR-037)
- [X] T031 [P] Add ledger pruning (rows older than the longest retry budget plus margin) to the existing sweeper in `apps/api/internal/activity/`
- [X] T032 [P] Integration test in `apps/api/internal/events/idempotency_test.go` proving a redelivered work event produces exactly one stored result

### Python service skeleton

- [X] T033 Implement `apps/ai/src/jobfinder_ai/settings.py` with startup validation that fails naming the missing key for `GATEWAY_URL`, `LITELLM_MASTER_KEY`, `RABBITMQ_URL`, `AI_SERVICE_TOKEN`, and never probes reachability (K3-1, K3-2, K3-3)
- [X] T034 Implement the gateway client in `apps/ai/src/jobfinder_ai/gateway.py` — `langchain-openai` chat and embeddings clients pointed at `GATEWAY_URL`, by task key only, **`max_retries=0`**, per-request timeout above the gateway's `request_timeout`, with no bypass path under any condition (FR-009, FR-010, FR-010a, research R3, C7-1, C7-2, C7-4)
- [X] T035 Implement the capability registry in `apps/ai/src/jobfinder_ai/capabilities/registry.py` — the service-side home of prompt assembly, step sequencing, tool loops, validation and retry (FR-001) — validating name, task key, layer, models, bounds and prompt module at startup (C1-1 – C1-4, FR-007)
- [X] T036 Implement Langfuse bootstrap in `apps/ai/src/jobfinder_ai/tracing.py` — callback handler, background flush, bounded shutdown flush, collector errors logged once and otherwise ignored (FR-016, research R8)
- [X] T037 Implement `apps/ai/src/jobfinder_ai/main.py` running the FastAPI app and FastStream consumers in one process, sharing the registry (research R9)
- [X] T038 [P] Implement `/health/live` and `/health/ready` in `apps/ai/src/jobfinder_ai/main.py` — ready verifies registry, broker connection and gateway configuration, never issuing a model call (H2, H8-1, H8-2, H8-3)
- [X] T039 [P] CI check asserting no provider SDK appears in the Python dependency tree, in `.github/workflows/` (C7-3, FR-011)
- [X] T040 [P] CI check asserting the `ai` service environment contains no `DATABASE_URL` and no provider credential (K2-2, K2-3, FR-008)
- [X] T130 [P] Contract test in `apps/ai/tests/contract/test_no_prompt_leakage.py` asserting no work-event payload and no HTTP request body carries prompt text, a step sequence, or a model/provider name (FR-002)
- [X] T131 [P] Registry-level assertion in `apps/ai/src/jobfinder_ai/capabilities/registry.py` that a capability whose transport is `event` returns 404 through `/invoke` (H1-2, FR-027)

**Checkpoint**: Messaging layer exists and is unit-tested; the Python service boots, validates and reports ready. Neither is wired to real work yet.

---

## Phase 3: User Story 5 — Work is dispatched as events that survive restarts (P1) 🎯 FOUNDATION MVP

**Goal**: Every work type dispatched through RabbitMQ; work survives restarts; failures
dead-letter where an operator can find them. asynq removed.

**Independent Test**: With no AI capability migrated, run all existing work types through the
broker, restart consumers mid-flight, and confirm no work is lost, none is processed twice, and
repeatedly failing work reaches the DLQ.

### Migration — non-AI work first (research R11 phase 1)

- [X] T041 [US5] Replace asynq dispatch for `ingest` with event publish/consume in `apps/api/internal/jobsources/interfaces/worker/` and `apps/api/cmd/server/servers.go`, preserving `IngestMaxRetry` and `AITaskTimeoutIngest` (data-model.md § 6)
- [X] T042 [US5] Replace asynq dispatch for `enrich` in `apps/api/internal/enrichment/` and `apps/api/cmd/server/servers.go`, applying the five-attempt ladder from data-model.md § 6 (a deliberate reduction from asynq's inherited 25) and preserving its timeout
- [X] T133 [US5] Record the retry-budget change (25 attempts → 5, fixed ladder) in the migration notes and `docs/docs/async/`, so it is read rather than discovered (data-model.md § 6)
- [X] T043 [US5] Verify stuck-run detection still works over the new transport — `activity` heartbeat, stale sweep and `ACTIVITY_QUEUED_GRACE` are DB-level and must be untouched (FR-036)

### Migration — remaining work types, then asynq removal (research R11 phase 2)

- [X] T044 [US5] Replace asynq dispatch for `match`, `generate`, `salary` and `ghost` in `apps/api/cmd/server/servers.go` and each domain's worker entry point, still handled in Go, on the same five-attempt ladder
- [X] T045 [US5] Delete asynq from `apps/api`, leaving exactly one asynchronous mechanism (FR-026) — `internal/queue` asynq bindings, `asynq.Server` wiring in `cmd/server/servers.go`, `RedisOpt`, and the `hibiken/asynq` dependency from `go.mod`. Retain the payload structs and `TaskPolicy` values (plan.md § Project Structure)
- [X] T046 [US5] Remove the `asynqmon` service from `docker-compose.yml`; queue inspection passes to the RabbitMQ management UI (K4-3, FR-032)
- [X] T047 [US5] Correct every now-false asynq statement: `AGENTS.md` (`apps/api` line), `README.md` (architecture summary, asynqmon section), `docs/docs/async/*`, `docs/docs/architecture/*`, and the asynq references in `specs/domains/llm-routing.md`, `platform-operations.md`, `resume-generation.md`, `codebase-structure.md`

### Operator surface

- [X] T048 [P] [US5] Add DLQ depth per work type to the backend health endpoint in `apps/api/internal/health/` (M8-2)
- [X] T049 [P] [US5] Add broker unavailability as a distinct health signal, separate from database and gateway (M8-3)
- [X] T050 [P] [US5] Add queue depth, consumer count, redelivery rate and publish-confirm latency metrics per work type in `apps/api/internal/events/metrics.go` (M8-1)
- [X] T051 [P] [US5] Document the DLQ re-dispatch workflow (reset `x-attempt`, publish to `jobfinder.work`) in `docs/docs/async/monitoring.md` (M4-7, FR-032)

### Tests for User Story 5

- [X] T052 [P] [US5] Integration test in `apps/api/internal/events/durability_test.go`: work published while the consumer is stopped is processed on restart, with no loss (US5 scenario 1, FR-033)
- [X] T053 [P] [US5] Integration test: consumer killed mid-processing → redelivery → exactly one stored result (US5 scenario 2, SC-011)
- [X] T054 [P] [US5] Integration test: work exhausting its retry budget lands in `dlq.<work_type>` with `x-first-failure-reason` set (US5 scenario 3, SC-012)
- [X] T055 [P] [US5] Integration test: duplicate delivery does not duplicate or corrupt stored results (US5 scenario 4)
- [X] T056 [P] [US5] Integration test: publish with the broker down returns an error to the caller and the triggering HTTP request is not 202 (US5 scenario 5, M2-3)
- [X] T057 [P] [US5] Integration test: broker restart under continuous load loses zero accepted units of work (SC-011)
- [X] T058 [P] [US5] Integration test: an unknown `event_type` and an unimplemented `schema_version` are both dead-lettered without body deserialization, rather than best-effort parsed (FR-029, M6-1, M6-2)

**Checkpoint**: SC-011, SC-012, SC-013 verifiable. One asynchronous mechanism remains. No AI change made.

---

## Phase 4: User Story 1 — Operator diagnoses a bad AI result end to end (P1) 🎯 AI MVP

**Goal**: One capability runs through the Python service and produces a complete, findable
trace. `ghost` first — single-call, smallest blast radius (research R11).

**Independent Test**: Run `ghost` end to end, then locate its trace and confirm every step,
input, output, duration, token count and cost is visible and findable by user, job and
capability.

### Cutover mechanism (used by every later capability)

- [X] T059 [US1] Implement the `AI_CAPABILITY_ROUTING` switch in `apps/api/internal/config/config.go` — per-capability `python` or `go`, absent meaning `go` (FR-020, K1)
- [X] T060 [US1] Implement `apps/api/internal/aiclient/` HTTP client with per-capability timeouts, no retry on non-retryable categories, `Retry-After` honoured, and `trace_id` surfaced into backend structured logs (H6-1 – H6-3)
- [X] T061 [US1] Implement result-event consumption in `apps/api/internal/events/results.go` — persist on success, classify failure, discard superseded and orphaned results with counters (M5-3, M5-4, FR-037)

### First capability

- [X] T062 [US1] Implement the `ghost` capability in `apps/ai/src/jobfinder_ai/capabilities/single/ghost.py` as a LangChain single call with Pydantic structured output (C2-1, FR-039, FR-003)
- [X] T063 [US1] Add the `ghost` prompt module in `apps/ai/src/jobfinder_ai/prompts/ghost.py`, ported from the Go implementation verbatim, retaining `response_format: {"type":"json_object"}` (C6-1, C6-4)
- [X] T064 [US1] Implement the FastStream consumer for `ghost.requested` and result publication in `apps/ai/src/jobfinder_ai/messaging/consumers.py`, acking only after the result event is confirmed published (M3-2)
- [X] T065 [US1] Add the input snapshot for `ghost` to the publish path in `apps/api/internal/ghostjob/`, with `snapshot_hash` computed at publish (E3-2, E3-4)
- [X] T066 [US1] Enforce the configured maximum message size at publish, failing explicitly with work id and size rather than truncating (E3-5)

### Tracing

- [X] T067 [US1] Emit one trace per run with the structure in data-model.md § 9 — name, `user_id`, `session_id`, metadata, tags — from `apps/ai/src/jobfinder_ai/tracing.py` (FR-012, FR-014)
- [X] T068 [US1] Emit a span per orchestration step with input, output, duration, model tier, token counts and cost (FR-012)
- [X] T069 [US1] Emit traces for failed runs, marked failed, showing the failing step, its error and each retry attempt (FR-013)
- [X] T070 [US1] Send the trace id as request metadata on the gateway call so LiteLLM's own Langfuse record correlates with the run trace (FR-017, research R8)
- [X] T071 [US1] Record `workflow_version` (the service's committed revision) and `snapshot_hash` in trace metadata (FR-015)

### Tests for User Story 1

- [X] T072 [P] [US1] Integration test in `apps/ai/tests/integration/test_tracing.py`: a successful run produces exactly one trace with one span per step, each carrying input, output, duration, tier, tokens and cost (US1 scenario 1)
- [X] T073 [P] [US1] Integration test: a failed run still produces a trace, marked failed, naming the failing step and showing retries (US1 scenario 2, FR-013)
- [X] T074 [P] [US1] Integration test: traces are findable by user, job and capability (US1 scenario 3, SC-001, SC-002)
- [X] T075 [P] [US1] Integration test with the collector stopped: runs complete and median latency is unchanged within noise (US1 scenario 4, SC-006, FR-016)
- [X] T076 [P] [US1] Integration test with the AI service stopped: work waits in the queue, is processed on restart, and nothing hangs or returns a substituted result (SC-007, US4 scenario 3)
- [X] T077 [US1] Capture the `ghost` pre-migration baseline over a fixed input set before cutover, stored in `specs/047-langchain-ai-service/baseline-ghost.json` (FR-021, C8-1)
- [X] T078 [US1] Verify post-cutover `ghost` output stays within tolerance — ≤5% mean deviation, ≤5% outcome changes (SC-004, C8-2)
- [ ] T079 [US1] Delete the Go `ghost` LLM path in `apps/api/internal/ghostjob/` once cutover is confirmed (FR-023, C8-4)

**Checkpoint**: US1 fully demonstrable. SC-001, SC-002, SC-006, SC-007 verifiable on one capability.

---

## Phase 5: User Story 2 — Change an AI workflow without a backend release (P2)

**Goal**: A prompt or step change takes effect by editing `apps/ai` and restarting that one
service, with the change visible in subsequent traces.

**Independent Test**: Change one prompt and add one step to a migrated capability, restart only
the AI service, confirm the new behaviour takes effect and appears in traces with no backend
rebuild.

- [X] T080 [US2] Enforce in-repo-only definitions in `apps/ai/src/jobfinder_ai/capabilities/registry.py` — no runtime fetch from a registry, database or remote source (FR-015a, C6-2)
- [X] T081 [US2] Fail startup naming the invalid definition when a workflow or prompt module is malformed, missing or has non-positive bounds (FR-007, US2 scenario 3)
- [X] T082 [US2] Resolve `workflow_version` from the committed revision at build time in `apps/ai/src/jobfinder_ai/tracing.py`, so before/after runs are distinguishable (FR-015, US2 scenario 2)
- [X] T083 [P] [US2] Test in `apps/ai/tests/unit/test_registry.py`: an invalid definition fails startup rather than failing at request time (US2 scenario 3)
- [X] T084 [P] [US2] Test: two runs across a definition change record different `workflow_version` values (US2 scenario 2)
- [X] T085 [P] [US2] Document the prompt-change workflow — edit, `docker compose restart ai`, verify in trace — in `docs/docs/ai/orchestration.md` (SC-003)

**Checkpoint**: SC-003 measurable. Prompt iteration no longer requires a backend release.

---

## Phase 6: User Story 3 — Multi-step and tool-using capabilities keep their guarantees (P2)

**Goal**: `salary` (bounded tool loop) and the generation pipeline (state graph) run under
LangGraph with today's guarantees intact.

**Independent Test**: Run both against a fixed input set; results stay schema-valid and within
baseline tolerance, bounds hold, and each intermediate step is its own span.

### Salary — LangGraph bounded loop

- [ ] T086 [US3] Implement the `salary` bounded agent loop in `apps/ai/src/jobfinder_ai/capabilities/graphs/salary.py` with `max_tool_rounds` and `max_nodes` enforced by the LangGraph runtime, not by counters (C4-1, C4-2, FR-041)
- [ ] T087 [US3] Port the salary tools to `apps/ai/src/jobfinder_ai/capabilities/graphs/salary_tools.py` as read-only, side-effect-free lookups (C5-2)
- [ ] T088 [US3] Enforce untrusted handling of tool output, preserving the property `apps/api/internal/platform/llm/application/toolloop/untrusted.go` enforces today (FR-006, C5-1)
- [ ] T089 [US3] Set `salary` bounds at or above the existing `toolloop.Bounds` values so behaviour cannot silently tighten (C4-5)
- [ ] T090 [US3] Emit a span per tool call and per tool result (C5-3, US3 scenario 2)

### Generation — LangGraph state graph

- [ ] T091 [US3] Implement the generation state graph in `apps/ai/src/jobfinder_ai/capabilities/graphs/generation.py` — analyze → select (standard/premium) → summarize → assemble, with shared run state and conditional routing (C2-3)
- [ ] T092 [US3] Bind each stage node to its own task key (`generation-analyze`, `generation-select`, `generation-select-premium`, `generation-summary*`) so per-stage routing stays a `gateway/config.yaml` edit (C2-3, 030-FR-005)
- [ ] T093 [US3] Port the generation-stage prompts to `apps/ai/src/jobfinder_ai/prompts/generation/` verbatim (C6-1)
- [ ] T094 [US3] Enforce grounding — prompts draw only from the input snapshot, never fetching supplementary content (C6-3, Constitution II)
- [ ] T095 [US3] Declare `max_nodes`, per-node timeout and whole-run timeout for the generation graph, all recorded in the trace (C4-1, C4-4)
- [ ] T096 [US3] Set `failed_step` on every graph-capability failure so a stage failure names its stage (E5-4, FR-040, US3 scenario 1)
- [ ] T097 [US3] Produce `bound_exceeded` naming the bound when any bound is hit, never a truncated result (C4-3, E5-3, US3 scenario 2)

### Tests for User Story 3

- [ ] T098 [P] [US3] Test in `apps/ai/tests/unit/test_generation_graph.py`: each stage is a separate span with its own input and output, and a stage failure fails the run naming that stage (US3 scenario 1)
- [ ] T099 [P] [US3] Test in `apps/ai/tests/unit/test_salary_loop.py`: tool rounds stay within bound, each call and result is a span, and exceeding the bound ends the run with `bound_exceeded` (US3 scenario 2)
- [ ] T100 [P] [US3] Test: instruction-like text in a tool result does not alter the run's instructions (US3 scenario 3, FR-006)
- [ ] T101 [US3] Capture pre-migration baselines for `salary` and the generation pipeline before cutover (FR-021, C8-1)
- [ ] T102 [US3] Verify post-cutover parity for both against their baselines and confirm stored data shapes are unchanged (SC-004, FR-022, C8-2)
- [ ] T103 [US3] Delete the Go paths — `apps/api/internal/salary/application/service.go` tool loop and the generation-stage orchestration in `apps/api/internal/generation/` — once each cutover is confirmed (FR-023, C8-4)

**Checkpoint**: The two capabilities the feature exists for run under LangGraph, with bounds and grounding intact.

---

## Phase 7: User Story 4 — The migration is reversible per capability (P3)

**Goal**: Remaining capabilities migrate; each is revertible by configuration alone until its
Go path is deleted.

**Independent Test**: Cut one capability over, verify it runs through the service, revert it by
configuration, verify it runs the old path again — with other capabilities unaffected in both
directions.

### Remaining capability migrations

- [ ] T104 [P] [US4] Implement `match` in `apps/ai/src/jobfinder_ai/capabilities/single/match.py` with its prompt module, event consumer, and input snapshot on the publish path
- [ ] T105 [P] [US4] Implement `rephrase`, `recruiter` and `outreach` in `apps/ai/src/jobfinder_ai/capabilities/single/`, served over the interactive HTTP surface (C2, H1-1). `rephrase` serves two callers — the keyword path and coach chat — as one capability (C2-2a)
- [ ] T106 [P] [US4] Implement the three summary capabilities (`generation-summary`, `-premium`, `-fast`) in `apps/ai/src/jobfinder_ai/capabilities/single/summary.py`
- [ ] T107 [US4] Repoint `apps/api/internal/coach/application/service.go` at the `rephrase` capability through `internal/aiclient`, keeping `RephraseModel` as its interface. No graph, no streaming — coach makes one model call (spec.md § Which layer serves which capability)
- [ ] T108 [US4] Implement `embed` in `apps/ai/src/jobfinder_ai/capabilities/single/embed.py` over HTTP, migrated last as the highest-volume, lowest-benefit path (research R11, C2-2)
- [ ] T109 [US4] Add shared-secret authentication to the HTTP surface and reject unauthenticated requests with 401, accepting no end-user credentials (H7-1, H7-2)
- [ ] T110 [US4] Map failure categories to HTTP status codes per H4-2, returning `trace_id` on both success and failure (H4-2, H4-3)

### Reversibility and completion

- [ ] T111 [US4] Verify each capability reverts to its Go path by configuration alone, with other capabilities unaffected (US4 scenarios 1 and 2, SC-009, C8-3)
- [ ] T112 [US4] Capture and verify baselines for every remaining capability before and after its cutover (FR-021, SC-004)
- [ ] T113 [US4] Delete each capability's Go LLM path once its cutover is confirmed, capability by capability (FR-023, C8-4)
- [ ] T114 [US4] Assert every one of the fourteen task keys is requested by exactly one capability, and that the backend retains no direct model or embedding call path (FR-019a, C1-3)
- [ ] T115 [US4] Delete `apps/api/internal/platform/llm/` entirely, including `application/toolloop/` (FR-019a, SC-010)
- [ ] T116 [P] [US4] Test in `apps/ai/tests/integration/test_routing_switch.py`: a partially migrated system serves both sets of capabilities normally (US4 scenario 1)
- [ ] T117 [P] [US4] Test: an unreachable AI service produces a clear user-facing error with no substituted result (H6-4)
- [ ] T128 [US4] Extend T112's parity verification to compare stored **field sets and enum ranges** per capability, not only values (FR-022)
- [ ] T129 [P] [US4] Measure the `embed` HTTP-hop cost against SC-005 **before** T108's cutover; a breach is escalated as a spec question about FR-019a's totality, not relaxed (C2-2)

**Checkpoint**: All fourteen task keys served by the AI service. SC-009, SC-010 verifiable.

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T118 [P] Implement the Langfuse payload retention job purging trace inputs and outputs older than `LANGFUSE_PAYLOAD_RETENTION_DAYS` while retaining metadata, timings, tokens and cost (FR-018, FR-018a, K5)
- [ ] T119 [P] Verify no trace payload older than 30 days remains, and that older runs still show step sequence, timings and cost (SC-015, K5-3)
- [ ] T120 [P] Restrict trace access to operators; confirm traces are unreachable by end users or from outside the stack (FR-018b)
- [ ] T121 Measure end-to-end completion time per capability against pre-migration figures; confirm ≤10% median growth (SC-005)
- [ ] T122 Measure backend worker occupancy for AI work types before and after; confirm it drops to publish + persist time (SC-014)
- [ ] T123 [P] Verify zero provider credentials and zero database credentials are readable by the AI service (SC-008, K2-2, K2-3)
- [ ] T124 [P] Add the operator-facing account of broker topology, DLQ workflow and the AI service to `docs/docs/async/` and `docs/docs/ai/`
- [ ] T125 Fold this feature's durable requirements into `specs/domains/llm-routing.md` and `specs/domains/platform-operations.md`, then delete `specs/047-langchain-ai-service/` per the repository's no-archive convention
- [ ] T126 Run the full `quickstart.md` validation end to end
- [ ] T127 Run `make test-lint`, `make test-integration`, `make audit` and `make contracts-check` and confirm all pass

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies
- **Phase 2 (Foundational)**: Depends on Phase 1 — **blocks every user story**
- **Phase 3 (US5, P1)**: Depends on Phase 2. **Blocks US1, US2, US3, US4** — they consume work events that do not exist until US5 lands
- **Phase 4 (US1, P1)**: Depends on Phase 3
- **Phase 5 (US2, P2)**: Depends on Phase 4 — needs at least one migrated capability to change
- **Phase 6 (US3, P2)**: Depends on Phase 4 for the cutover mechanism; independent of US2
- **Phase 7 (US4, P3)**: Depends on Phase 4; T115 additionally depends on Phase 6 completing
- **Phase 8 (Polish)**: Depends on all desired stories

### The one departure from priority order

US5 is P1 and runs first, before US1 (also P1). Not a re-prioritization — a dependency. Every
other story's work arrives as an event, and events need the broker. Within the constraint,
priority order holds: US1 → US2 → US3 → US4.

### Within each user story

- Baselines are captured **before** cutover (T077, T101, T112), never after
- Cutover → parity verification → Go-path deletion, in that order. Deleting first removes the
  revert path FR-020 requires
- Tests marked [P] run in parallel; implementation tasks touching the same file do not

### Parallel Opportunities

- **Phase 1**: T003, T004, T005, T007, T008, T010, T011 in parallel
- **Phase 2**: T015, T018, T019 in parallel; T027, T028, T031, T032 in parallel; T038, T039, T040 in parallel
- **Phase 3**: all of T048–T058 in parallel once T041–T047 land
- **Phase 4**: T072–T076 in parallel
- **Phase 6**: T098, T099, T100 in parallel
- **Phase 7**: T104, T105, T106 in parallel (different capability files); T116, T117 in parallel
- **Phase 8**: T118, T119, T120, T123, T124 in parallel

---

## Parallel Example: Phase 3 operator surface and tests

```bash
# After T041–T047 land, launch together:
Task: "Add DLQ depth per work type to the health endpoint in apps/api/internal/health/"
Task: "Add broker unavailability as a distinct health signal"
Task: "Add queue metrics per work type in apps/api/internal/events/metrics.go"
Task: "Document the DLQ re-dispatch workflow in docs/docs/async/monitoring.md"
Task: "Integration test: work published while consumer stopped is processed on restart"
Task: "Integration test: consumer killed mid-processing produces exactly one stored result"
Task: "Integration test: retry budget exhaustion lands in dlq.<work_type>"
```

---

## Implementation Strategy

### MVP scope

**Two MVPs, in sequence** — a consequence of the feature carrying two migrations.

1. **Foundation MVP = Phases 1–3 (US5).** RabbitMQ replaces asynq; the platform is
   event-driven; asynq is gone. Delivers real value on its own — durability, dead-lettering,
   operator-inspectable queues — with zero AI change. If the AI migration were abandoned
   tomorrow, this still stands.
2. **AI MVP = Phase 4 (US1).** One capability (`ghost`) through the Python service with full
   tracing. Proves the whole pattern on the smallest possible surface.

Stop and validate at each. Neither commits you to what follows.

### Incremental delivery

Phases 1–2 → foundation ready → Phase 3 (US5) → **validate, deploy** → Phase 4 (US1) →
**validate, deploy** → Phase 5 (US2) → Phase 6 (US3) → Phase 7 (US4) → Phase 8 close-out.

Within Phases 4–7, capability order is fixed by risk: `ghost` → `match` and other single-call →
`salary` → generation pipeline → `embed` last.

### Parallel team strategy

Phases 1–3 are one workstream — the messaging migration touches shared files and does not
parallelize cleanly across people. From Phase 4 on:

- Developer A: US1 tracing, then US2
- Developer B: US3 graphs (independent files under `capabilities/graphs/`)
- Developer C: US4 single-call capability migrations (independent files under
  `capabilities/single/`)

T115 (delete `internal/platform/llm/`) is the join point — it needs every capability migrated.

---

## Notes

- **Split check**: 61 of 133 tasks (46%) are messaging (Phases 1–3); 72 are AI orchestration.
  `checklists/requirements.md` flagged splitting into two features if the count skewed to
  messaging. It is close to even, and Phase 3 is a clean, independently valuable checkpoint —
  so one feature holds, with the split available at that checkpoint if it stops feeling true.
- **Baselines gate cutover.** T077, T101 and T112 are not paperwork: without a recorded
  baseline, "behaviour unchanged" is a claim rather than a measurement (FR-021).
- **Delete the Go path only after confirming.** T079, T103, T113 come after parity checks —
  reversed, they remove the revert path FR-020 promises.
- `[P]` = different files, no dependency on an incomplete task
- Commit after each task or logical group
