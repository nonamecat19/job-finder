---

description: "Task list for CI Test Gate"
---

# Tasks: CI Test Gate

**Input**: Design documents from `/specs/007-ci-test-gate/`

**Prerequisites**: plan.md, spec.md, research.md, quickstart.md

**Tests**: Not requested as automated test tasks — this feature's "test" is the CI
workflow itself; verification is manual per `quickstart.md` (inject a failure, confirm the
job fails, revert, confirm it passes).

**Organization**: All tasks edit the single file `.github/workflows/api-ci.yml`, so within
that file none are truly `[P]` (same-file edits conflict) — parallelism instead comes from
independently verifying each story once its job block exists.

## Format: `[ID] [P?] [Story] Description`

## Path Conventions

Single web-app repo. Only path touched: `.github/workflows/api-ci.yml`. No application
source changes (per plan.md Structure Decision).

---

## Phase 1: Setup

**Purpose**: Confirm the local tooling this feature wires into CI is currently green,
so CI failures introduced later are attributable to the new jobs, not pre-existing breaks.

- [X] T001 Run `go build ./... && go vet ./... && go test ./...` (apps/api) and confirm it passes on `master` before branching (per quickstart.md step 1) — **RESULT: RED.** `go vet` and `go test` both fail with pre-existing build errors unrelated to this feature (`internal/dto/dto_test.go:262` references a removed `SearchQuery.Site` field; `internal/jobsources/adapters/remotive_test.go:107` references undefined `strPtr`), left over from uncommitted working-tree changes predating this feature. User was asked whether to fix first or wire CI anyway; chose **wire CI anyway, ship a known-red gate** — the new `go-vet`/`go-test` jobs will fail on `master` until those two pre-existing build errors are fixed separately.
- [X] T002 Run `pnpm --filter @job-finder/shared build && pnpm --filter @job-finder/dashboard test && pnpm typecheck` from repo root and confirm it passes on `master` before branching (per quickstart.md step 1) — **RESULT: GREEN.** 163/163 dashboard tests pass, typecheck clean.

**Checkpoint**: Frontend baseline green. Backend baseline red (pre-existing, out of this feature's scope) — proceeding per explicit user decision rather than stopping.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Nothing structurally blocks the user stories (no shared library/schema
introduced), but all three stories edit the same `.github/workflows/api-ci.yml`, so this
phase establishes the baseline copy of the file the stories add jobs onto, keeping
`sqlc-drift`/`tygo-drift` byte-for-byte as they are today (FR-007).

- [X] T003 Confirm `.github/workflows/api-ci.yml` currently contains only `sqlc-drift` and `tygo-drift` jobs unchanged, as the baseline to add new jobs to

**Checkpoint**: Baseline workflow confirmed — safe to start adding jobs.

---

## Phase 3: User Story 1 - Backend regressions caught before merge (Priority: P1) 🎯 MVP

**Goal**: CI runs `go test` and `go vet` on every push/PR.

**Independent Test**: Push a branch with a failing Go test (or a `go vet`-flagged issue)
and confirm the new CI job(s) fail on that branch and pass once fixed (spec.md Acceptance
Scenarios 1-3 for US1).

### Implementation for User Story 1

- [X] T004 [US1] Add `go-vet` job to `.github/workflows/api-ci.yml`: checkout, `actions/setup-go@v5` with `go-version-file: apps/api/go.mod`, run `go vet ./...` from `apps/api`
- [X] T005 [US1] **REVISED during implementation**: no `services:` block added. Inspection of the codebase (`internal/db/integration_test.go`, `internal/applications/integration_test.go`, `internal/httpapi/activity_test.go`, `internal/dbtest/lock.go`, and other `*_integration_test.go`/`live_test.go` files) showed every DB/Redis-touching test carries `//go:build integration`, which plain `go test ./...` never compiles. No Postgres/Redis needed for this job (see research.md revision).
- [X] T006 [US1] Add `go-test` job to `.github/workflows/api-ci.yml`: checkout, `actions/setup-go@v5` with `go-version-file: apps/api/go.mod`, run `go test ./...` from `apps/api` (no DB env vars needed — see T005 revision)
- [X] T007 [US1] Followed `specs/007-ci-test-gate/quickstart.md` section 2 intent locally (ran `go vet ./...` / `go test ./...` directly rather than pushing a throwaway PR): confirmed both currently **fail** on this tree due to pre-existing build errors (see T001) — job wiring is correct and will go green as soon as those two build errors are fixed; `sqlc-drift`/`tygo-drift` job definitions unaffected (diff is pure addition, see T011)

**Checkpoint**: Backend regressions are now blocked by CI independently of frontend work — deployable as-is if only this story ships.

---

## Phase 4: User Story 2 - Frontend regressions caught before merge (Priority: P2)

**Goal**: CI runs the dashboard's Vitest suite and TypeScript typecheck on every push/PR.

**Independent Test**: Push a branch with a failing Vitest test (or a TypeScript type
error) and confirm the new CI job(s) fail; fix and confirm they pass (spec.md Acceptance
Scenarios 1-3 for US2).

### Implementation for User Story 2

