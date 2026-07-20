---

description: "Task list for the ghost-job detector"
---

# Tasks: Ghost-Job Detector

**Input**: Design documents from `/specs/005-ghost-job-detector/`

**Prerequisites**: spec.md, plan.md, research.md, data-model.md, quickstart.md

**Tests**: Included. Constitution Principle IV makes them a gate — *"A change is not 'done' until its own language's test suite passes locally."* This change spans `apps/api` and `apps/dashboard`, so `make test-lint` is additionally required before merge. Signal-measurement tests run against fixtures with no LLM; the live model smoke is opt-in behind the `live` build tag.

**Organization**: Grouped by user story so each ships independently. Phases 1-3 are shared foundation; US1 (feed badge) is the first shippable increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Exact file paths included per task

## Path Conventions

All paths are repo-relative from the repository root. This feature touches `apps/api` (new package + migration + query file + HTTP surface), `packages/shared` (regenerated), and `apps/dashboard` (two screens + one component).

---

## Phase 1: Schema (Blocking Prerequisite)

**Purpose**: The `JobSignal` table everything else reads and writes.

- [ ] T001 Create goose migration `apps/api/internal/db/migrations/00007_job_signal.sql` with the `CREATE TABLE "JobSignal"` statement exactly as specified in [data-model.md](./data-model.md) — quoted PascalCase table, quoted camelCase columns, `uuid` PK defaulting to `gen_random_uuid()`, `timestamp (3)` `createdAt`, matching `00001_init.sql` house style. Verify `00007` is still the next free version before writing; **never duplicate a goose version** (Constitution architecture constraint).
- [ ] T002 In the same migration, add `CONSTRAINT "JobSignal_jobId_kind_unique" UNIQUE("jobId","kind")` — this is FR-009's replace-don't-accumulate guarantee and the conflict target every upsert uses. It is deliberately **not** `MatchResult`'s `UNIQUE("jobId")`.
- [ ] T003 In the same migration, add the FK `"JobSignal_jobId_Job_id_fk"` → `"Job"("id")` `ON DELETE cascade ON UPDATE no action` — byte-identical cascade/no-action pairing to `MatchResult_jobId_Job_id_fk` (`00001_init.sql:110`). This is SC-010.
- [ ] T004 In the same migration, add `CREATE INDEX "JobSignal_kind_score_idx" ON "JobSignal" USING btree ("kind","score")` — leading `kind` so future signal kinds never scan each other's rows. Add the `-- +goose Down` block with `DROP TABLE IF EXISTS "JobSignal"`.
- [ ] T005 Apply and verify: `goose up`, then `\d "JobSignal"` shows the FK, the unique constraint, and the index. Then `goose down` / `goose up` to prove the down migration is clean.

**Checkpoint**: Table exists. Query and service work can begin.

---

## Phase 2: Query surface

**Purpose**: sqlc-generated typed access. All four signal measurements are SQL, never model output (SC-006).

- [ ] T006 Create `apps/api/internal/db/queries/jobsignal.sql` with `UpsertJobSignal` — `INSERT … ON CONFLICT ("jobId","kind") DO UPDATE SET "score", "signals", "model", "createdAt" = now()`.
- [ ] T007 [P] Add `GetJobSignal` (one row by `jobId` + `kind`) and `ListJobSignalsByJobIds` (batch, `= ANY($1)`) to `apps/api/internal/db/queries/jobsignal.sql`. The batch query exists so the feed issues one query, not one per card.

### ⚠️ Repost-count trap — read before writing T008

> `Job` carries `Job_dedupeKey_unique`. A `SELECT count(*) FROM "Job" GROUP BY "dedupeKey"` therefore **compiles, runs, looks correct, and can only ever return 1**. It is the most likely bug in this feature. The repost count must come from ingestion appearance/run history, joined on the *stored* `dedupeKey` column — never from a recomputed identity (FR-003), and never from counting `Job` rows. See research Decision 2.

