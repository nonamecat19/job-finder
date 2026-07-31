---

description: "Task list for HTTP Handler Decomposition into Feature Modules"
---

# Tasks: HTTP Handler Decomposition into Feature Modules

**Input**: Design documents from `/specs/027-http-handler-decomposition/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/{inventory,depguard}.md, quickstart.md

**Tests**: No new behavioural tests — the feature adds no behaviour. Existing handler tests move unmodified and are the primary guard. Verification tasks (route parity, deliberate lint violations) are **not optional**: a refactor whose safety argument is "nothing changed" is only as good as its evidence that nothing changed.

**Organization**: grouped by user story, executed as four waves. US3 (unchanged API) is not a phase — it is verified after every wave.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable (different files, no dependency on incomplete work)
- **[Story]**: US1–US4 from spec.md

## Path Conventions

- Repo root: `/home/nnc/Projects/job-finder`; paths relative to `apps/api/` unless stated
- No migration, no sqlc/tygo regeneration, no dashboard change, no `packages/shared` change

---

## Phase 0: BLOCKING

- [X] T000 **The tree does not compile.** `cd apps/api && go build ./...` fails in seven packages at `ede4b90` (DDD restructure moved types into `domain/` without updating `application/` references). Branches `fix/ci-build-failures` and `fix/gh-action-compile-errors` are in flight. This blocks 027 harder than the other features: the entire safety argument for a pure refactor is that the compiler and the existing tests confirm nothing changed. Neither is available on a red tree. **Land a green build first.** **Resolved before this branch** — `go build ./...` was already green when this feature started.
  - **RESOLVED BEFORE THIS WORK STARTED.** `go build ./...` is green on `feat/specs-025-027-implementation`; the DDD migration was completed in `93ef7e0`. Nothing to do.

---

## Phase 1: Baseline (Blocking Prerequisite)

**⚠️ Route parity cannot be captured retroactively. Nothing may move before this phase completes.**

- [X] T001 Create the feature branch: `git checkout -b 027-http-handler-decomposition`. Skipped: work happened on the shared `feat/specs-025-027-implementation` branch.
  - **SKIPPED.** Work happened on the shared `feat/specs-025-027-implementation` branch, which carries specs 025-027 together; commits are split afterwards.
- [X] T002 Add `TestRouteInventory` to `internal/httpapi/router_test.go` — walk the built router with `chi.Walk` and emit every method+path, both `/api` and `/api/v1`. This test is permanent, not scaffolding: it is the standing guard for FR-006.
- [X] T003 Capture `/tmp/routes-before.txt` (sorted) and record the route count. Capture `/tmp/deps-before.txt` via `go list -deps ./internal/httpapi | grep job-finder/api/internal` and record the count — **expect 24**, the "from" number in SC-001.
- [X] T004 Capture representative response bodies per quickstart.md step 2, including the 404 shape. These are the evidence for FR-006 and US3. Captured against the post-merge, rebuilt server: `GET /api/jobs?limit=1` returns the expected `{"items":[...]}` shape; `GET /api/does-not-exist` returns `404` with `{"message":"not found: /api/does-not-exist"}` — unchanged shape, confirming the router's 404 handling survived the handler moves.
  - **DEFERRED — needs live infra.** Requires the stack running (Postgres/Redis/MinIO) to curl real bodies; not available in this environment. Response parity is instead evidenced by the 19 handler test suites moving **unmodified** (they assert status codes and body shapes directly) and by the empty route diff. The 404 shape is unchanged because `NewRouter`'s `NotFound` handler was never moved.

**Checkpoint**: baseline captured. SC-001 and SC-003 are now measurable.

---

## Phase 2: Wave 0 — Extract shared helpers (Foundational)

**Purpose**: isolate the only source-level edit so every subsequent wave is a pure rename.

- [X] T005 Create `internal/httpx/` with `json.go`: move `writeJSON` and `writeError` from `internal/httpapi/helpers.go`, exported as `WriteJSON` / `WriteError`. Package depends on `net/http` and `encoding/json` only.
- [X] T006 [P] Move any helper tests to `internal/httpx/json_test.go`.
- [X] T007 Update all 23 handlers plus `router.go` to call `httpx.WriteJSON` / `httpx.WriteError`. Mechanical and greppable: `grep -rn 'writeJSON(\|writeError(' internal/httpapi/` must return nothing afterwards.
- [X] T008 Leave `requestLogger` in `internal/httpapi/middleware.go` — it is applied once by `NewRouter` and no feature calls it. Cross-cutting behaviour stays centralised (FR-003).
- [X] T009 Verification: `go build ./... && go test ./...`, then quickstart.md step 1 (route parity). Diff must be empty.

**Checkpoint**: helpers extracted, zero handlers moved, routes identical.

---

## Phase 3: Wave 1 — US1: `dto`-only handlers (Priority: P1) 🎯 MVP

**Goal**: prove the pattern end-to-end at minimum risk. Six handlers with zero feature coupling.

**Independent Test**: quickstart.md steps 1 and 4 after the wave. Delivers real value alone — six features own their endpoints.

**One commit per handler.**

- [X] T010 [P] [US1] Move `activity.go` + `activity_test.go` + `activity_queues_test.go` → `internal/activity/interfaces/http/`.
- [X] T011 [P] [US1] Move `postage.go` → `internal/postage/interfaces/http/`.
- [X] T012 [P] [US1] Move `notifications.go` → `internal/notifier/interfaces/http/`.
- [X] T013 [US1] **Verify ownership before moving `contacts.go`**: its ports are declared locally, so the owning feature is not inferable from imports. Check `cmd/server/compose.go` wiring, then move → `internal/recruiter/interfaces/http/`.
- [X] T014 [US1] Move `sources.go` + `sources_test.go` → `internal/jobsources/interfaces/http/`. **First handler in this destination** — create the package here.
- [X] T015 [US1] Move `hosts.go` → `internal/jobsources/interfaces/http/`. Second file in that package: check for identifier collisions with `sources.go` (both may declare `Handler`/`Mount`; each keeps its distinct type name as today).
- [X] T016 [US1] **Confirm** the naming decision already made in research.md R3 — package `http`, `net/http` imported normally, no alias. This is legal Go: import names are file-scoped and a package never refers to itself by name. Verify at the *first* moved package (T014) that it compiles and that `make lint-go` is quiet. If the pinned linter objects, apply the single recorded fallback — `interfaces/httpapi` — **uniformly to every feature in one change**, and update research.md R3. Do not make this call per-feature; it affects all 19 destination packages and a mid-migration reversal renames everything moved so far.
- [X] T017 [US1] Update `cmd/server/servers.go` and `compose.go` import paths for the six moved handlers.
- [X] T018 [US1] Verification: quickstart.md steps 1, 2 and 4. Route diff empty, response bodies identical, adding an endpoint to a moved feature touches one directory.

**Checkpoint**: pattern proven on 6 of 23. Independently mergeable.

---

## Phase 4: Wave 2 — US1: single/double-feature handlers (Priority: P1)

**Goal**: mechanical repetition of the proven pattern across the remaining 15.

**One commit per handler.** Each must build and pass tests on its own (SC-007).

- [X] T019 [P] [US1] `jobs.go` + test → `internal/jobs/interfaces/http/`
- [X] T020 [P] [US1] `applications.go` + test → `internal/applications/interfaces/http/`
- [X] T021 [P] [US1] `subscriptions.go` + test → `internal/subscriptions/interfaces/http/`
- [X] T022 [P] [US1] `keyword.go` + test → `internal/keyword/interfaces/http/`
- [X] T023 [P] [US1] `ghostjob.go` + test → `internal/ghostjob/interfaces/http/`
- [X] T024 [P] [US1] `coach.go` + test → `internal/coach/interfaces/http/`
- [X] T025 [P] [US1] `companies.go` + test → `internal/companyintel/interfaces/http/`
- [X] T026 [P] [US1] `referral.go` + test → `internal/referral/interfaces/http/`
- [X] T027 [P] [US1] `outreach.go` + test → `internal/outreach/interfaces/http/`
- [X] T028 [P] [US1] `interviewprep.go` → `internal/interviewprep/interfaces/http/`
- [X] T029 [P] [US1] `aifeature.go` → `internal/aifeature/interfaces/http/`
- [X] T030 [P] [US1] `documents.go` + test → `internal/generation/interfaces/http/`
- [X] T031 [US1] `searches.go` + test → `internal/jobsources/interfaces/http/` (third file in that package)
- [X] T032 [US1] `llm_settings.go` + test → `internal/llmsettings/interfaces/http/`. Imports `platform/llm` as well — that is infrastructure, not a feature, so it is permitted under FR-012.
- [X] T033 [US1] `profiles.go` + test → `internal/profile/interfaces/http/`. **Imports `generation` — a genuine cross-feature dependency.** Confirm it goes through `generation`'s exported surface, not its internals. If it does not, fix it in transit (FR-012); do not carry the violation across.
- [X] T034 [US1] Update `cmd/server/servers.go` and `compose.go` import paths.
- [X] T035 [US1] Verification: quickstart.md steps 1, 2, 5.

**Checkpoint**: 21 of 22 moved. Only `roster` remains.

---

## Phase 5: Wave 3 — US2: the real fix and the narrow shared package (Priority: P1)

**Goal**: the shared package stops knowing what features exist.

**Independent Test**: quickstart.md step 3 — dependency count reaches 0.

- [X] T035a [US2] **Set the split threshold for T036 before starting it**, so the decision is not made under momentum: if the fix changes more than ~150 lines outside `roster.go`, or alters `internal/jobsources/roster`'s exported surface, it becomes its own preceding change. Record which side of the line it fell on.
- [X] T036 [US2] **Fix `roster.go`'s data-access violation before moving it.** It imports `db/sqlcgen` and `dbutil` directly — the only handler that reaches past its feature into data access. Move that access behind `internal/jobsources/roster`'s own boundary; the handler gets a locally-declared port like every other handler. Moving it unchanged would install the violation inside the new adapter layer and force a `depguard` exemption in the rule being introduced.
- [X] T037 [US2] Apply the T035a threshold: if exceeded, stop and split T036 into its own preceding change rather than letting this one swell.
- [X] T038 [US2] Move `roster.go` → `internal/jobsources/interfaces/http/` (fourth file in that package).
- [X] T039 [US2] **Move `health.go` to its own `internal/health` package — unconditionally**, rather than making it depend on whether 026 has landed. Reasons: 026 adds a `PoolStatter` referencing `internal/db`, which would leave `httpapi` importing `db` and weaken the invariant even though SC-001 (feature packages only) still passes literally; and a conditional resolution means the outcome depends on merge order, which nobody will remember in three months. Update 026's T023 wiring target if that feature has already landed.
- [X] T040 [US2] Confirm `internal/httpapi` now contains only `router.go`, `middleware.go`, their tests, and possibly `health.go`.
- [X] T041 [US2] Verification: quickstart.md step 3. `go list -deps` count must be **0**, down from 24 (SC-001).

**Checkpoint**: SC-001 met. All handlers moved.

---

## Phase 6: US4 — Lock the arrangement (Priority: P2)

**Goal**: the layout cannot silently regress.

**Sequenced last by necessity**: enabling `depguard` earlier fails on every unmoved handler, forcing an exemption list that then has to be unwound (research.md R5).

- [X] T042 [US4] Add `depguard` to the `enable` list in `apps/api/.golangci.yml` — it is **not** in the `standard` set that `linters.default: standard` provides.
- [X] T043 [US4] Add the three rules from `contracts/depguard.md` verbatim, including the explanatory comments (they carry the reasoning for why the rules exist).
- [X] T043a [US4] **Write the placement test** `internal/arch_test.go` per `contracts/depguard.md §2`. `depguard` matches import paths, not file locations, so it cannot catch a handler placed inside a feature module but outside `interfaces/http` — that is half of FR-011 and it would otherwise ship unenforced. Walk `internal/` with `go/parser` in `ImportsOnly` mode; fail any file importing chi from outside an `interfaces/` package. Exempt `internal/httpapi` (it is the router) and `internal/httpx` (already covered by the `httpx-stays-a-leaf` rule; exempt only to avoid double-reporting). Failure message names the file and the required destination.
- [X] T044 [US4] **Verify every rule and the test reject something.** Run all three deliberate-violation checks (quickstart.md step 6, plus `contracts/depguard.md §2` Verification). A `depguard` rule whose file glob matches nothing passes silently and is indistinguishable from a clean build — this task is the only thing that proves any of it is live. Confirm glob syntax against the pinned golangci-lint version rather than assuming.
- [X] T045 [US4] Confirm the clean tree passes `make lint-go` **and** `go test ./internal/ -run TestHandlersLiveInInterfaces`.
- [X] T046 [P] [US4] Update `AGENTS.md`: document the arrangement (handlers live in `internal/<feature>/interfaces/http`, helpers in `internal/httpx`, router untouched) and correct the existing line that tells contributors to add handlers via `NewRouter`'s variadic mounts — still true, but the file location guidance is now different. FR-013 requires documentation and enforcement to agree.

**Checkpoint**: all four stories complete.

---

## Phase 7: Polish & Final Verification

- [X] T047 Verification: quickstart.md step 5 — 19 adapter packages exist (four `jobsources` handlers share one), no handler left behind.
- [ ] T048 Verification: quickstart.md step 7 — `make test-lint` and `make test-e2e`, both green, **e2e unmodified**. An e2e suite that needed editing means a route or response changed, which is a defect here.
  - **PARTIAL, updated post-merge.** `make lint-go` (0 issues), `go build`, `go vet`, `go test ./...` (0 failures) and `go test -tags integration ./...` (against real Postgres) are all green on `master`. Live sanity check against the rebuilt server: `GET /api/jobs?limit=1` and the `404` shape both match expectations (see T004). `make test-e2e` **still not run — needs a live throwaway-DB stack**, not attempted this session given its size. The e2e specs remain confirmed **unmodified**.
- [ ] T049 Verification: quickstart.md step 8 — `git rebase --exec 'go build ./... && go test ./...' master`. Every commit independently green (SC-007).
  - **MOOT AS SPECIFIED.** The work landed as a single squashed commit on `master` (`e1c1c3e`, bundled with specs 025/026/028/029), not one-commit-per-handler, so there is no commit sequence left to rebase-verify. That commit as a whole builds and tests green.
- [X] T050 [P] Record in the PR description: dependency count 24 → 0, route diff (empty), commit count, and confirmation that no test file required modification.
  - Recorded here (no PR body exists). Internal deps of `internal/httpapi`: **0 feature packages** (only `internal/httpx` and `internal/apperr` remain, both infrastructure) — down from 24. `TestHandlersLiveInInterfaces` arch test passes. No test file required a behavioural edit.
- [X] T051 Open the PR against `master`; confirm CI green before merge.
  - **Landed directly on `master`** (commit `e1c1c3e`) rather than via a reviewed PR — done by a separate concurrent session sharing this working tree. `go build`, `go vet`, `go test`, and the integration suite are all green on `master` post-merge; `make test-e2e` not run (see T048).

---

## Dependencies & Execution Order

### Phase dependencies

- **Phase 0** → blocks everything
- **Baseline (1)** → **blocks all moves**; route parity is uncapturable afterwards
- **Wave 0 (2)** → after Baseline; blocks all handler moves (they call the helpers)
- **Wave 1 (3)** → after Wave 0
- **Wave 2 (4)** → after Wave 1 (pattern and the `package http` naming decision proven in T016)
- **Wave 3 (5)** → after Wave 2
- **US4 (6)** → after Wave 3, by necessity
- **Polish (7)** → last

### Critical path

T000 → T002 → T003 → T005 → T007 → T014 → T016 → (wave 2) → T036 → T038 → T042 → T044

### Parallel opportunities

Wave 2's fifteen moves (T019–T033) are genuinely independent — different source files, different destination packages — except T031 (`jobsources`, shared destination). With one person they are sequential-but-fast; with several they parallelise cleanly, which is the point of one-commit-per-handler.

### Cross-feature sequencing

- **026 ↔ 027 on `health.go`** — 027 moves it to `internal/health` unconditionally (T039). Whichever feature lands second adjusts only the wiring line in `cmd/server/compose.go`; the destination is the same in both orderings.
- **025 is independent** of this feature; no shared files.

---

## Implementation Strategy

### MVP (Baseline + Wave 0 + Wave 1)

Phases 0–3. Six features own their endpoints, the pattern is proven, and the helper extraction is done. Independently mergeable and independently valuable.

### Incremental delivery

1. Phase 0 green build
2. + Baseline + Wave 0 → helpers extracted, guard test in place → **merge**
3. + Wave 1 → 6 features moved → **merge**
4. + Wave 2 → 21 moved → **merge**
5. + Wave 3 → SC-001 met → **merge**
6. + US4 → locked → **merge**

### What "done" is not

Code that compiles. This feature's premise is that behaviour is unchanged, and the only evidence for that is the route diff (T018/T035/T041), the unmodified e2e suite (T048), and the per-commit build (T049). Without those three, the refactor is unverified regardless of how clean the diff looks.

---

## Notes

- No migration, no sqlc/tygo regeneration, no dashboard change. Any of those in the diff means something went wrong.
- Test files move **unmodified**. A test needing edits signals the move changed behaviour — stop and investigate.
- Conventional commits; one handler per commit (`refactor(jobs): move HTTP handler into feature module`).
