---

description: "Task list for Explicit Database Connection Capacity"
---

# Tasks: Explicit Database Connection Capacity

**Input**: Design documents from `/specs/026-db-pool-capacity/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/{config,readiness}.md, quickstart.md

**Tests**: Yes — required. Constitution Principle IV applies: budget arithmetic and config validation get Go unit tests; pool behaviour under saturation gets an integration test against real Postgres, not a mock.

**Organization**: Grouped by user story. US2 (configuration + validation) is foundational for US1 and US3 and is sequenced first despite equal priority — nothing can be observed or measured until capacity is explicitly set.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable (different files, no dependency on incomplete work)
- **[Story]**: US1–US3 from spec.md

## Path Conventions

- Repo root: `/home/nnc/Projects/job-finder`
- All paths below are relative to `apps/api/` unless stated otherwise
- No migration, no sqlc regeneration, no tygo regeneration, no dashboard change

---

## Phase 0: BLOCKING — the tree does not compile

**`apps/api` does not build at `ede4b90`.** Verified 2026-07-30:

```
$ cd apps/api && go build ./... 2>&1 | grep '^#' | sort -u
# github.com/job-finder/api/internal/generation/domain
# github.com/job-finder/api/internal/ghostjob/application
# github.com/job-finder/api/internal/keyword/infrastructure/rephraseadapter
# github.com/job-finder/api/internal/outreach/domain
# github.com/job-finder/api/internal/queue
# github.com/job-finder/api/internal/recruiter/application
# github.com/job-finder/api/internal/salary/application
```

Seven packages, one uniform cause: the DDD restructure moved types into `domain/`
sub-packages without updating the `application/` code that references them unqualified
(`undefined: Repository`, `undefined: SalaryBand`, `undefined: ResolvedContact`, …), plus
`llm.ProviderClass` missing for `internal/queue`.

Two branches are already in flight against this: `fix/ci-build-failures`
(worktree `fix-ci-build-failures-from-ddd-restructure`, still 1 package broken) and
`fix/gh-action-compile-errors`.

**Consequence for this feature**: every task below assumes a compiling tree. `internal/queue`
is one of the broken packages and T006 depends on it directly. No baseline can be measured
(T002), no test can run, and SC-001/SC-002 cannot be evaluated until this is resolved.

- [ ] T000 Land a green build on `master` before starting Phase 1 — either by finishing
  `fix/ci-build-failures` or by fixing the seven packages directly. Confirm with
  `cd apps/api && go build ./... && go test ./...`. Do not start this feature on a red tree.

---

## Phase 1: Setup

- [ ] T001 Create the feature branch: `git checkout -b 026-db-pool-capacity`. Do not work on `master` — `.githooks/pre-commit` and `scripts/hooks/guard-master.sh` will reject it.
- [ ] T002 Confirm the baseline claim before changing anything: start the backend and record the effective pool size, so the "before" number in SC-005 is measured rather than assumed. `psql "$DATABASE_URL" -c "SELECT count(*) FROM pg_stat_activity WHERE application_name LIKE '%job-finder%' OR datname='jobfinder';"` under load, or add a one-off `slog` line locally. Record the value in the PR description.

**Checkpoint**: on a branch, baseline recorded.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the capacity arithmetic and configuration surface every story depends on.

**⚠️ No user story work can begin until this phase is complete.**

- [ ] T003 [P] Add the seven configuration fields to `internal/config/config.go` exactly as specified in `contracts/config.md`: `DBMaxConns`, `DBMinConns`, `DBMaxConnLifetime`, `DBMaxConnIdleTime`, `DBAcquireTimeout`, `DBServerMaxConns`, `DBInteractiveReserve`, each with a `mapstructure` tag equal to its env var name and a comment stating its effect (match the density of the surrounding fields).
- [ ] T004 [P] Register defaults in `internal/config/defaults.go` under a `// 026-db-pool-capacity` comment block, mirroring the existing `// 019-ai-job-throughput` block style: `DB_MAX_CONNS: 0`, `DB_MIN_CONNS: 2`, `DB_MAX_CONN_LIFETIME: "1h"`, `DB_MAX_CONN_IDLE_TIME: "30m"`, `DB_ACQUIRE_TIMEOUT: "5s"`, `DB_SERVER_MAX_CONNS: 100`, `DB_INTERACTIVE_RESERVE: 8`.
- [ ] T005 Create `internal/db/capacity.go` with `CapacityBudget` and its `Required()` method per data-model.md §2. `BackgroundSlots` is a named constant `backgroundConnectionSlots = 2` whose comment enumerates what it covers (ingestion scheduler, activity sweeper) and instructs raising it when another long-lived connection-holding goroutine is added.
- [ ] T006 Add `db.BudgetFromPolicies(policies []queue.TaskPolicy, reserve, serverMax int) CapacityBudget`, summing `TaskPolicy.PoolSize()`. **Use `PoolSize()`, not the live provider class** — this is what satisfies FR-013 (see research.md R3). Note: this makes `internal/db` import `internal/queue`; verify `queue` does not import `db` (it does not — `queue` imports only `config` and `asynq`) so no cycle is created.
- [ ] T007 [P] Write `internal/db/capacity_test.go`: `Required()` arithmetic at shipped defaults equals 25; raising `AI_CONCURRENCY_CLOUD` raises the requirement; a policy set whose `LocalConcurrency` exceeds `HostedConcurrency` is budgeted at the local value. Include a test asserting `backgroundConnectionSlots` matches the number of long-lived goroutines launched in `cmd/server/servers.go:runServers` — a comment alone will not survive a future edit.
- [ ] T008 Implement validation in `internal/config` per the table in `contracts/config.md`, producing every listed message verbatim. Fail on under-capacity; warn on over-declared capacity and on unreachable idle retirement.
- [ ] T009 [P] Write `internal/config/config_test.go` cases for each row of the validation table — one test per failure condition asserting the message names the offending key, plus one asserting shipped defaults validate cleanly.