- [ ] T008 Add `CountRepostsByDedupeKey` to `apps/api/internal/db/queries/jobsignal.sql`, counting appearances of the job's stored `dedupeKey` across ingestion runs. Assert in T017 that a value > 1 is reachable.
- [ ] T009 [P] Add `CountCrossBoardDuplicates` — count of **distinct** `"sourceKey"` values other than this job's own, among jobs ingested in the last 60 days whose description hash is within the similarity threshold (FR-005).
- [ ] T010 [P] Add `CountAlwaysHiringByCompany` — jobs with the same `lower("company")`, ingested in the last 90 days, still `"status" = 'found'`, **left-joined to `"Application"` and excluding any job whose application status is not `found`** (FR-006). `Job.status` narrows; the `Application` join is authoritative. `rejected` counts as **progression**, not as unprogressed.
- [ ] T011 Run `sqlc generate` and confirm the generated Go compiles. Do not hand-edit generated files (Principle III).

**Checkpoint**: Typed queries available.

---

## Phase 3: Foundational — types and measurement

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T012 Create `apps/api/internal/ghostjob/types.go` with `GhostJobResult{Score float64, Confidence float64, Explanation string, TopSignals []string}`, `json` + `jsonschema:"minimum=…,maximum=…"` tags, mirroring `apps/api/internal/matching/types.go` field-for-field in style.
- [ ] T013 Add `func (g *GhostJobResult) Validate() error` to `apps/api/internal/ghostjob/types.go` — errors when `Score` is outside 0-100 or `Confidence` outside 0-1. Reproduces the semantic check `llm.CompleteStructured`'s retry loop enforces, exactly as `FitResult.Validate()` does (FR-010).
- [ ] T014 [P] Create `apps/api/internal/ghostjob/ports.go` with the `Repository` interface the service needs, mirroring `apps/api/internal/matching/ports.go`.
- [ ] T015 [P] Create `apps/api/internal/ghostjob/simhash.go` — normalize description text (lowercase, collapse whitespace, strip markup), shingle, and produce a similarity hash. Stdlib only, no new dependency. The duplicate threshold is a **named constant**, tunable without touching the query.
- [ ] T016 Create `apps/api/internal/ghostjob/signals.go` with `GhostSignals{RepostCount int, DaysOpen *int, CrossBoardCount *int, AlwaysHiringCount *int, Notes map[string]string}` and the measurement function that fills it from T008-T010. Every unmeasurable signal is `nil` **plus** a `"unknown: <reason>"` entry in `Notes` — never `0`, never a guess (FR-011).
- [ ] T017 Create `apps/api/internal/ghostjob/signals_test.go` covering: repost count > 1 is reachable (the T008 trap); `DaysOpen` nil on null `postedAt`; `AlwaysHiringCount` nil for `""`/whitespace/punctuation-only/`"Unknown"` company, and two `"Unknown"` jobs **not** grouped; `AlwaysHiringCount == 1` for a one-off company; progressed and `rejected` applications excluded; `CrossBoardCount` distinct-source-only and nil on teaser descriptions; two runs over unchanged fixtures byte-identical (SC-006).
- [ ] T018 [P] Create `apps/api/internal/ghostjob/simhash_test.go` — reformatted copies of one JD hash within threshold; genuinely different JDs do not; Cyrillic text survives normalization.

**Checkpoint**: Signals measurable and tested with no LLM involved.

---

## Phase 4: US1 — Feed badge (Priority: P1) 🎯 MVP

**Goal**: A scored job shows a coloured ghost badge next to its fit score on the feed.

**Independent test**: Score a few jobs, open the feed, confirm yellow at 50-79, red at 80-100, nothing below 50, nothing for unscored jobs.

