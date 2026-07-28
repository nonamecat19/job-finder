---

description: "Task list for 019-ai-job-throughput"
---

# Tasks: AI Job Throughput & Stuck-Run Recovery

**Input**: Design documents from `/specs/019-ai-job-throughput/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Test tasks ARE included. Constitution IV makes per-language suites a hard gate,
and plan.md's Constitution Check commits to specific new coverage. `make test-lint` must
pass — this change spans `apps/api`, `packages/shared`, and `apps/dashboard`.

**Organization**: Grouped by user story so each ships and validates independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1..US4, mapping to spec.md user stories
- Exact file paths in every description

## Path Conventions

Monorepo, per plan.md Structure Decision: Go backend at `apps/api/`, shared TS types at
`packages/shared/src/`, dashboard at `apps/dashboard/src/`. Go tests live beside the code
they test (`*_test.go`), integration tests behind the existing build tag used by
`make test-integration`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Capture the pre-change baseline and register configuration. Nothing here
changes runtime behaviour.

- [ ] T001 Capture pre-change baseline per quickstart.md "Capture the baseline": drain a fixed 50-job match set, record total wall-clock, median `ActivityRun.elapsedMs`, and each job's `MatchResult.score` into `specs/019-ai-job-throughput/baseline.json`. MUST run on the current build before any code change — SC-001/SC-003 are unverifiable without it.
- [X] T002 Add concurrency, deadline, liveness, and local-performance fields to `apps/api/internal/config/config.go` per `contracts/config.md` (`AI_CONCURRENCY_CLOUD`, `AI_CONCURRENCY_LOCAL`, `INGEST_CONCURRENCY`, `ENRICH_CONCURRENCY`, `AI_TASK_TIMEOUT_*`, `ACTIVITY_HEARTBEAT_INTERVAL`, `ACTIVITY_STALE_AFTER`, `ACTIVITY_SWEEP_INTERVAL`, `ACTIVITY_QUEUED_GRACE`, `OLLAMA_KEEP_ALIVE`, `LLM_MAX_IDLE_CONNS_PER_HOST`)
- [X] T003 Register defaults for the new keys in the `defaults` map in `apps/api/internal/config/defaults.go` using the values in `contracts/config.md`
- [X] T004 [P] Document the new keys with their defaults and rationale in `.env.example`, next to the existing LLM block

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The task-policy value type and its validation. Every user story phase reads
policies; nothing else is shared.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T005 Create `apps/api/internal/queue/policy.go`: `TaskPolicy` struct (`TaskType`, `Queue`, `LocalConcurrency`, `HostedConcurrency`, `MaxDuration`, `LLMTaskKey`) per data-model.md §2, a `PoliciesFromConfig(*config.Config) ([]TaskPolicy, error)` builder covering all six task types, and `PoolSize()` returning `max(Local, Hosted)`
- [X] T006 Add validation in `apps/api/internal/queue/policy.go`: concurrencies ≥ 1, `MaxDuration` > 0, `ACTIVITY_STALE_AFTER` ≥ 2× `ACTIVITY_HEARTBEAT_INTERVAL`, and `ACTIVITY_STALE_AFTER + ACTIVITY_SWEEP_INTERVAL` < 5m (FR-009/SC-005); errors name the offending env key
- [X] T007 [P] Unit tests for policy construction and validation in `apps/api/internal/queue/policy_test.go`, including the < 5m bound and each rejection message
- [X] T008 Call `queue.PoliciesFromConfig` during startup in `apps/api/cmd/server/platform.go`, storing the result on `Platform` so `buildServers` and the sweeper both read one source

**Checkpoint**: Policies exist and are validated at startup — user stories can begin

---

## Phase 3: User Story 1 - Cloud-backed AI work runs in parallel (Priority: P1) 🎯 MVP

**Goal**: Hosted-provider AI tasks run 3 at a time per task type; local stays at 1; the
level follows the provider class resolved from live settings, with no restart.

**Independent Test**: quickstart.md Scenarios 1–2 — hosted `match` shows `active: 3` and
drains a 50-job set in ≤40% of the T001 baseline; flipping the task to a local model in
Settings drops in-flight to 1 without a restart.

### Tests for User Story 1

- [X] T009 [P] [US1] Provider-class tests in `apps/api/internal/llm/router_test.go`: local URL → `local`; loopback with `OLLAMA_KEY` set → `hosted`; `https://ollama.com` → `hosted`; Cerebras/OpenRouter with credential → `hosted`; Cerebras/OpenRouter *without* credential → class follows the Ollama fallback (research.md R2)
- [X] T010 [P] [US1] Guard test in `apps/api/internal/llm/factory_test.go` asserting no provider built by `llm.NewProviders` uses `retrieval.DefaultTransport` as its RoundTripper (FR-003, research.md R1)
- [X] T011 [P] [US1] Admission-gate unit tests in `apps/api/internal/queue/middleware_test.go`: hosted class admits `HostedConcurrency` simultaneously and blocks the next; local class admits `LocalConcurrency`; a waiting task honours ctx cancellation; class is resolved once and does not change mid-flight
- [X] T012 [P] [US1] Integration test in `apps/api/internal/matching/integration_test.go` running the same job through two concurrent match calls and asserting `UpsertMatchResult` converges to one consistent row (FR-017)
- [X] T013 [P] [US1] Rate-limit test in `apps/api/internal/llm/ollama_test.go`: three concurrent calls against a stub returning `429` + `Retry-After` all back off via the shared `rateLimitBreaker` and surface a retryable error, none permanent (FR-015, quickstart Scenario 7)