**Checkpoint**: capacity can be computed and validated; nothing yet applies it to a pool.

---

## Phase 3: User Story 2 — Capacity is stated, validated, consistent (Priority: P1) 🎯 MVP

**Goal**: the pool's size is an explicit, validated decision instead of pgx's default.

**Independent Test**: quickstart.md steps 1–5. Deliverable on its own: even with no observability work, the pool stops being 4 on a 4-core host.

- [ ] T010 [US2] Add functional options to `internal/db/db.go`: `type Option func(*pgxpool.Config)` and `WithPoolConfig(PoolConfig) Option`. Change `Open(ctx, databaseURL string, opts ...Option)`. **Variadic, not a required parameter** — `db.Open` has 15 call sites, 13 of them tests (`grep -rn "db.Open(" apps/api`); a required parameter buries a 6-file change under 13 mechanical test edits.
- [ ] T011 [US2] Rewrite `Open` to `pgxpool.ParseConfig(databaseURL)` → apply options → `pgxpool.NewWithConfig(ctx, cfg)` → `Ping`. Preserve the existing error wrapping (`db: connect: %w`, `db: ping: %w`) and the existing `pool.Close()` on ping failure.
- [ ] T012 [US2] In `cmd/server/platform.go`, build the budget from `queue.PoliciesFromConfig(cfg)` — note `buildPlatform` currently receives only `cfg`, and `Platform.Policies` is populated later; either build policies once in `buildPlatform` and reuse them, or move budget construction to where policies already exist. Do not compute policies twice, and do not duplicate the concurrency arithmetic.
- [ ] T013 [US2] Emit the single startup info line specified in `contracts/config.md`, including `derived=true|false` and the full breakdown.
- [ ] T014 [US2] Update `apps/api/.env.example` with the documented block from `contracts/config.md` verbatim, including the explanatory comments.
- [ ] T015 [US2] Update `README.md` and `AGENTS.md` where connection or concurrency configuration is described, so the documented behaviour matches the implemented behaviour (Constitution: docs must not contradict enforcement).
- [ ] T016 [US2] Verification: run quickstart.md steps 1–5 and confirm each expected message appears. A validation rule never seen rejecting anything has not been tested.

