---

description: "Task list for Throttle-Only Rate Control"
---

# Tasks: Throttle-Only Rate Control

**Input**: Design documents from `/specs/017-throttle-only-rate-control/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Test tasks ARE included. FR-020 requires daily-limit tests be replaced with pacing
tests, Constitution Principle IV mandates per-language suites plus `make test-lint` for
cross-app changes, and research Finding 4 established that **no test currently covers the
budget logic being removed** — so removal is unprotected until new coverage lands.

**Organization**: Grouped by user story. US1 alone is a shippable MVP.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on incomplete work)
- **[Story]**: US1 / US2 / US3, mapping to spec.md user stories
- Exact file paths included in every task

## Path Conventions

Web application per plan.md: Go API at `apps/api/`, React dashboard at `apps/dashboard/`,
shared TS types at `packages/shared/`.

**Generated code is never hand-edited**: `apps/api/internal/db/sqlcgen/` regenerates via
`make sqlc-generate`; `packages/shared/src/generated.ts` regenerates via `make tygo-generate`.

---

## Phase 1: Setup

**Purpose**: Working environment and a measurable "before" picture.

- [X] T001 Bring up the stack and confirm a clean baseline: `pnpm install`, `pnpm --filter @job-finder/shared build`, `make up`, then `make test-go` and `make test-react` both green before any edits
- [X] T002 [P] Confirm migration numbering: verify `apps/api/internal/db/migrations/` still tops out at `00028` and that `00029` is free, per plan.md Constitution Check
- [X] T003 [P] Capture the SC-006 baseline: record current per-host block and cooling-off counts by querying `consecutive_blocks`, `last_block_at`, and `cooling_off_until` from `host_retrieval_state`, saving the snapshot for the post-change comparison in T047

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the test harness that makes the budget removal verifiable. Research Finding
4 found zero existing coverage of the budget path, so without this phase nothing fails if the
removal is wrong.

**⚠️ CRITICAL**: T004 blocks the US1 verification tasks (T017, T018).

- [X] T004 Add a stub-host retrieval test harness in `apps/api/internal/retrieval/testhelpers_test.go` that spins an `httptest` server, points a `ServiceImpl` at it with a real Postgres-backed `StateStore` (per Constitution IV, no mocks for cross-service behaviour), and can issue an arbitrary number of sequential fetches while collecting every returned `PageOutcome`
- [X] T005 [P] Extend the harness in `apps/api/internal/retrieval/testhelpers_test.go` with a `robots.txt` route whose `Crawl-delay` value is settable per test, for the FR-009 cases in T028–T030
- [X] T006 [P] Add an outcome-assertion helper in `apps/api/internal/retrieval/testhelpers_test.go` that fails if any collected outcome carries `PageDeferred` or if any reason string contains `budget`, `quota`, `allowance`, or `limit` — the reusable form of FR-002 and FR-017

**Checkpoint**: Removal can now be proven, not just asserted.

---

## Phase 3: User Story 1 — Collection is never cut short by an artificial daily cap (Priority: P1) 🎯 MVP

**Goal**: Delete the per-host daily budget end to end. A host can serve unlimited requests in a
day; nothing is refused for volume.

**Independent Test**: Drive one host past 200 requests in a single session and confirm every
fetch is attempted, no outcome is `deferred`, no reason mentions a budget, and the run verdict
is `success`.

**Scope note**: DTO, shared-types, and dashboard *removals* live in this phase rather than US2.
Dropping the DB columns forces `HostStatus` to stop populating budget fields, which breaks the
dashboard's `status.budgetUsed` reference and the TypeScript build. US1 is not shippable unless
the tree compiles, so the removals travel with it. US2 adds the pacing replacement.

### Tests for User Story 1

- [X] T007 [P] [US1] Write a failing integration test in `apps/api/internal/retrieval/service_integration_test.go` that issues 250 sequential fetches to the stub host and asserts zero `PageDeferred` outcomes — must fail against the current cap of 200 before implementation begins
- [X] T008 [P] [US1] Write a failing test in `apps/api/internal/retrieval/service_integration_test.go` asserting no outcome reason string ever contains `budget` (FR-017), using the T006 helper

### Database

- [X] T009 [US1] Create `apps/api/internal/db/migrations/00029_drop_host_budget.sql` dropping `budget_period_start`, `budget_used`, and `budget_limit` from `host_retrieval_state`; the `Down` block re-adds all three with their original types and defaults (`budget_used INTEGER NOT NULL DEFAULT 0`, `budget_limit INTEGER NOT NULL DEFAULT 200`) so a rollback yields a schema the previous binary can run
- [X] T010 [US1] Remove the `IncrementHostBudget`, `ResetHostBudget`, and `DeductHostBudgetCheck` queries from `apps/api/internal/db/queries/hostretrievalstate.sql`, and drop the three budget columns from both the column list and the `ON CONFLICT DO UPDATE` clause of `UpsertHostRetrievalState`
- [X] T011 [US1] Run `make sqlc-generate` to regenerate `apps/api/internal/db/sqlcgen/`, then `make sqlc-check` to confirm it is not stale (depends on T010; never hand-edit the output)

### Go API

- [X] T012 [US1] Delete `IncrementBudget`, `CheckBudget`, and `DeductBudget` from `apps/api/internal/retrieval/state.go` (lines ~108-200, including `CheckBudget`'s period-rollover branching) and remove `BudgetPeriodStart`, `BudgetUsed`, `BudgetLimit` from the `UpsertHostRetrievalStateParams` literal in `Upsert`
- [X] T013 [US1] Delete the budget gate at `apps/api/internal/retrieval/service_impl.go:87-118` — the `CheckBudget` call, the "daily budget exhausted" `PageDeferred`, the `DeductBudget` call, and the "deduct race" `PageDeferred`. Leave the cooling-off check immediately above (lines 72-85) untouched (FR-011)
- [X] T014 [US1] Remove `BudgetUsed`, `BudgetLimit`, and `BudgetResetsAt` from the `HostStatus` struct in `apps/api/internal/retrieval/service.go` and from its construction in `HostStatus` at `apps/api/internal/retrieval/service_impl.go:274-283`, including the `BudgetPeriodStart`-derived reset calculation
- [X] T015 [P] [US1] Remove the `PerHostDailyBudgetDefault` field and its doc comment from `apps/api/internal/config/config.go:114-116`
- [X] T016 [P] [US1] Remove the `"PER_HOST_DAILY_BUDGET_DEFAULT": 200` entry from `apps/api/internal/config/defaults.go:24`

### Contract chain (Constitution III)

- [X] T017 [US1] Remove `BudgetUsed`, `BudgetLimit`, and `BudgetResetsAt` from `HostRetrievalStatusDto` in `apps/api/internal/dto/jobs.go:126-138`
- [X] T018 [US1] Remove the three budget field mappings from `hostsAdapter.HostStatus` in `apps/api/cmd/server/compose_types.go:143-158`
- [X] T019 [US1] Run `make tygo-generate` to regenerate `packages/shared/src/generated.ts`, then `make tygo-check` to confirm freshness (depends on T017)
- [X] T020 [US1] Hand-edit the duplicate `HostRetrievalStatusDto` interface in `packages/shared/src/index.ts:382-391` to drop `budgetUsed`, `budgetLimit`, and `budgetResetsAt`, matching the regenerated copy — this duplicate is the pre-existing Constitution III violation recorded in plan.md Complexity Tracking
- [X] T021 [US1] Rebuild the shared package with `pnpm --filter @job-finder/shared build` so `packages/shared/dist/index.d.ts` no longer exposes the removed fields

### Dashboard

- [X] T022 [US1] Delete the "Budget: {used}/{limit}" and "Budget resets:" spans from `apps/dashboard/src/features/sources/SourcesPage.tsx:171-176`, leaving the `Rung:` span alone in its neutral `text-xs text-muted` row (FR-014)

### Verification

- [X] T023 [US1] Run `make test-go` and confirm T007 and T008 now pass, plus `make test-integration` for the Postgres-backed path
- [X] T024 [US1] Run `make test-react` and `make typecheck` to confirm the dashboard builds with the removed fields

**Checkpoint**: US1 complete. No daily cap exists, the tree builds, and the host panel shows
rung and block state with no budget counter. Shippable on its own.

---

## Phase 4: User Story 2 — Pacing reads as normal operation, not as a problem (Priority: P2)

**Goal**: Make pacing the visible, sole rate control — including the crawl-delay awareness that
research Finding 1 proved was never implemented — and present it as an ordinary fact.

**Independent Test**: Open a healthy host's retrieval status and confirm the pacing line is
present, states the rate in interval form with its provenance, and carries no error, warning, or
danger styling; then confirm a blocked host still renders its warning treatment.

**Why the engine lives here**: US2's deliverable is pacing shown *with its provenance*. The
`source` field has no meaning until something resolves it, so FR-006–FR-010 are US2's
implementation, not separate infrastructure.

**Shipping note**: US1 and US2 should ship together. US1 alone removes the cap while leaving
pacing at a flat default; US2 is what keeps SC-006 (block rate must not rise) safe.

### Tests for User Story 2

- [X] T025 [P] [US2] Write failing resolver-precedence tests in `apps/api/internal/ratelimit/transport_test.go`: operator override wins over crawl delay; a crawl delay slower than default is adopted; a crawl delay faster than default is ignored
- [X] T026 [P] [US2] Write a failing test in `apps/api/internal/ratelimit/transport_test.go` asserting `crawl_delay_seconds == 0` resolves to `DefaultRPS` with source `default` — the highest-risk case per research Finding 3, since `parseCrawlDelay` returns `0` for absent, malformed, zero, and negative values alike and it must never mean "no delay"
- [X] T027 [P] [US2] Write a failing test in `apps/api/internal/ratelimit/transport_test.go` asserting a nil `RateResolver` yields `DefaultRPS` for every host, pinning backward compatibility
- [X] T028 [P] [US2] Write failing tests in `apps/api/internal/ratelimit/transport_test.go` asserting two concurrent callers to one host share a single limiter (FR-008) and that loopback destinations remain unpaced

### Pacing engine

- [X] T029 [US2] Add `RateResolver func(host string) (rps float64, source string, ok bool)` to the `Transport` struct in `apps/api/internal/ratelimit/transport.go`, consulted inside `limiterFor` at limiter-construction time only — never per request — with `ok == false` and nil resolver both falling through to `DefaultRPS`
- [X] T030 [US2] Add a short TTL cache for resolved rates in `apps/api/internal/ratelimit/transport.go` so a crawl delay discovered mid-session takes effect at the next TTL boundary without a restart, and without putting a database query in the hot path
- [X] T031 [US2] Add an exported `RateFor(host string) (rps float64, source string)` method to `apps/api/internal/ratelimit/transport.go` so the API can report a host's effective rate without issuing a request
- [X] T032 [US2] Replace the three-line `apps/api/internal/retrieval/transport.go` with a constructor that wires a `StateStore`-backed resolver into the transport, implementing the precedence from `data-model.md` (override → site-requested when slower than default → default) while keeping `ratelimit` free of any `retrieval`, `db`, or Postgres import
- [X] T033 [US2] Add a `crawl_delay_seconds` reader to `apps/api/internal/retrieval/state.go` for the resolver, returning a tri-state that distinguishes `NULL` (never looked) from `0` (looked, nothing advertised) from `N > 0` (advertised)
- [X] T034 [US2] Trigger `FetchAndSetCrawlDelay` from `apps/api/internal/retrieval/service_impl.go` on first contact with a host whose `crawl_delay_seconds` is `NULL`, dispatched out of band so no user-facing fetch waits on the `robots.txt` round trip — this function currently has zero callers (research Finding 1)

### Contract

- [X] T035 [US2] Add the `pacing` object (`requestsPerSecond`, `intervalSeconds`, `source`) to `HostRetrievalStatusDto` in `apps/api/internal/dto/jobs.go` per `contracts/host-retrieval-status.md`, with `source` as `"default" | "site-requested" | "override"`; retain `crawlDelaySeconds` as the raw diagnostic input
- [X] T036 [US2] Add the corresponding pacing fields to the `HostStatus` struct in `apps/api/internal/retrieval/service.go` and populate them from `RateFor` in `HostStatus` at `apps/api/internal/retrieval/service_impl.go`
- [X] T037 [US2] Map the pacing object in `hostsAdapter.HostStatus` in `apps/api/cmd/server/compose_types.go`
- [X] T038 [US2] Run `make tygo-generate` then `make tygo-check`, and hand-edit the duplicate interface in `packages/shared/src/index.ts` to match; rebuild with `pnpm --filter @job-finder/shared build` (depends on T035)

### Dashboard

- [X] T039 [US2] Add the pacing line to `apps/dashboard/src/features/sources/SourcesPage.tsx` in the same neutral `text-xs text-muted` row as `Rung:`, formatted in interval terms with provenance — e.g. `Pace: ~1 request every 5s (site-requested)` — with no alert icon and no `text-warning` or `text-danger` class (FR-013, FR-016)
- [X] T040 [US2] Verify in `apps/dashboard/src/features/sources/SourcesPage.tsx` that the block line keeps `text-warning` with its `AlertTriangle` icon and cooling-off keeps `text-danger` with its `Clock` icon, preserving the visual distinction between routine and problem (FR-015)
- [X] T041 [P] [US2] Add a component test in `apps/dashboard/src/features/sources/` asserting the pacing line renders without warning or danger classes, that block and cooling-off lines retain theirs, and that no budget text appears

**Checkpoint**: US1 and US2 both work. Pacing is the sole rate control, honours advertised crawl
delays, and reads as an ordinary fact.

---

## Phase 5: User Story 3 — No leftover daily-limit surfaces (Priority: P3)

**Goal**: No stale knob, column, reason string, test filename, or paragraph pointing at a
mechanism that no longer exists.

**Independent Test**: Run the residue sweep from `quickstart.md` Scenario 6 and get zero hits
outside `specs/`.

- [X] T042 [US3] Rename `apps/api/internal/retrieval/budget_test.go` to `crawldelay_test.go` — it contains only `TestParseCrawlDelay` and `TestCrawlDelayRe`, and no budget test has ever existed (research Finding 4). Rename, do not delete
- [X] T043 [P] [US3] Run the residue sweep from `quickstart.md` Scenario 6 across `apps/`, `packages/shared/src/`, and `README.md` for `PER_HOST_DAILY_BUDGET`, `budgetUsed`, `budgetLimit`, `budgetResetsAt`, `BudgetLimit`, `CheckBudget`, `DeductBudget`, and `IncrementBudget`; resolve every hit outside `specs/`
- [X] T044 [P] [US3] Update any documentation in `README.md` describing how the system avoids IP bans so it names pacing and block-triggered cooling-off as the mechanisms, with no reference to a daily cap (FR-019)
- [X] T045 [US3] Confirm `PageDeferred` retains exactly one producer — cooling-off in `apps/api/internal/retrieval/service_impl.go` — and that the `"deferred"` block marker at `apps/api/internal/ingestion/handler.go:453` plus the adapter conversions in `jobgether.go:80`, `glassdoor.go:78`, and `wellfound.go:82` are all left intact; a cooling-off deferral is a genuine block and must keep reporting as one (research Finding 5)

**Checkpoint**: All three stories complete.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T046 Run the full quickstart validation in `specs/017-throttle-only-rate-control/quickstart.md`, Scenarios 1 through 6
- [X] T047 Compare per-host block and cooling-off counts against the T003 baseline to verify SC-006 — the rate must not have increased now that the cap is gone
- [X] T048 [P] Verify SC-005 by measuring observed request rate to a single host across a many-request run, including two concurrent runs against that host, confirming it stays at or below the configured pace
- [X] T049 [P] Confirm FR-018 end to end: a run delayed only by pacing is recorded with verdict `success`, not `partial` or `blocked`
- [X] T050 Run `make sqlc-check` and `make tygo-check` to confirm no generated file is stale
- [X] T051 Run `make test-lint` — required by Constitution IV because this change crosses `apps/api`, `apps/dashboard`, and `packages/shared`. Not done until green

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: depends on Setup; blocks US1 verification (T023)
- **US1 (Phase 3)**: depends on Foundational — the MVP
- **US2 (Phase 4)**: depends on US1, because it adds `pacing` to a DTO whose budget fields US1 removes. Touching the same struct in both phases makes them sequential in practice
- **US3 (Phase 5)**: depends on US1; T045 also touches US2 territory, so run it last
- **Polish (Phase 6)**: depends on all desired stories

### Critical path within US1

```
T009 (migration) → T010 (queries) → T011 (sqlc regen) → T012, T013, T014 (Go)
                                                              ↓
                              T017 (DTO) → T019 (tygo) → T020 (index.ts) → T021 (build) → T022 (UI)