- [X] T008 [US2] Add `frontend-test` job to `.github/workflows/api-ci.yml`: checkout, `pnpm/action-setup@v4` (version 11, matching local pnpm), `actions/setup-node@v4` (node-version 20, cache pnpm), `pnpm install --frozen-lockfile`, `pnpm --filter @job-finder/shared build` (must precede dashboard steps per constitution Development Workflow), then run `pnpm --filter @job-finder/dashboard test`
- [X] T009 [US2] Add `frontend-typecheck` job to `.github/workflows/api-ci.yml` with the same setup steps as T008, then run `pnpm typecheck`
- [X] T010 [US2] Ran locally in place of a throwaway PR push (per quickstart.md section 3 intent): `pnpm --filter @job-finder/shared build && pnpm --filter @job-finder/dashboard test && pnpm typecheck` — **RESULT: GREEN**, 163/163 tests pass, typecheck clean; job wiring verified correct against a passing baseline

**Checkpoint**: Frontend and backend regressions are both now blocked by CI, independently of each other.

---

## Phase 5: User Story 3 - Existing drift checks remain intact (Priority: P3)

**Goal**: Confirm `sqlc-drift` and `tygo-drift` still behave exactly as before — this
feature adds gates, it never weakens existing ones.

**Independent Test**: Introduce un-regenerated drift (sqlc or tygo) and confirm the
corresponding existing job still fails as it did pre-feature (spec.md Acceptance Scenario
1 for US3).

### Implementation for User Story 3

- [X] T011 [US3] Diffed `.github/workflows/api-ci.yml`'s `sqlc-drift` and `tygo-drift` job definitions against `git diff HEAD` — confirmed the change is a pure append (new jobs added after line 45); zero lines changed within the existing two jobs
- [X] T012 [US3] Not exercised via a live PR (no CI runner available in this session); confirmed instead by code inspection that `sqlc-check.sh`/`tygo-check.sh` invocations and their trigger conditions (`push: [master]`, `pull_request`) are byte-identical to before — behavior is unchanged by construction, not just by test

**Checkpoint**: All five original guarantees (2 drift checks unchanged) plus 4 new gates (go-test, go-vet, frontend-test, frontend-typecheck) are verified independently.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final confirmation the whole workflow file is coherent and matches SC-004
(job names alone identify which check failed).

- [X] T013 Reviewed final `.github/workflows/api-ci.yml` — job `name:` fields (`sqlc generate is up to date`, `tygo generate is up to date`, `go vet`, `go test`, `frontend test (vitest)`, `frontend typecheck`) are distinct and self-explanatory, satisfying SC-004
- [ ] T014 Open a PR for `007-ci-test-gate` and confirm job results: `sqlc-drift`/`tygo-drift`/`frontend-test`/`frontend-typecheck` green, `go-vet`/`go-test` red until the two pre-existing build errors (T001) are fixed separately — **deferred**, requires an actual GitHub PR/CI run, not completable from this local session

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — run first to establish a trustworthy baseline
- **Foundational (Phase 2)**: Depends on Setup — confirms the file to build on
- **User Story 1 (Phase 3)**: Depends on Foundational — MVP, no dependency on US2/US3
- **User Story 2 (Phase 4)**: Depends on Foundational only — independently testable, but edits the same file as US1 so apply sequentially (T004-T006 before T008-T009) to avoid merge conflicts within one PR
- **User Story 3 (Phase 5)**: Depends on Foundational only (verification-only phase, no new job code) — can run anytime after Phase 2, ordered last here since it verifies non-regression of the other two stories' work
- **Polish (Phase 6)**: Depends on all three user stories being complete

### Parallel Opportunities

- T001 and T002 (Setup) can run in parallel — different toolchains, no shared state
- T004-T006 (US1) and T008-T009 (US2) touch the same file — do not parallelize edits, but the underlying verification steps (T007 vs T010) can run in parallel once both job blocks exist, since they exercise unrelated jobs
- T011-T012 (US3) can run in parallel with US1/US2 verification since it only reads/diffs existing jobs

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1 (go-test + go-vet)
4. **STOP and VALIDATE**: quickstart.md section 2
5. Ship — backend regressions are now gated even before frontend gating exists

### Incremental Delivery

1. Setup + Foundational → baseline confirmed
2. US1 (go-test, go-vet) → validate → this alone closes the highest-value gap named in the report
3. US2 (frontend-test, frontend-typecheck) → validate → closes the second gap
4. US3 (verify drift jobs untouched) → validate → confirms no regression
5. Polish → confirm job-name clarity and a clean end-to-end PR run

---

## Notes

- No `[P]` markers are used across T004-T012 despite being organized by story, because all
  edit the same `.github/workflows/api-ci.yml` file — marking them parallel would be
  misleading (same-file edits conflict). Parallelism instead applies to the *validation*
  steps (T007, T010, T012) which exercise independent CI jobs once the file changes land.
- `golangci-lint` and ESLint gating are explicitly out of scope (FR-008) — no task adds
  them; do not add ad hoc lint steps while implementing this list.
- Commit after each user-story phase (T004-T007, then T008-T010, then T011-T012) rather
  than one bundled commit, per this repo's small-per-feature-commit convention.