**Checkpoint**: US2 complete and independently landable. Pool is explicit and validated.

---

## Phase 4: User Story 1 — Dashboard stays responsive under load (Priority: P1)

**Goal**: interactive requests are not starved by background work.

**Independent Test**: quickstart.md step 6 — measured idle vs loaded latency.

- [ ] T017 [US1] Add an acquisition-deadline middleware in `internal/httpapi` that derives a `context.WithTimeout(r.Context(), cfg.DBAcquireTimeout)` for the request. Register it in `NewRouter` after `middleware.Recoverer`. Per research.md R5 this bounds the interactive path only; background workers keep their `TaskPolicy.MaxDuration` deadlines.
- [ ] T018 [US1] Map a deadline-exceeded failure on the acquire path to a distinguishable error response identifying connection-capacity exhaustion, via the existing `writeError` helper and `internal/apperr` conventions — not a bare 500. Check how `apperr` classifies errors today and follow it rather than inventing a parallel scheme.
- [ ] T019 [P] [US1] Write `internal/httpapi` unit tests for the middleware: a handler that outlives the deadline produces the capacity error; a fast handler is unaffected; the middleware does not shorten a request context that is already shorter.
- [ ] T020 [US1] Write `internal/db/pool_integration_test.go` (`//go:build integration`) following the `internal/dbtest` helper convention: open a pool with `MaxConns=2`, hold both connections, assert a third acquire fails within the configured timeout rather than blocking indefinitely, and assert it succeeds once one is released.
- [ ] T021 [US1] Verification: run quickstart.md step 6 and record idle mean, loaded mean, and the ratio in the PR description. **SC-001 requires ratio ≤ 1.5.** If it is not met, stop — a number that misses the criterion is the finding, not a rounding matter.
- [ ] T021a [US1] **If T021 misses SC-001**: the reserve default is provisional (spec Assumptions, contracts/config.md), so the first remedy is to raise `DB_INTERACTIVE_RESERVE` from 8 and re-measure, not to redesign. Record every value tried and its ratio. Only if raising the reserve fails to converge does the design assumption itself need revisiting — in which case stop and report rather than continuing to tune. Skip this task if T021 passed.

**Checkpoint**: US1 complete. SC-001 and SC-002 measured, not asserted.

---

## Phase 5: User Story 3 — Saturation is visible (Priority: P2)

**Goal**: connection starvation is diagnosable from the readiness report alone.

**Independent Test**: quickstart.md steps 7–8.

- [ ] T022 [P] [US3] Add `PoolStats` and `(*DB).PoolStats()` to `internal/db`, mapping from `pgxpool.Pool.Stat()` per data-model.md §3, including the derived `Saturated` field.
- [ ] T023 [US3] Add the `PoolStatter` interface and nil-able `Pool` field to `internal/httpapi/health.go` per `contracts/readiness.md`. **Do not widen the existing `Pinger` interface** — Redis and MinIO have no pool and would need meaningless implementations. When `Pool` is nil, omit the `pool` key entirely rather than emitting zeros.
- [ ] T024 [US3] Confirm `ok` is unaffected by saturation. A saturated pool is still serving; flipping readiness to false under load would remove the process from rotation for a load condition and make it worse (contracts/readiness.md).
- [ ] T025 [US3] Wire the pool into `HealthHandler` in `cmd/server/compose.go` alongside the existing `Postgres`/`Redis`/`Minio` wiring. **If feature 027 has already landed**, `HealthHandler` lives in `internal/health`, not `internal/httpapi` — target that package instead. 027 moves it unconditionally (027 T039), so this is a location change only, not a design question.
- [ ] T026 [P] [US3] Extend `internal/httpapi/health_test.go`: the `pool` block appears with correct values when `Pool` is set; the key is absent when nil; `ok` stays true when `saturated` is true.
- [ ] T027 [US3] Create `internal/db/saturation.go`: a ticker-driven sampler per data-model.md §4 that warns **only** after N consecutive saturated samples (default 4 at 30s ⇒ ~2 minutes), with the specified fields. Launch it from `runServers` alongside the existing `p.Sweeper.Run(ctx)`, and stop it on context cancellation.
- [ ] T028 [P] [US3] Unit-test the saturation sampler with an injected clock and a fake stats source: 3 consecutive saturated samples produce no log; 4 produce exactly one; an intervening unsaturated sample resets the counter; a second sustained episode produces a second log.
- [ ] T029 [US3] Verification: run quickstart.md steps 7 and 8. Confirm the warning fires once per sustained episode, not once per sample, and that a starved interactive request fails within `DB_ACQUIRE_TIMEOUT` instead of hanging.