### Implementation for User Story 1

- [X] T014 [P] [US1] Add `IsHosted() bool` to `OllamaProvider` in `apps/api/internal/llm/ollama.go`: true when `apiKey != ""` or the base URL host is not loopback/private (research.md R2)
- [X] T015 [US1] Add `ProviderClass` type and `(*Router).ProviderClass() ProviderClass` in `apps/api/internal/llm/router.go`, reusing `resolve()` so a credential-less remote provider reports the *effective* (fallback) class (data-model.md §3)
- [X] T016 [US1] Create the admission gate in `apps/api/internal/queue/middleware.go`: per task type, one `hosted` and one `local` counting semaphore sized from `TaskPolicy`; ctx-aware acquire before the handler, release on return (data-model.md §4)
- [X] T017 [US1] Add a class resolver seam to the middleware so non-LLM task types (`ingest`, `enrich`, empty `LLMTaskKey`) bypass class resolution and use their single configured concurrency
- [X] T018 [US1] Rework `(*Platform).worker` in `apps/api/cmd/server/servers.go` to take a `queue.TaskPolicy`, size `asynq.Config.Concurrency` from `PoolSize()`, and wrap the handler in the middleware from T016
- [X] T019 [US1] Replace the six hardcoded concurrency literals in `buildServers` in `apps/api/cmd/server/servers.go` with policy lookups, and update the block comment above them to describe the gate instead of "concurrency=1"
- [X] T020 [US1] Pass each task's `llm.Router` (or nil) into worker construction in `apps/api/cmd/server/compose_tasks.go` / `apps/api/cmd/server/servers.go` so the middleware can resolve provider class per task
- [X] T021 [P] [US1] Give the LLM providers a tuned shared transport with `MaxIdleConnsPerHost = LLM_MAX_IDLE_CONNS_PER_HOST` in `apps/api/internal/llm/ollama.go`, `apps/api/internal/llm/cerebras.go`, and `apps/api/internal/llm/openrouter.go` — Go's default of 2 forces a fresh TLS handshake on the third concurrent request (research.md R5)

**Checkpoint**: Hosted AI work runs 3-wide, local unchanged, settings flips take effect live

---

## Phase 4: User Story 2 - Stuck runs are cancelled instead of hanging forever (Priority: P1)

**Goal**: Every run reaches a terminal state — `timed_out` when it exceeds its task-type
deadline, `interrupted` when its worker vanishes — within the deadline + 5 minutes.

**Independent Test**: quickstart.md Scenarios 3–5 and 8 — a 10s deadline yields `timed_out`
with elapsed recorded and no downstream enqueue; `docker compose kill -s SIGKILL api`
followed by restart leaves zero rows in `running` within 5 minutes; existing 10-hour ghosts
(with `heartbeatAt = null`) are swept on the first startup sweep.

### Tests for User Story 2

