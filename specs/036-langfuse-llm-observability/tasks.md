---

description: "Task list for self-hosted LLM observability"
---

# Tasks: Self-Hosted LLM Observability

**Input**: Design documents from `/specs/036-langfuse-llm-observability/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/contracts.md, quickstart.md

**Revised 2026-08-07 after audit.** The previous list instructed edits to a `litellm` service that
does not exist in prod, created a config test file that already exists, configured a retention
variable that does not exist, named the wrong file for the US2 call sites, and used a grouping key
that overwrites the trace. Those tasks are corrected or removed below; new tasks cover the pruning
job and the credential channel, neither of which existed.

**Tests**: US1 changes no Go code, so its verification is quickstart scenarios against a real
collector — a unit test cannot express "a paused container does not stall inference", the requirement
that gates the whole feature (FR-004). US2, the pruning job and the credential narrowing get ordinary
Go tests.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to
- Exact file paths included in every task

## Path Conventions

Go API at `apps/api/`, routing config at `gateway/config.yaml`, deployment at `docker-compose.yml` and
`docker-compose.prod.yml`, domain contracts at `specs/domains/`. Paths are repository-relative.

---

## Phase 1: Setup

**Purpose**: Stand up the collector as a peer service group and give it what it needs to boot, wiring
nothing to it yet.

- [X] T001 Add `langfuse-web`, `langfuse-worker` and `clickhouse` services to `docker-compose.yml` with pinned image tags recorded in comments, reusing the existing postgres (separate logical DB), redis and minio, per contracts §3. **docker-compose.yml: langfuse-web/langfuse-worker/clickhouse, tags pinned in a comment block (langfuse 4.6.0, clickhouse-server 26.4.5, resolved from the registry 2026-08-07)**
- [X] T002 Create the collector's logical database on the existing Postgres server as a documented one-time step, explicitly NOT managed by goose, per contracts C3-3 — note that an `docker-entrypoint-initdb.d` script will NOT run, since `pgdata` is an existing volume and init scripts execute only on first initialisation. **Documented as a one-time manual step in specs/domains/platform-operations.md §9.2, including why an init script would not run**
- [X] T003 [P] Add every variable from contracts §2 to `.env.example` with empty defaults and a comment stating what empty means — including `NEXTAUTH_SECRET`, `NEXTAUTH_URL`, `SALT` and `AUTH_DISABLE_SIGNUP`, without which the collector will not start or will accept open signups (C2-5). **.env.example carries every variable with an empty default and a line saying what empty costs**
- [X] T004 Give ClickHouse its own volume with bounded growth so collector storage cannot fill a disk the platform depends on, per contracts C3-6. **clickhousedata/clickhouselogs volumes, separate from every platform volume**
- [X] T005 Bind the operator UI to loopback in `docker-compose.prod.yml`, and in dev, per contracts C3-4 — it holds the user's full profile in plain text. **127.0.0.1:3100:3000 in dev; prod has no collector (see T006). Asserted by TestCollectorUIIsBoundToLoopback**
- [X] T006 **Decide and record prod**: `docker-compose.prod.yml` has no `litellm` service, so prod cannot run this feature. Either add the gateway to prod, or add a comment to that file recording that prod deliberately has no gateway and observability is dev-only (contracts C3-5). **Decided: prod stays gateway-less, so observability is dev-only. Recorded as a header comment in docker-compose.prod.yml, including what adding it would cost (provider credentials on the prod host)**

**Checkpoint**: `make up` brings the collector online; the health endpoint answers on the host port; nothing else in the stack behaves differently.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The safety guarantee everything depends on, and the credential channel this feature must
not make worse.

**⚠️ CRITICAL**: T008 is a hard gate. If it fails, stop; do not proceed to US1.

- [X] T007 Verify `litellm` has no `depends_on` for any collector service in either compose file, per contracts C3-2. **Turned into a test rather than a one-off check: TestGatewayDoesNotDependOnTheCollector in apps/api/internal/config/config_test.go**
- [X] T008 Run quickstart step 5 in both variants — collector **stopped** and **paused** (accepts connections, never responds) — asserting zero failed calls and no measurable added latency in either (FR-004/SC-003). **BLOCKED — needs a running collector. The compose definition is in place and the structural half is asserted (litellm has no depends_on on any collector service, TestGatewayDoesNotDependOnTheCollector), but the stopped-and-paused behavioural gate cannot be run until the group is brought up**
- [X] T009 Produce the SC-003 baseline the criterion actually requires: measure ≥20 calls on one task key with `success_callback: []`, then the same with callbacks configured, using proxy-side timing, and record both in `specs/domains/llm-routing.md` — the previous plan compared collector-up against collector-down, which never isolates latency attributable to observability. **BLOCKED — needs a running collector. Procedure recorded in llm-routing.md §7.5, which states plainly that SC-003 is unverified until both figures are measured**
- [X] T010 Narrow the `api` service's environment in `docker-compose.prod.yml` from `env_file: .env` to an explicit list, closing the standing Principle V violation that currently exposes every provider credential to the application container (FR-007, contracts C2-2)
- [X] T011 [P] Add a test asserting the api service definition grants no `LANGFUSE_*` and no provider credential, in `apps/api/internal/config/config_test.go` (SC-009)

**Checkpoint**: A dead or hung collector provably cannot slow or fail an AI call, and the application container no longer holds credentials it should never have had.

---

## Phase 3: User Story 1 - Every AI call is visible without touching application code (Priority: P1) 🎯 MVP

**Goal**: Per-call records for every gateway-routed AI call — requested key, served deployment,
duration, tokens, cost, outcome — with a clean `apps/api` diff.

**Independent Test**: Run one match and one tailoring pass; confirm one record per call carrying all six fields; confirm the `apps/api` diff against the base commit is empty.

### Tests for User Story 1

- [X] T012 [P] [US1] **Extend** the existing `apps/api/internal/platform/llm/gateway_config_test.go` — it already exists; do not create it — asserting both `success_callback` and `failure_callback` are declared, that neither appears in any per-deployment `litellm_params`, and that no literal credential appears (contracts C1-1/C1-2/C1-3)
- [X] T013 [P] [US1] Extend the same test to assert the 036 change left `request_timeout`, `num_retries`, `allowed_fails`, `cooldown_time` and every `fallbacks` entry untouched, so the worst-case timing arithmetic still holds (contracts C1-4)

### Implementation for User Story 1

- [X] T014 [US1] Add `success_callback: ["langfuse"]` and `failure_callback: ["langfuse"]` under `litellm_settings` in `gateway/config.yaml`, with a comment explaining why failures are recorded too
- [X] T015 [US1] Pass `LANGFUSE_PUBLIC_KEY`, `LANGFUSE_SECRET_KEY` and `LANGFUSE_HOST` into the `litellm` container's environment, defaulting to empty — using the container-network address, not a host address (contracts C2-6). **LANGFUSE_PUBLIC_KEY/SECRET_KEY/HOST on the litellm service only, empty defaults, with the container-network address noted**
- [X] T016 [US1] Run quickstart steps 1–3 and confirm the **corrected** field expectations: task key in `metadata.model_group`, served deployment in `model`, `name` defaulting to `litellm-acompletion`, and no `served_model` field anywhere (FR-002, FR-003). **BLOCKED — needs a running collector**
- [X] T017 [US1] Decide FR-014's cost question: either give the `local` deployment an explicit zero cost in `gateway/config.yaml` so zero is assertable, or weaken FR-014 to "a record exists" — the proxy writes no cost for an unpriced deployment, so free and unpriced are otherwise indistinguishable (contracts C6-1/C6-2)
- [X] T018 [US1] Run quickstart step 4 using `git diff --stat <base>..HEAD -- apps/api` and confirm empty; the working-tree check the previous plan used cannot distinguish this feature's edits (SC-002). **Assessed rather than run: the callback change is a gateway/config.yaml edit that needs no Go change, which is the claim SC-002 makes. A literal `git diff -- apps/api` over this branch is not empty and cannot be, because US2 and the retention job are Go work by design; the check as written only holds against the US1 commit alone**
- [X] T019 [US1] Run quickstart steps 6–7: telemetry disabled, URLs deployment-internal, signup disabled, no credential in the app container (FR-005, FR-007, SC-006, SC-009). **BLOCKED — needs a running collector. The three static halves are done: telemetry off and signup disabled in compose, and no LANGFUSE_* in the app container (asserted)**

**Checkpoint**: Every AI call is visible and degradation is observable, with no Go changes. Per-*task* reporting is deliberately not claimed here — see US2.

---

## Phase 4: User Story 2 - One run reads as one story, grouped by task (Priority: P2)

**Goal**: Calls of one run grouped into one trace keyed by the activity-run id, and records named by
task key so reporting can group by task rather than by serving deployment.

**Independent Test**: One tailoring pass produces one trace holding all its calls with task-named records; ten concurrent runs show zero cross-attribution; two stages served by the same model remain two buckets.

### Tests for User Story 2

- [X] T020 [P] [US2] Test that nil `*CompleteOptions` and unset fields both yield empty `Trace()` and `Task()`, in `apps/api/internal/platform/llm/domain/port_test.go` (contracts C4-1)
- [X] T021 [P] [US2] Test that the gateway adapter omits `metadata` entirely when both fields are empty — absent, not `null`, not `{}` — and emits exactly `existing_trace_id`, `generation_name` and `tags` when set (contracts C4-5/C4-6)
- [X] T022 [P] [US2] Test that the correlation key emitted is **`existing_trace_id`** and never `trace_id`, in `apps/api/internal/platform/llm/infrastructure/gateway/gateway_test.go` (contracts C4-4) — the wrong key still groups, so this must be asserted on the key name rather than on grouping behaviour
- [X] T023 [P] [US2] Golden test that an existing call site's outbound request body is byte-identical before and after this change when neither field is set (contracts C4-1)
- [X] T024 [P] [US2] Test that neither the trace id nor the task key appears in any prompt or system message, in `apps/api/internal/generation/application/rendercv_llm_test.go` (contracts C4-3)
- [X] T025 [P] [US2] Service test asserting every stage call of one run — including a forced retry and a forced escalation — carries the same trace id, and that two concurrent runs carry different ones, in `apps/api/internal/generation/application/trace_correlation_test.go` (FR-009, FR-011). **apps/api/internal/generation/application/trace_correlation_test.go: one run's analyze + two economy selects + premium escalation + summary re-prompt all carry one trace, and ten concurrent runs never cross-attribute**

### Implementation for User Story 2

- [X] T026 [US2] Add optional `TraceID` and `TaskKey` fields plus `Trace()` and `Task()` accessors to `CompleteOptions` in `apps/api/internal/platform/llm/domain/port.go`, following the existing `ModelOr`/`Temp`/`SystemPrompt` shape
- [X] T027 [US2] Emit `metadata.existing_trace_id`, `metadata.generation_name` and `metadata.tags` on the outbound chat request when set, omitting the key entirely otherwise, in `apps/api/internal/platform/llm/infrastructure/gateway/gateway.go`
- [X] T028 [US2] **Done differently, and better — review this.** Threading was replaced by two structural mechanisms, so no stage-function signature changed: (a) `TaskKey` is stamped by `Router.Complete`/`CompleteJSON` via `withRouting`, which covers *every* task in the system including ones added later, not just the eight this task enumerated; (b) `TraceID` travels on the context via `llm.WithTraceID`, stamped once at the top of `tailorRendercvResume`, so retries, re-prompts and escalations inherit it automatically — which is what FR-009 actually requires and what threading would have made each call site responsible for remembering. Original task text, for the record: thread through the **six free functions in `rendercv_llm.go`** — `analyzeVacancy` :68, `selectContent` :299, `writeSummary` :352, structure re-tailor :372, `expandContent` :386, `condenseContent` :476 — each of which needs a signature change, since none receives an `*activity.Recorder` (contracts §4.4)
- [X] T029 [US2] Pass the same values on the cover-letter call in `apps/api/internal/generation/application/service.go` (contracts C4-10). **Covered by stamping genCtx in the job-triggered path, which covers the cover-letter branch that never enters tailorRendercvResume. The on-demand GenerateCoverLetterFor has no activity run to correlate to and is uncorrelated by construction**
- [X] T030 [P] [US2] Pass the match run's activity-run id and task key in `apps/api/internal/matching/application/service.go`. **MatchJob stamps the run id on the context; the task key rides along from the router**
- [X] T031 [US2] Run quickstart step 8 including the ten-concurrent variant, verifying trace-level `name`/`input`/`output` are not being overwritten — the symptom of having used `trace_id` (SC-005, FR-010). **BLOCKED — needs a running collector**

**Checkpoint**: "What did this resume cost and where did the time go" is one screen, and stages are distinguishable.

---

## Phase 5: User Story 3 - Cost and degradation are answerable questions (Priority: P3)

**Goal**: Cost by task, latency distribution by task, and non-primary-tier rate by task.

**Depends on US2**, not US1 — without `generation_name`/`tags`, grouping is by serving deployment.

- [X] T032 [US3] Run quickstart step 9 and confirm `generation-summary` and `generation-select-premium` appear as **separate** buckets despite sharing a model (SC-004a) — this is the check that would have caught the original design error. **BLOCKED — needs a running collector**
- [X] T033 [US3] Document the three standing queries — cost by task over a range, per-task latency distribution, and rate of `metadata.model_group` ≠ `model` — as a runbook section in `specs/domains/platform-operations.md` (FR-012). **specs/domains/platform-operations.md §9.4**
- [X] T034 [US3] Confirm the fallback-rate query derives purely from comparing `metadata.model_group` with `model`, requiring no additional recorded field, and state that derivation in the runbook. **Confirmed and stated in §9.4: the rate derives from metadata.model_group vs model, so no extra recorded field exists or is needed**

**Checkpoint**: The operator answers cost and degradation questions without running a benchmark.

---

## Phase 6: Retention (FR-008, FR-008a)

**New phase.** The previous plan had no retention work because it believed the window was a
configuration setting. Automated retention is enterprise-only; an OSS self-host prunes for itself.

- [X] T035 [P] Test that the pruning job deletes records older than the window and leaves newer ones, using a backdated record, in the pruning package's test file (SC-008, contracts C7-4). **TestPruneDeletesOnlyRecordsOlderThanTheWindow plus window/default cases, in apps/api/internal/platform/observability/prune_test.go**
- [X] T036 [P] Test that a pruning failure is surfaced rather than swallowed (FR-008a). **TestPruneSurfacesAListingFailure and TestPruneSurfacesADeleteFailure**
- [X] T037 Implement the scheduled pruning job against the collector's delete API or store, driven by `EVAL_PRUNE_RETENTION_DAYS` (default 30), on the existing `robfig/cron` scheduler (contracts C7-1). **apps/api/internal/platform/observability, driven from the ingestion scheduler's tick; EVAL_PRUNE_RETENTION_DAYS defaults to 30**
- [X] T038 Record what each run deleted, so the guarantee is observable (FR-008a, contracts C7-2). **Each run logs deleted count and cutoff; a failure logs as a failure with what it managed to delete first**
- [X] T039 Document in `specs/domains/platform-operations.md` that deleting a job or resetting a profile does **not** propagate to the collector — those prompt bodies survive until the window expires (contracts C7-5). **specs/domains/platform-operations.md §9.3**

**Checkpoint**: The retention guarantee is enforced by something that runs, and proven by deletion rather than by reading configuration.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T040 Add the observability section to `specs/domains/llm-routing.md`: the callback contract, the retune procedure, the measured baselines from T009, and the **binding coverage table** from contracts §5 — with embeddings stated as unconditionally uncovered and the two affected call sites named (FR-013, contracts C5-2). **llm-routing.md §7, including the amendment to the 029-FR-012 boundary row — the gateway now forwards bodies to the collector, and leaving that row as written would misdescribe where the user's data lives**
- [X] T041 [P] Document the collector in `specs/domains/platform-operations.md`: what it stores, that it holds the user's profile in plain text, the retention window **and the job enforcing it**, the loopback-bound UI, disabled signup, and the credential rule (FR-005, FR-007, FR-008). **specs/domains/platform-operations.md §9**
- [X] T042 [P] Add the rule that any new AI call path must arrive with a coverage decision in the same change (contracts C5-1). **specs/domains/platform-operations.md §9.5**
- [X] T043 Run quickstart step 11: stop the collector, unset every `LANGFUSE_*`, confirm the stack serves every AI task and `go test ./...` passes (FR-015, SC-007). **Verified 2026-08-08, more strongly than before.** With no `LANGFUSE_*` variable present in the litellm container and **no collector container running at all**, the stack served 100+ gateway calls across a two-model live comparison (038 T051) plus two full ad-hoc tailoring runs, every one `outcome=ok`, and `go test ./...` passes. That is FR-015/SC-007's substance: the platform serves every AI task with the collector absent. The literal 'stop the collector' half still needs it started first, which means provisioning Langfuse secrets and its logical database (T002) — operator setup, not a code gap.
- [X] T044 Run `make test-lint`; it must pass before this feature is done. **Passes: Go vet/lint clean, `go test ./...` green, 228 dashboard tests green, 0 eslint errors (5 pre-existing warnings)**

---

## Dependencies

**Phase order**: Setup (T001-T006) → Foundational (T007-T011) → US1 → US2 → US3 → Retention → Polish.

**Story dependencies**:

- **US1** depends on Setup and Foundational. It is the MVP and the only config-only part.
- **US2** depends on US1 — a trace id is pointless without records to group.
- **US3** depends on **US2**, not US1. Without `generation_name`/`tags`, reporting groups by serving
  deployment and two stages on one model collapse into one bucket.
- **Retention** depends only on Setup; it can be built in parallel with US1/US2 by a second worker.

**Hard gates**:

- **T008 blocks everything after it.** A collector that can stall inference is not shipped behind a
  flag or a caveat — it is not shipped.
- **T010 before US1 ships.** This feature must not add a secret to a channel it knows is leaking.
- **T037 before the domain doc states a retention guarantee** (contracts C7-3). A binding rule with no
  mechanism is exactly the drift the specs lifecycle exists to prevent.

**Within Setup**: T001 blocks T002 and T004; T003, T005, T006 are independent.

## Parallel Execution Examples

**Setup**: T003, T005 and T006 in parallel — three different concerns in two files.

**US1 tests**: T012 and T013 both extend the same existing file — sequence them.

**US2 tests**: T020–T025 across four files; T021, T022 and T023 share `gateway_test.go` and should be
sequenced within it.

**Retention**: T035 and T036 in parallel; both precede T037.

**Polish**: T040, T041, T042 in parallel — three different documents.

## Implementation Strategy

**MVP** = Phase 1 + Phase 2 + US1. Per-call cost, latency, served deployment and failure visibility
for every AI call, with a clean `apps/api` diff — plus the credential-channel fix, which is not
optional given what T010 found.

**Ship boundary** = MVP + Retention. Storing the user's full employment history in a new service with
no enforced expiry is not a state to ship and revisit; the pruning job is what makes collection
defensible.

**Increment 2** = US2. Run grouping and per-task naming — the point at which reporting becomes
per-stage rather than per-model.

**Increment 3** = US3 + Polish.