**Checkpoint**: all three stories complete.

---

## Phase 6: Polish & Cross-Cutting

- [ ] T030 Verification: quickstart.md step 9 — restart Postgres beneath the running process, confirm recovery within 60s with no API restart (SC-006, FR-011).
- [ ] T031 Run the full merge gate: `make test-lint`, plus `go test -tags integration ./internal/db/...`. Both must pass (AGENTS.md).
- [ ] T032 [P] Record in the PR description: baseline pool size from T002, derived capacity, and the T021 latency ratio. These three numbers are the evidence for SC-001, SC-002 and SC-005.
- [ ] T033 Open the PR against `master` and confirm CI is green before merge.

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1)** → no dependencies
- **Foundational (Phase 2)** → depends on Setup; **blocks all stories**
- **US2 (Phase 3)** → depends on Foundational. Sequenced before US1 and US3 despite equal priority: nothing can be measured or observed until capacity is explicitly set
- **US1 (Phase 4)** → depends on US2 (needs a real configured capacity to measure against)
- **US3 (Phase 5)** → depends on US2; independent of US1 and may proceed in parallel with it
- **Polish (Phase 6)** → depends on all stories

### Critical path

T003/T004 → T005 → T006 → T008 → T010 → T011 → T012 → T017 → T021

### Parallel opportunities

- T003 ∥ T004 (different files)
- T007 ∥ T009 (different packages)
- T019 ∥ T020 (different packages)
- T022 ∥ T026 ∥ T028 once T010–T012 have landed
- US1 (Phase 4) ∥ US3 (Phase 5) after US2 completes

### Ordering constraint worth stating

T006 introduces an `internal/db` → `internal/queue` import. Verify the direction before writing it: `queue` imports `config` and `asynq` only. If a later change makes `queue` import `db`, this becomes a cycle — the arrangement check proposed in feature 027 would catch it, but that feature has not landed.

---

## Implementation Strategy

### MVP (US2 only)

Phases 1–3. Delivers the entire correctness fix: the pool stops being sized by core count. Independently mergeable and independently valuable — worth shipping alone if US1/US3 slip.

### Incremental delivery

1. Setup + Foundational → arithmetic exists and is tested
2. + US2 → capacity explicit and validated → **merge**
3. + US1 → interactive path bounded and measured → **merge**
4. + US3 → saturation visible → **merge**

### Relationship to feature 025

025 (batch ingest persistence) depends on this feature landing first. Batching ingestion against a 4-connection pool moves the contention earlier rather than removing it, and 025's SC-001 (95% reduction in storage time) is not measurable while the pool is the binding constraint. **Land 026 before starting 025.**

---

## Notes

- No migration, no sqlc regeneration, no tygo regeneration, no dashboard change. If any of those appear in the diff, something has gone wrong.
- The `.claude/settings.json` PostToolUse hooks run `gofmt` and `go vet` on Go edits automatically; do not hand-run them.
- Commit per task or logical group, conventional format (`feat:`, `fix:`, `chore:`, `docs:`).
- T021 and T032 are the tasks that decide whether this feature worked. Do not mark the feature done on the basis of code that compiles.