- [X] T022 [P] [US2] Sweeper unit tests in `apps/api/internal/activity/sweeper_test.go`: stale `running` → `interrupted`; `heartbeatAt = null` on a `running` row → `interrupted` (the existing-ghost case); fresh heartbeat untouched; terminal rows never re-opened; a row that finishes mid-sweep is not clobbered
- [X] T023 [P] [US2] Deadline middleware tests in `apps/api/internal/queue/middleware_test.go`: exceeding `MaxDuration` finalizes the run `timed_out` with elapsed and limit in `error`, returns a retryable error while retries remain, and triggers no downstream enqueue (FR-008)
- [X] T024 [P] [US2] Integration test in `apps/api/internal/activity/sweeper_integration_test.go` against real Postgres: insert `running` rows with stale/null heartbeats plus a fresh one, run one sweep, assert exactly the stale ones become `interrupted` with a reason string
- [X] T025 [P] [US2] Vitest coverage in `apps/dashboard/src/features/status/StatusPage.test.tsx` asserting `timed_out` and `interrupted` render distinctly from `failed`/`cancelled`, each showing its reason, and both appear in the retryable set (SC-007)

### Implementation for User Story 2

- [X] T026 [US2] Create migration `apps/api/internal/db/migrations/00030_activity_run_liveness.sql` adding `ActivityRun."heartbeatAt"` and `ActivityRun."timeoutMs"`, the partial index `ActivityRun_heartbeat_idx` on `("heartbeatAt") WHERE "state" = 'running'`, and `Job."embeddingHash"` (used by US3); goose version 30, unique and sequential per Constitution
- [X] T027 [US2] Add queries to `apps/api/internal/db/queries/activityrun.sql`: `TouchActivityRunHeartbeat`, `FinishActivityRunTimedOut`, `SweepStaleRunningActivityRuns` (by `heartbeatAt` older than a cutoff, or null, filtered to `state = 'running'`), and `ListStaleQueuedActivityRuns` (data-model.md §1.2)
- [X] T028 [US2] Widen `ListFailedActivityRuns` in `apps/api/internal/db/queries/activityrun.sql` to `state IN ('failed','cancelled','timed_out','interrupted')` so the existing retry flow accepts the new states (FR-012)
- [X] T029 [US2] Set `startedAt`/`heartbeatAt` in `StartActivityRun` and refresh `heartbeatAt` in `SetActivityStep` in `apps/api/internal/db/queries/activityrun.sql`, then regenerate sqlc (do not hand-edit `sqlcgen`)
- [X] T030 [US2] Extend the `Store` interface and `Recorder` in `apps/api/internal/activity/recorder.go` with `Heartbeat(ctx)` and `TimedOut(ctx, elapsed, limit)`, keeping the existing nil-Recorder tolerance
- [X] T031 [US2] Add the heartbeat ticker and per-task `context.WithTimeout` to `apps/api/internal/queue/middleware.go` (same wrapper as T016): tick at `ACTIVITY_HEARTBEAT_INTERVAL` while the handler runs; on `ctx.DeadlineExceeded` finalize `timed_out` with elapsed and the configured limit
- [X] T032 [US2] Record `timeoutMs` on the run at start, from the task policy, so the UI and sweeper can report which limit applied (data-model.md §1)
- [X] T033 [US2] Create `apps/api/internal/activity/sweeper.go`: `Sweeper` running once at startup then every `ACTIVITY_SWEEP_INTERVAL`; marks stale `running` rows `interrupted` with the reason format from data-model.md §1.3, and `queued` rows past `ACTIVITY_QUEUED_GRACE` with no live asynq task (Inspector lookup by `queueTaskId`) likewise
- [X] T034 [US2] Wire the sweeper into startup and graceful shutdown in `apps/api/cmd/server/platform.go` and `apps/api/cmd/server/servers.go` (`runServers`), alongside the ingestion scheduler goroutine
- [X] T035 [P] [US2] Add `'timed_out'` and `'interrupted'` to `ACTIVITY_STATES` and add `heartbeatAt` / `timeoutMs` to `ActivityRunDto` in `packages/shared/src/index.ts` (Constitution III: single source of truth), then rebuild shared
- [X] T036 [US2] Mirror the DTO change in the Go activity DTO and mapper (`apps/api/internal/dto`, `apps/api/internal/httpapi/activity.go`) so `heartbeatAt`/`timeoutMs` reach the API response
- [X] T037 [US2] Render the new states in `apps/dashboard/src/features/status/StatusPage.tsx`: `timed_out` as a danger variant, `interrupted` as a warning variant, both showing `error` as the reason and both included in the failed/retryable grouping at lines 44–49

**Checkpoint**: No run can outlive its deadline + 5 minutes; existing ghosts cleared

---

## Phase 5: User Story 3 - Local model matching completes noticeably faster (Priority: P2)

**Goal**: Median per-job local match latency ≥30% below the T001 baseline, with scores
unchanged beyond tolerance and zero feature-threshold flips.

