---

description: "Task list for salary inference"
---

# Tasks: Salary Inference

**Input**: Design documents from `/specs/006-salary-inference/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md

**Tests**: Included. Constitution Principle IV makes them a gate. This feature spans `apps/api` and `apps/dashboard`, so `make test-lint` is binding, not optional.

**Organization**: Grouped by user story so each ships independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 / US2 / US3, or Setup/Polish

## Path Conventions

Repo-relative from the repository root.

---

## Blocking decisions before implementation

Two open items from [plan.md](./plan.md). Neither blocks Phase 1 or 2.

- [ ] **D1 — Reference dataset licensing (Director).** The "levels.fyi public CSV" premise does not survive checking: no official open export exists, scraping their site is against their terms, and the redistributed scrapes carry a third party's licence claim ([research.md](./research.md) Decision 1). Select an explicitly-licensed dataset instead. **Blocks Phase 4 (dataset source) only** — everything else proceeds. `"SalaryCache"` is dataset-agnostic by design so this lands late at no structural cost.
- [ ] **D2 — Story 3 breakdown persistence.** `BlendedEstimate.Components` has nowhere to live given the no-related-table constraint. Recompute-on-request costs a model call per page view; one JSON column on `"Job"` is the likely answer ([research.md](./research.md) Open question). **Blocks Phase 6 (US3) only.**

---

## Phase 1: Schema

- [ ] T001 **Verify migration number 00009 is free** — tree holds 00001–00006, siblings are landing 00007/00008. The constitution requires goose versions be unique and sequential; renumber if taken.
- [ ] T002 Write `apps/api/internal/db/migrations/00009_salary_inference.sql` — up: five `ALTER TABLE "Job" ADD COLUMN` statements (`"salaryMin"` int, `"salaryMax"` int, `"salaryCurrency"` text, `"salaryConfidence"` double precision, `"salarySource"` text), the partial index `"Job_salary_floor_idx"` on `"salaryMax" WHERE "salaryMax" IS NOT NULL`, and `CREATE TABLE "SalaryCache"` per [data-model.md](./data-model.md) including the unique constraint on `("titleBucket","geoBucket","companySizeBucket","source")`. Down: drop all of it.
- [ ] T003 Write `apps/api/internal/db/queries/salary.sql` — `UpsertSalaryCacheEntry` (on-conflict against the natural key), `LookupSalaryBucket`, `CountSalaryCache`.
- [ ] T004 Regenerate sqlc; confirm the generated `Job` struct carries the five new fields. Do not hand-edit generated files (Principle III).
- [ ] T005 Integration test: migration applies and rolls back cleanly; the cache upsert is idempotent across two loads (row count unchanged).

**Checkpoint**: schema in place, typed access generated.

---

## Phase 2: Parser and blender (foundational — everything depends on these)

Ships first because the parser produces the labelled held-out set SC-004 is measured against ([research.md](./research.md) Decision 2). Without it, every accuracy claim later is unfalsifiable.

- [ ] T006 [P] `apps/api/internal/salary/types.go` — `SalaryBand` (Min, Max, Currency, Period), `SourceEstimate`, `BlendedEstimate`, and the `Source` interface each of the three sources implements. `SalaryBand`'s field names are part of the `CompleteStructured` prompt contract; they are not free to rename later.
- [ ] T007 `apps/api/internal/salary/parse.go` — regex table over the patterns actually in the corpus. Resolve ambiguous symbols (`$` = USD/CAD/AUD) using the posting's geo. Normalize to annual before returning.
- [ ] T008 `apps/api/internal/salary/parse_test.go` — table-driven per [quickstart.md](./quickstart.md) Level 1. Must cover: every currency in the corpus, en-dash ranges, trailing symbols, `up to` / `від` open-ended forms, and non-numeric refusals parsing to **nothing rather than zero** (a `0`–`0` band is hidden by every non-zero floor).
- [ ] T009 `apps/api/internal/salary/blend.go` — confidence-weighted average, weights normalized, final confidence the **sum capped at 1**.
- [ ] T010 `apps/api/internal/salary/blend_test.go` — including the two traps: three sources at 0.5 yield 1.0 not 1.5, and **a single source at 0.4 blends to 0.4, not 1.0**.
- [ ] T011 [P] `apps/api/internal/salary/bucket.go` — title normalization with seniority extracted as a separate signal, country-level geo with `"*"` catch-all, company-size inference defaulting to `unknown`.
- [ ] T012 [P] `apps/api/internal/salary/bucket_test.go`.
- [ ] T013 Backfill command/task: parse `salaryRaw` across ingested jobs, write parsed bands with `"salarySource" = 'posting'`. This populates both the ingested-cache source and the held-out set.

**Checkpoint**: `go test ./internal/salary/...` green; corpus has parsed bands.

---

## Phase 3: US1 — inferred band on the card (P1) 🎯 MVP

- [ ] T014 [US1] `apps/api/internal/salary/source_llm.go` — `CompleteStructured[SalaryBand]` against the local provider (FR-025). Prompt carries title, seniority, geo, company-size hints, required skills. Must ask for `Period` explicitly; a model asked about a Kyiv salary answers monthly by default.
- [ ] T015 [P] [US1] `apps/api/internal/salary/source_ingested.go` — query parsed bands for the job's bucket; confidence scales with sample size.
- [ ] T016 [US1] `apps/api/internal/salary/service.go` — run each source independently (FR-003), isolate per-source failure (FR-023), let a `posting` band replace rather than blend (FR-008), skip jobs that already have a band (FR-022), write all five columns or none (FR-009, SC-002).
- [ ] T017 [US1] Wire the service into the background work path in `apps/api/cmd/server/main.go`.
- [ ] T018 [US1] Expose the band on the job DTO through the existing tygo path — never a hand-written TS duplicate (Principle III).
- [ ] T019 [P] [US1] `apps/dashboard/src/features/feed/FeedPage.tsx` — render the band where `salaryRaw` renders today; keep `salaryRaw` displayed alongside (FR-024); mark confidence `< 0.3` as low confidence (FR-006); band-less jobs fall back to current display.
- [ ] T020 [P] [US1] `vitest` coverage for the band component: band, low-confidence band, no band, band-plus-`salaryRaw`.
- [ ] T021 [US1] Integration test: inference over a job set, then a second run issuing **zero** model calls (SC-009); and a direct query for partial-band rows returning zero (SC-002).

**Checkpoint**: US1 independently shippable. Feed shows bands.

---

## Phase 4: dataset source (blocked on D1)

- [ ] T022 Select the dataset per D1 and record the choice and its licence in [research.md](./research.md).
- [ ] T023 `apps/api/internal/salary/loader.go` — retrieve once at startup, `encoding/csv` parse, bucket, upsert. **Non-blocking**: failure logs and continues on the cached copy (SC-010).
- [ ] T024 `apps/api/internal/salary/source_dataset.go` — bucket lookup with fallback widening (exact → size `unknown` → geo `*`), lowering confidence at each widening.
- [ ] T025 Tests: loader idempotence, fallback widening order, and confidence decreasing monotonically as buckets widen.
- [ ] T026 Integration test: dead dataset URL at startup → server starts, feed serves, cached buckets still answer (SC-010).

**Checkpoint**: three sources blending. If D1 stalls, US1 ships on two sources at reduced SC-004 accuracy.

---

## Phase 5: US2 — floor filter (P2)

- [ ] T027 [US2] `apps/api/internal/config/config.go` — `SalaryFloorUsd` with `mapstructure:"SALARY_FLOOR_USD"`, alongside the existing viper-loaded fields.
- [ ] T028 [US2] Currency conversion for floor comparison (FR-020). A band whose currency cannot be converted is **not** filtered — unfilterable fails open.
- [ ] T029 [US2] `apps/api/internal/db/queries/joblist.sql` — add the floor predicate to `ListJobsByScore`, `ListJobsByDate`, **and `CountJobs`**. Divergence here makes pagination totals disagree with page contents.
  - Floor `0` omits the predicate entirely rather than evaluating `> 0` (FR-018).
  - The predicate must let `NULL` `"salaryMax"` pass. `"salaryMax" >= $1` silently drops `NULL` rows and exactly inverts FR-019 — this is the single most likely bug in the feature.
- [ ] T030 [US2] `apps/api/internal/httpapi/` — accept the below-floor toggle param, default to filtering on.
- [ ] T031 [P] [US2] `apps/dashboard/src/lib/api.ts` — one optional field on `JobFilters`; existing fields untouched.
- [ ] T032 [US2] `apps/dashboard/src/features/feed/FeedPage.tsx` — below-floor filter chip on by default; toggling off reveals below-floor jobs, each with a red below-floor marker (FR-016).
- [ ] T033 [P] [US2] `vitest`: chip defaults on, toggling refetches, revealed jobs carry the marker.
- [ ] T034 [US2] Integration tests per [quickstart.md](./quickstart.md) Level 3 items 4–10: below-floor hidden by default, revealed on toggle, straddling band stays visible, band-less job stays visible, floor `0` disables, **status unchanged and still `found`** (FR-017, SC-007), pagination totals agree with rows returned.

**Checkpoint**: US2 independently shippable.

---

## Phase 6: US3 — estimate breakdown (P3, blocked on D2)

- [ ] T035 [US3] Implement D2's decision — persist `Components` (likely one JSON column, needing its own migration) or recompute.
- [ ] T036 [US3] Expose the breakdown on the job detail endpoint.
- [ ] T037 [US3] `apps/dashboard/src/features/job-detail/JobDetailPage.tsx` — list each contributing source with its own band and confidence; a single-source estimate implies no blending; a `posting`-sourced band names the posting and shows no estimate (FR-021, SC-008).
- [ ] T038 [P] [US3] `vitest` for the three breakdown shapes: blended, single-source, posting-stated.

---

## Phase 7: settings surface (separate scope)

**`SALARY_FLOOR_USD` alone satisfies FR-014 on the API side.** The dashboard entry requires a settings feature that does not exist — `apps/dashboard/src/features/` today holds feed, job-detail, profile, sources, status, tailor, and tracker. Building it means a route, a persistence path for user-set config, and the API endpoints behind it. That is plausibly larger than the rest of this feature's dashboard work combined, and it is why this is its own phase and should be its own task rather than absorbed into US2.

- [ ] T039 Create a separate task for the settings surface; this phase is a placeholder pointing at it.

---

## Phase 8: Polish

- [ ] T040 SC-003 measurement: hand-check a parser sample spanning every currency in the corpus; ≥95% match.
- [ ] T041 SC-004 measurement: held-out estimate accuracy ≥60% band-contains-truth, **and confidence correlating with correctness**. The correlation is the substantive check — a wrong-but-honestly-low-confidence source is behaving correctly; a confidently wrong one is the harm this feature can actually cause.
- [ ] T042 SC-011 regression: with no floor and no bands, feed ingestion/scoring/display unchanged. Existing feed tests run unmodified.
- [ ] T043 Principle I audit: grep the feature for any write to `"Job"."status"`. There must be none.
- [ ] T044 `make test-lint` — binding, since this spans two apps.

---

## Dependencies

- **Phase 1** → everything.
- **Phase 2** → all three sources. The parser also gates the SC-004 measurement in Phase 8.
- **Phase 3 (US1)** → Phase 5 (nothing to filter without a band).
- **Phase 4** is independent of Phase 5 and gated only by D1.
- **Phase 6** gated by D2.
- **Phase 7** is independent of everything.

## Suggested MVP

Phases 1–3. Delivers Story 1 on two sources — the feature's whole value proposition — without waiting on the dataset licensing decision.