```

T015 and T016 (config) are independent of that chain and can land any time after T013.

### Critical path within US2

```
T029 (resolver) → T030 (TTL cache) → T031 (RateFor) → T032 (wiring) → T033, T034 (crawl delay)
                                                            ↓
                        T035 (DTO) → T036, T037 → T038 (regen) → T039, T040 (UI)
```

### Within each story

- Tests first: T007–T008 must fail before US1 implementation; T025–T028 must fail before US2
- Schema before generated code before Go code before contract before UI
- Regeneration (`make sqlc-generate`, `make tygo-generate`) immediately after its source changes

---

## Parallel Opportunities

### Phase 1

```bash
Task: "T002 Confirm migration numbering"
Task: "T003 Capture SC-006 block-rate baseline"
```

### Phase 2

```bash
Task: "T005 Add robots.txt route to the harness"
Task: "T006 Add outcome-assertion helper"
```

### User Story 1

```bash
# Failing tests together:
Task: "T007 Failing test: 250 fetches, zero PageDeferred"
Task: "T008 Failing test: no reason string contains 'budget'"

# Config removals, independent of the schema chain:
Task: "T015 Remove PerHostDailyBudgetDefault from config.go"
Task: "T016 Remove PER_HOST_DAILY_BUDGET_DEFAULT from defaults.go"
```

### User Story 2

```bash
# All four failing pacing tests target the same file but independent cases:
Task: "T025 Resolver precedence tests"
Task: "T026 Zero-crawl-delay case"
Task: "T027 Nil resolver backward compatibility"
Task: "T028 Concurrency and loopback exemption"
```

### User Story 3

```bash
Task: "T043 Residue sweep"
Task: "T044 Documentation update"
```

---

## Implementation Strategy

### MVP: User Story 1 only

1. Phase 1 Setup
2. Phase 2 Foundational — do not skip; nothing else proves the removal
3. Phase 3 US1
4. **STOP and VALIDATE**: 250 requests to one host, zero refusals, tree builds, panel shows no budget
5. Shippable — this is the user's core ask

### Recommended shipping unit: US1 + US2

US1 alone removes the cap while leaving pacing at a flat 0.7 RPS for every host. US2 adds the
crawl-delay awareness that keeps SC-006 safe. Ship them together unless there is a reason not to.

### Incremental delivery

1. Setup + Foundational → removal is verifiable
2. US1 → cap gone, tree builds → validate → MVP
3. US2 → pacing carries the load and is visible as normal → validate
4. US3 → residue cleared → validate
5. Polish → SC-006 comparison and `make test-lint`

### Parallel team strategy

Limited. US2 depends on US1 through the shared DTO struct, so the stories do not fan out
cleanly. The realistic split is one developer on the Go pacing engine (T029–T034) while another
handles the contract and dashboard chain (T035–T041), after US1 lands.

---

## Notes

- `[P]` = different files, no dependency on incomplete work
- Never hand-edit `apps/api/internal/db/sqlcgen/` or `packages/shared/src/generated.ts`; regenerate
- `packages/shared/src/index.ts` **is** hand-edited — it is a pre-existing duplicate, tracked in plan.md Complexity Tracking
- Migration `00029` is fixed; `00027` and `00028` are reserved by uncommitted work on another feature
- The riskiest single line in this feature is the `crawl_delay_seconds == 0` case (T026). Read as "no delay" it produces an unbounded rate on most hosts — the exact opposite of the goal
- Commit after each task or logical group; stop at any checkpoint to validate a story independently