**Independent Test**: quickstart.md Scenario 6 — re-run the same 50-job benchmark set
against a local model, compare median elapsed and per-job scores to `baseline.json`; a
second run over the same set performs no embedding calls.

### Tests for User Story 3

- [X] T038 [P] [US3] Profile snapshot cache tests in `apps/api/internal/profile/snapshot_test.go`: hit returns identical text; a newer profile `updatedAt` invalidates; an explicit write invalidates (Constitution II — stale profile text must never reach a prompt)
- [X] T039 [P] [US3] Embedding-skip tests in `apps/api/internal/matching/service_test.go`: unchanged content hash skips `Embed` and reuses the stored vector; changed description recomputes; null hash always recomputes
- [X] T040 [P] [US3] Test in `apps/api/internal/llm/ollama_test.go` that `keep_alive` is present on chat and embed request bodies when `OLLAMA_KEEP_ALIVE` is set, and absent when it is empty

### Implementation for User Story 3

- [X] T041 [P] [US3] Create `apps/api/internal/profile/snapshot.go`: versioned cache of `{ProfileID, ProfileText, HasEmbedding, Version}` per data-model.md §5, built from `GetDefault` + `masterFor` + `RendercvToText` + extra notes, truncated to 6000 exactly as `scoring.go:64` does today
- [X] T042 [US3] Invalidate the snapshot on profile mutation in `apps/api/internal/profile/service.go` (`Update`, `SaveConfig`, `UpdateResume`, `RefreshEmbedding`)
- [X] T043 [US3] Use the snapshot in `apps/api/internal/matching/service.go` and `apps/api/internal/matching/scoring.go` in place of the per-job `GetDefault` + `MasterFromConfig` + `RendercvToText` + `HasEmbedding` sequence
- [X] T044 [US3] Add `UpdateJobEmbeddingWithHash` and expose `embeddingHash` on job reads in `apps/api/internal/db/queries/job.sql`, then regenerate sqlc
- [X] T045 [US3] Skip re-embedding in `apps/api/internal/matching/scoring.go` when the hash of the exact embed text (`title|company|description`, truncated to 8000) matches the stored `embeddingHash`; store the hash whenever the embedding is written (data-model.md §6)
- [X] T046 [US3] Send `keep_alive` on chat and embed requests in `apps/api/internal/llm/ollama.go`, from `OLLAMA_KEEP_ALIVE`, omitting the field entirely when the value is empty (research.md R5)
- [ ] T047 [US3] Re-run the Scenario 6 benchmark against `baseline.json` and record the measured median delta and score drift in `specs/019-ai-job-throughput/baseline.json` alongside the before figures; if the 30% target is missed, note the gap and the remaining levers rather than silently accepting it

**Checkpoint**: Local matching measurably faster with match quality intact

---

## Phase 6: User Story 4 - Backlog progress is visible (Priority: P3)

**Goal**: Pending/in-flight counts per AI task type, plus a throughput-derived ETA.

**Independent Test**: With a large backlog draining,
`GET /api/activity/queues` reports per-queue counts, the effective concurrency, the
resolved provider class, and an ETA that moves as the queue drains.

### Tests for User Story 4

- [X] T048 [P] [US4] Handler tests in `apps/api/internal/httpapi/activity_test.go` with a fake inspector: fixed queue ordering (ingest, match, generate, enrich, salary, ghost); `providerClass: null` for non-LLM queues; `etaSeconds: null` when throughput or pending is 0; a single failing queue yields a per-entry `error` rather than a failed response; total Inspector failure yields 503

### Implementation for User Story 4

- [X] T049 [P] [US4] Add `QueueBacklogDto` to `packages/shared/src/index.ts` per `contracts/activity-api.md` §2, then rebuild shared
- [X] T050 [US4] Extend the `ActivityInspector` interface in `apps/api/internal/httpapi/activity.go` with the queue-info methods and implement `GET /activity/queues`, mounted next to the existing activity routes at lines 67–71
- [X] T051 [US4] Compute `concurrency` and `providerClass` per queue from the task policies and each task's `llm.Router.ProviderClass()` (US1's T015), and derive `etaSeconds` from pending ÷ processed-per-minute
- [X] T052 [P] [US4] Add the `activity.queues` client call in `apps/dashboard/src/lib/api.ts` next to the existing activity calls at lines 207–215
- [X] T053 [US4] Add a backlog panel to `apps/dashboard/src/features/status/StatusPage.tsx` showing per-queue pending/active, effective concurrency, provider class, and ETA