- [ ] T019 [US1] Add `LLMModelGhost string \`mapstructure:"LLM_MODEL_GHOST"\`` to `apps/api/internal/config/config.go`, resolved through the existing `ModelOr` fallback, mirroring `LLM_MODEL_MATCH`.
- [ ] T020 [US1] Create `apps/api/internal/ghostjob/service.go`: measure signals → build the prompt → `llm.CompleteStructured[GhostJobResult]` → round `Score` to `int` → upsert. The prompt hands the model the four measured numbers and forbids asserting anything the signals do not support (FR-019, Principle II). It must state that (a) an always-hiring count of 1 is **no evidence** (SC-004) and (b) cross-board duplication alone never justifies the red band (agency cross-post edge case).
- [ ] T021 [US1] In `apps/api/internal/ghostjob/service.go`, implement the decline-to-score path: when `RepostCount == 1` and all three optional signals are `nil`, make **no LLM call** and write **no row** (spec edge case; keeps SC-003 true).
- [ ] T022 [US1] In `apps/api/internal/ghostjob/service.go`, ensure a validation failure past the retry budget returns an error and persists nothing, leaving any prior row untouched (FR-010). Ensure a scoring failure for one job never propagates to another job or to the ingestion run (FR-018, SC-009).
- [ ] T023 [US1] Create `apps/api/internal/ghostjob/service_test.go` with a fake `llm.Provider`: out-of-range score persists nothing and preserves the prior row; two upserts leave exactly one row with the later values (FR-009); every persisted row carries score/model/confidence and a value-or-explicit-unknown for all four signals (SC-003); deleting the job cascades (SC-010).
- [ ] T024 [US1] Add `JobSignalDto` to `apps/api/internal/dto/dto.go` (tygo-exported: score, kind, model, createdAt, and the typed signals breakdown with nullable signal values).
- [ ] T025 [US1] Wire the ghost signal onto the job list payload in `apps/api/internal/httpapi/jobs.go` using the batch query from T007 — one query for the page, not one per job.
- [ ] T026 [US1] Construct and register the ghost service in `apps/api/cmd/server/main.go`, and invoke scoring after ingestion alongside fit scoring — asynchronously, so scoring latency never blocks or fails an ingestion run.
- [ ] T027 [US1] Regenerate the shared TS types (tygo) and run `pnpm --filter @job-finder/shared build`. Do not hand-write the TS mirror (Principle III).
- [ ] T028 [US1] Add a `GhostBadge` component to `apps/dashboard/src/components/ui.tsx`: renders nothing below 50, yellow for 50-79, red for 80-100, nothing when no signal exists (FR-012, FR-017).
- [ ] T029 [US1] Render `<GhostBadge>` next to `<ScoreBadge>` in the feed card in `apps/dashboard/src/features/feed/FeedPage.tsx` (~line 151). **Do not** add any filter, sort, opacity, or hide behaviour keyed on the ghost score — the feed's membership and ordering must be identical with the feature on and off (FR-015, SC-007).
- [ ] T030 [P] [US1] Add `GhostBadge` cases to `apps/dashboard/src/components/ui.test.tsx`: each band, the no-signal case, and a snapshot proving an unscored card is unchanged from today (SC-008).

**Checkpoint**: US1 ships. Feed badge works end-to-end without US2 or US3.

---

## Phase 5: US2 — Detail breakdown panel (Priority: P2)

**Goal**: The flagged job's detail page explains itself.

**Independent test**: Open a scored job, confirm score, confidence, model, all four signal values, and a readable explanation.

