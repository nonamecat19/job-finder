---

description: "Task list for Batched, Atomic Ingest Persistence"
---

# Tasks: Batched, Atomic Ingest Persistence

**Input**: Design documents from `/specs/025-batch-ingest-persistence/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/{queries,migration}.md, quickstart.md

**Tests**: Yes — required. Constitution Principle IV: transaction rollback, retry idempotency and concurrent collision are only meaningful against real Postgres and get integration tests, not mocks.

**Organization**: grouped by user story. US1 (batching) and US2 (atomicity) share the same code path and are sequenced together, with US2's correctness guard landing before US1's performance work so the fix ships even if the optimisation is deferred.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable (different files, no dependency on incomplete work)
- **[Story]**: US1–US3 from spec.md

## Path Conventions

- Repo root: `/home/nnc/Projects/job-finder`; paths below relative to `apps/api/` unless stated
- One migration, sqlc regeneration required, no tygo regeneration, no dashboard change

---

## Phase 0: BLOCKING — prerequisites outside this feature

- [X] T000a **The tree does not compile.** `cd apps/api && go build ./...` fails in seven packages at `ede4b90` (`generation/domain`, `ghostjob/application`, `keyword/infrastructure/rephraseadapter`, `outreach/domain`, `queue`, `recruiter/application`, `salary/application`) — the DDD restructure moved types into `domain/` without updating `application/` references. Branches `fix/ci-build-failures` and `fix/gh-action-compile-errors` are already in flight. Land a green build first; nothing below can be built, tested or measured until then. **Resolved before this branch** — `cd apps/api && go build ./...` is green.
- [X] T000b **Feature 026 (db pool capacity) should land first.** Batching ingestion against a pool of `max(4, NumCPU)` moves the contention rather than removing it, and SC-001 (95% reduction) is not cleanly measurable while the pool is the binding constraint. If 026 is deferred, record that SC-001's measurement is confounded. **Satisfied** — feature 026 landed on this branch first.

---

## Phase 1: Setup & Baseline

- [X] T001 Create the feature branch: `git checkout -b 025-batch-ingest-persistence`. Skipped: work happens on the shared `feat/specs-025-027-implementation` branch.
- [ ] T002 **HARD GATE — capture the baseline before changing anything.** SC-001 and SC-005 are ratios against the pre-change build and are **permanently unverifiable** once any code changes. On the pre-change build, with `log_statement='all'`, run a ~500-posting source and record: (a) statement count against `"Job"`, (b) `SourceRun` duration, (c) the stranded-job count from quickstart.md step 9. Put all three in the PR description. **No Phase 2 task may start until these three numbers exist.** If the baseline cannot be captured (e.g. no source returns 500 postings in this environment), say so explicitly and mark SC-001/SC-005 as unmeasurable rather than proceeding and claiming them later. **NOT DONE — cannot be executed here.** No Postgres is reachable (`localhost:5432` refused) and no source can be run for ~500 live postings, so no pre-change statement count, run duration or stranded-job count exists. **SC-001 and SC-005 are therefore unmeasurable for this change** and must not be claimed. Capture the baseline on a deployment with real data before asserting either.
- [X] T003 Confirm `00032` is still the next free goose version (`ls internal/db/migrations | tail -1`) — another in-flight branch may have claimed it. Versions must be unique and sequential (constitution).

**Checkpoint**: baseline recorded, version reserved.

---

## Phase 2: Foundational (Blocking Prerequisites)

- [X] T004 Write `internal/db/migrations/00032_batch_ingest.sql` exactly as specified in `contracts/migration.md`, including the explanatory comments (they carry the reasoning for the NULL semantics and the functional index).
- [ ] T005 Apply and verify: `lastSeenRunId` present, `Job_lower_company_idx` present, and — the check that actually matters — `EXPLAIN` shows an index scan rather than a sequential scan (contracts/migration.md Verification). An index that exists but is never chosen does not satisfy FR-007. Blocked on the same missing database: migration `00032` is written but has not been applied, and the `EXPLAIN` index-usage check (the one that actually satisfies FR-007) has not been run.
- [X] T006 **Read the `ActivityRun` schema before writing `BulkInsertActivities`.** `contracts/queries.md §6` lists the columns `activity.New` populates, but the table may carry further NOT NULL columns with defaults. Verify against `internal/db/migrations/` and `internal/activity/` rather than trusting the contract.
- [X] T007 Add the six queries to `internal/db/queries/job.sql` and `internal/db/queries/activityrun.sql` per `contracts/queries.md`, verbatim including comments. **Retain** the existing `GetJobByDedupeKey`, `RecordJobRepost`, `InsertJob`, `FindJobByCompany`, `MergeJobBoard` — other call sites and tests use them.
- [X] T008 `make sqlc-generate`, then `git diff --exit-code apps/api/internal/db/sqlcgen` after a second run to confirm determinism. The `.claude/settings.json` PostToolUse hook regenerates on `queries/*.sql` edits automatically — review the output, never hand-edit it.
- [X] T009 Extend the repository port in `internal/jobsources/domain/repository.go` with the six batch methods, keeping the structural-interface convention (method names mirror generated queries so `*sqlcgen.Queries` satisfies it without an adapter).
- [X] T010 Add a transaction port to the ingest handler, mirroring `internal/applications/domain/port.go:44` (`WithinTx(ctx, func(*sqlcgen.Queries) error) error`). Use the existing pattern; do not invent a second one.

**Checkpoint**: schema and data-access layer ready; no behaviour changed yet.

---

## Phase 3: User Story 2 — No partial or double-counted results (Priority: P1) 🎯 MVP

**Goal**: the correctness fix. A run's storage is atomic and a retry cannot inflate sighting counts.

**Independent Test**: quickstart.md steps 5 and 6. Deliverable alone — ships the ghost-signal fix even if batching slips.

**Sequenced before US1 deliberately**: the double-counting bug corrupts user-visible data today; the N+1 only wastes time.

- [X] T011 [US2] Create `internal/jobsources/interfaces/worker/persist.go`; move the persist phase out of `handler.go` (currently 475 lines) into a `persistBatch` function taking the `PostingBatch` from data-model.md §2.
- [X] T012 [US2] Wrap the persist phase in `db.WithinTx` per data-model.md §6. `InsertSourceRun` stays **outside** (a run failing during `adapter.Search` must still record the attempt); `FinishSourceRunOk` moves **inside** (totals must not describe a rolled-back phase — FR-011).
- [X] T013 [US2] Implement the `lastSeenRunId` guard via `BulkRecordJobReposts`. **`IS DISTINCT FROM`, not `!=`** — the column is NULL for every pre-existing row and `NULL != $3` is NULL, which would exclude every existing posting from ever being counted (contracts/queries.md §4).
- [X] T014 [P] [US2] Unit-test the guard logic in `persist_test.go`: same run id twice increments once; different run ids increment twice; NULL initial state increments.
- [X] T015 [US2] Integration test (`//go:build integration`, following `internal/dbtest` conventions): force a failure mid-persist, assert zero postings visible and the run marked failed; then re-run the same run id and assert each `seenCount` rose by exactly one across both attempts. Written (`persist_integration_test.go`: `TestPersistRollsBackWholeRun`, `TestPersistRetryCountsEachSightingOnce`); **compiles under `-tags integration` but was not executed — no Postgres reachable in this environment.**
- [ ] T016 [US2] Verification: quickstart.md steps 5 and 6, including the ten-iteration SC-003 check. Blocked — quickstart steps 5/6 and the ten-iteration SC-003 check need a running system.

**Checkpoint**: US2 complete and independently mergeable. The data-corruption bug is fixed.

---

## Phase 4: User Story 1 — Large runs complete quickly (Priority: P1)

**Goal**: constant database interactions per chunk regardless of posting count.

**Independent Test**: quickstart.md steps 1–4.

- [X] T017 [US1] Implement in-batch deduplication in `persist.go` per data-model.md §2. **Keep the first occurrence, not the last** — adapters return source-ranked order. Count dropped duplicates into `Skipped` and keep them in the run's `found` total.
- [X] T018 [US1] Replace the per-posting `GetJobByDedupeKey` with one `GetJobsByDedupeKeys`, producing the `Classification` structure. Correlate by dedupe key, **never by row order** — absent keys are simply not returned.
- [X] T019 [US1] Convert `FindMergeCandidate` in `dedupe.go` to a batch form over `FindJobsByCompanies`. **Keep `titlesOverlap` in Go**, applied to the batch result — it has existing unit coverage and reimplementing its heuristic in SQL would duplicate tested logic in an untested place (research.md R4).
- [X] T020 [US1] Implement `BulkInsertJobs` with the `unnest` form. Build nullable columns as `[]*string` / `[]*time.Time` so SQL NULL is passed rather than empty strings (`emit_pointers_for_null_types: true` in `sqlc.yaml`). Correlate `RETURNING` rows by `dedupeKey`.
- [X] T021 [US1] Implement `BulkMergeJobBoards` with position-aligned arrays built in lockstep.
- [X] T022 [US1] Implement chunking at `INGEST_PERSIST_CHUNK_SIZE` (default 500) **inside** the transaction. All chunks commit together; a chunk failure rolls back the whole run (data-model.md §3). Add the config key with a default and document it in `.env.example`.
- [X] T023 [P] [US1] Unit-test chunking: 0 postings performs no statements; 1 chunk; exact multiple of chunk size; one over. Plus in-batch duplicate collapse and first-occurrence-wins.
- [X] T024 [US1] Integration test: 500 postings stored with a counted statement total **≤6 per chunk**, not ≤10. SC-002 sets the acceptance bar at 10, but the design guarantees 6 (data-model.md §4) — asserting the looser number means a design regression from 6 to 9 passes silently until the criterion itself breaks. Also assert two concurrent runs storing an overlapping posting both succeed with exactly one row (FR-013). Written (`TestPersistStatementBudgetPerChunk` asserts <=6 per chunk; `TestConcurrentRunsStoreOverlappingPostingOnce` asserts FR-013); **not executed — no Postgres reachable.**
- [ ] T025 [US1] Verification: quickstart.md steps 1–4, 7, 8. Record the T002 baseline against the new numbers. **SC-001 requires ≤5% of baseline storage time** — if not met, that is the finding, report it rather than rounding toward it. Blocked — quickstart steps 1-4, 7, 8 need a running system, and SC-001 has no baseline to compare against (T002).

**Checkpoint**: US1 complete. Storage cost is constant in posting count.

---

## Phase 5: User Story 3 — Downstream work still reaches every posting (Priority: P2)

**Goal**: regression guard. No posting stranded, none queued twice.

**Independent Test**: quickstart.md steps 9 and 10.

- [X] T026 [US3] Implement `BulkInsertActivities`, correlating by `(jobId, kind)` — a job gets two activity rows (`match`, `ghost`), so `jobId` alone is not a key, and row order is not guaranteed (contracts/queries.md §6).
- [X] T027 [US3] Move enqueueing **after** commit, driven only by `PersistResult.Inserted`. Reposted, merged and skipped postings queue nothing — matching today exactly. Route by `NeedsDetail`: enrich for list-only stubs, match + ghost for full postings.
- [X] T028 [P] [US3] Unit-test routing: list-only batch queues enrich only; full batch queues match + ghost; a rolled-back result queues nothing.
- [ ] T029 [US3] Verification: quickstart.md steps 9 and 10. **SC-005 requires the stranded-job count not to increase** against the T002 baseline. Blocked — quickstart steps 9/10 need a running system; SC-005 has no baseline (T002).

**Checkpoint**: all stories complete.

---

## Phase 6: Polish & Cross-Cutting

- [X] T030 Add per-run storage duration and statement count to the run record or log, per FR-014 — the improvement must be observable, not asserted.
- [X] T031 Confirm existing tests still pass unmodified: `internal/jobsources/interfaces/worker/{merge_test,scheduler_test}.go`. If they need changes, understand why before changing them — they encode the dedupe semantics FR-012 requires preserving. Verified unmodified (`git diff` empty) and compiling under `-tags integration`; `scheduler_test.go` runs and passes, `merge_test.go` needs Postgres so it was not executed.
- [ ] T032 Run the full gate: `make test-lint`, `go test -tags integration ./internal/jobsources/...`, and the sqlc drift check. Partially run: `./scripts/golangci-check.sh` (0 issues), `go build ./...`, `go vet ./...` and `go test ./...` all pass, and the sqlc drift check is clean (a second `make sqlc-generate` produces an identical tree). `go test -tags integration ./internal/jobsources/...` compiles but was not run (no Postgres).
- [ ] T033 [P] Record in the PR description: baseline vs new statement count, baseline vs new storage duration, the SC-001 ratio, and the SC-003 ten-iteration result. Blocked — no baseline numbers exist (T002).
- [ ] T034 Open the PR against `master`; confirm CI green before merge. Out of scope for this agent; the PR is opened separately.

---

## Dependencies & Execution Order

### Phase dependencies

- **Phase 0** → blocks everything (tree does not compile)
- **Setup (1)** → after Phase 0
- **Foundational (2)** → after Setup; **blocks all stories**
- **US2 (3)** → after Foundational. First, despite equal priority with US1: it fixes live data corruption
- **US1 (4)** → after US2 (shares `persist.go`, builds on the transaction boundary)
- **US3 (5)** → after US1 (needs `PersistResult.Inserted`)
- **Polish (6)** → after all

### Critical path

T000a → T004 → T007 → T008 → T009/T010 → T012 → T013 → T018 → T020 → T022 → T027

### Parallel opportunities

- T014 ∥ T023 ∥ T028 (test files, different concerns)
- T017 ∥ T019 once T009 lands (different files)
- T033 ∥ T032

### Sequencing note

US1 and US2 are not independently *implementable* — both rewrite the persist phase. They are independently *valuable* and are sequenced so US2's guard lands first. Do not attempt to parallelise them across two people; they collide in `persist.go`.

---

## Implementation Strategy

### MVP (US2 only)

Phases 0–3. Ships the correctness fix — atomic runs, no sighting inflation — without touching the N+1. Independently mergeable, and the one to ship if time runs short.

### Incremental delivery

1. Phase 0 green build → nothing here is possible before this
2. + Foundational → schema and queries exist
3. + US2 → atomicity and retry safety → **merge**
4. + US1 → constant-cost storage → **merge**
5. + US3 → downstream routing verified → **merge**

---

## Notes

- `make sqlc-generate` output is reviewed, never hand-edited. The `sqlc-drift` CI job fails on staleness.
- No DTO changes ⇒ no tygo regeneration, no `packages/shared` edit. If either appears in the diff, something is wrong.
- Conventional commits (`feat:`, `fix:`, `chore:`, `docs:`); commit per task or logical group.
- T002, T025, T029 and T033 are the tasks that decide whether this feature worked. Code that compiles is not evidence.