**Checkpoint**: Backlog state legible from the dashboard

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T054 [P] Update `README.md` / `AGENTS.md` where worker concurrency is described, so "concurrency 1, local LLM handles one at a time" no longer contradicts the shipped behaviour
- [X] T055 [P] Verify the stale comments in `apps/api/internal/queue/queue.go` (lines 25–36) and `apps/api/internal/matching/handler.go` (lines 37–40) describing concurrency 1, and rewrite them to describe the policy + gate
- [ ] T056 Run the full quickstart.md suite (Scenarios 1–8) against a real stack and record results
- [X] T057 Run `make test-lint` (Go + vitest) and confirm green — required gate for a change spanning `apps/api`, `packages/shared`, and `apps/dashboard`
- [ ] T058 Confirm FR-018 by starting the stack with no `OLLAMA_KEY`, `CEREBRAS_API_KEY`, or `OPENROUTER_API_KEY` and draining a small batch — everything must resolve local and behave as before (Constitution V)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: T001 must precede all code changes (baseline is destroyed by them). T002–T004 have no dependencies.
- **Foundational (Phase 2)**: Depends on T002/T003 — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Phase 2
- **US2 (Phase 4)**: Depends on Phase 2; T031/T032 extend the middleware file created by US1's T016, so US2 lands after US1 or coordinates on that file
- **US3 (Phase 5)**: Depends on Phase 2 and on US2's migration T026 (which carries `Job.embeddingHash`); otherwise independent
- **US4 (Phase 6)**: Depends on Phase 2 and on US1's T015 for `providerClass`
- **Polish (Phase 7)**: After the stories you intend to ship

### User Story Dependencies

- **US1 (P1)**: Independent — the MVP
- **US2 (P1)**: Independent in behaviour; shares one file (`queue/middleware.go`) with US1
- **US3 (P2)**: Needs the migration from US2 only for the `embeddingHash` column; can be split out into its own migration if US2 is deferred
- **US4 (P3)**: Needs US1's `ProviderClass` to report the effective concurrency

### Within Each User Story

- Tests before implementation; they must fail first
- Migration → sqlc queries → regenerate → Go code that consumes the generated types
- `packages/shared` types before the Go DTO and dashboard consumers (Constitution III)
- Story complete and validated before moving to the next priority

### Parallel Opportunities

- T004 runs alongside T002/T003
- All US1 tests (T009–T013) run in parallel — separate files
- T014 and T021 are parallel to each other and to the test tasks
- All US2 tests (T022–T025) run in parallel
- T035 (shared types) is parallel to the Go-side sweeper work, and unblocks T036/T037
- All US3 tests (T038–T040) run in parallel; T041 is parallel to them
- T049 and T052 are parallel to the US4 handler work

---

## Parallel Example: User Story 1

```bash
# Tests first, all in different files:
Task: "Provider-class tests in apps/api/internal/llm/router_test.go"
Task: "Paced-transport guard test in apps/api/internal/llm/factory_test.go"
Task: "Admission-gate tests in apps/api/internal/queue/middleware_test.go"
Task: "Concurrent-match convergence test in apps/api/internal/matching/integration_test.go"
Task: "429 backoff under parallel load in apps/api/internal/llm/ollama_test.go"

# Then the two independent implementation tasks:
Task: "Add IsHosted() to apps/api/internal/llm/ollama.go"
Task: "Tune MaxIdleConnsPerHost across apps/api/internal/llm/{ollama,cerebras,openrouter}.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 Setup — **T001 first, on the unchanged build**
2. Phase 2 Foundational
3. Phase 3 US1
4. **STOP and VALIDATE**: quickstart Scenarios 1–2 against `baseline.json`
5. Ship — this alone turns a 700-item backlog from serial into 3-wide

### Incremental Delivery

1. Setup + Foundational → policies validated at startup
2. US1 → hosted parallelism → validate → ship (MVP)
3. US2 → deadlines + recovery → validate → ship (clears the 10-hour ghosts)
4. US3 → local speedup → validate against baseline → ship
5. US4 → backlog visibility → ship

### Notes

- [P] tasks touch different files with no incomplete dependencies
- `sqlcgen` is generated: change `.sql` and regenerate, never hand-edit (Constitution III)
- Goose version 30 must be unique and sequential
- Commit per task or per logical group; stop at any checkpoint to validate independently
- The baseline in T001 is the only irreversible ordering constraint in this plan