- [ ] T031 [US2] Expose the full ghost signal (score, confidence, model, createdAt, all four signal values, notes, explanation) on the job-detail payload in `apps/api/internal/httpapi/jobs.go`.
- [ ] T032 [US2] Add a ghost breakdown panel to `apps/dashboard/src/features/job-detail/JobDetailPage.tsx` (~line 148, beside `FitSummary`): score, confidence, model, one row per signal, and the explanation.
- [ ] T033 [US2] In the same panel, render an unmeasured signal as **"unknown"** with its reason from `notes` — never as `0` (FR-011). Render low confidence visibly enough that a weak verdict is not read as a strong one (Story 2, scenario 5).
- [ ] T034 [US2] In the same panel, handle the never-scored job: the panel is absent or shows an explicit "not scored yet" state, never an empty panel of zeroes (Story 2, scenario 4).
- [ ] T035 [P] [US2] Add `vitest` coverage in `apps/dashboard/src/features/job-detail/` for the scored, unknown-signal, low-confidence, and never-scored states.

**Checkpoint**: US2 ships independently of US3.

---

## Phase 6: US3 — Manual re-score (Priority: P3)

**Goal**: One button, one job, on demand. No scheduler.

**Independent test**: Open a scored job, press refresh, confirm the panel updates in place.

- [ ] T036 [US3] Create `apps/api/internal/httpapi/ghostjob.go` with `POST /api/jobs/{id}/ghost-score`, registered in `apps/api/internal/httpapi/router.go`. Recomputes and upserts; returns the new signal.
- [ ] T037 [US3] Confirm **no scheduled or background re-scoring path exists anywhere** — scoring is triggered by ingestion and by this endpoint only (FR-014, Story 3 scenario 5). Grep the queue/cron wiring to prove it.
- [ ] T038 [P] [US3] Add `apps/api/internal/httpapi/ghostjob_test.go`: happy path; model-unreachable returns an error with the prior row intact (Story 3, scenario 4); concurrent double-invocation is idempotent via the `(jobId, kind)` upsert.
- [ ] T039 [US3] Add the refresh mutation and cache key to `apps/dashboard/src/api/api.ts` and `apps/dashboard/src/api/queryKeys.ts`, invalidating the job-detail query so the panel updates without a page reload.
- [ ] T040 [US3] Add the refresh button to the panel in `apps/dashboard/src/features/job-detail/JobDetailPage.tsx`, **disabled while the request is in flight** so a double-click cannot start two runs (Story 3, scenario 3). Surface an error via the existing toast bus on failure, leaving the displayed score intact.

**Checkpoint**: All three stories shipped.

---

## Phase 7: Polish and gates

- [ ] T041 [P] Add `apps/api/internal/ghostjob/live_smoke_test.go` behind the `live` build tag (`TestLive_GhostScore`): a real Ollama call returns a schema-valid result, and the explanation asserts nothing the signals do not support (Principle II).
- [ ] T042 Walk [quickstart.md](./quickstart.md) Level 3 end-to-end against `make dev`, including the ordering check (SC-007) and the fit-score independence check (FR-008).
- [ ] T043 Walk [quickstart.md](./quickstart.md) Level 4 failure-mode table in full, including the legitimate-agency-cross-post case staying below 80.
- [ ] T044 Run `make test-lint`. **Mandatory** — this change crosses `apps/api` and `apps/dashboard` (Principle IV).
- [ ] T045 Constitution Principle I audit: grep the feature for any path that hides, filters, reorders, or auto-rejects a job based on its ghost score. There must be none. Confirm the feed list is identical with the feature on and off.

---

## Dependencies

- **Phase 1 → Phase 2 → Phase 3**: strictly sequential. Nothing above can be skipped.
- **Phase 3 → Phase 4/5/6**: all three stories depend on measurement existing.
- **US1 → US2 → US3**: in priority order, but US2 and US3 touch different files (panel vs endpoint + mutation) and can be worked in parallel once US1's service and DTO exist.
- **T027 (shared rebuild) blocks every dashboard task** — the dashboard will not typecheck until `packages/shared` is rebuilt.

## Parallel opportunities

- T007 / T009 / T010 — same file, different queries; coordinate or serialize.
- T014 / T015 — different files, no dependency.
- T017 / T018 — different test files.
- T030 / T035 / T038 — independent test files across three surfaces.
