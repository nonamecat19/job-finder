# Plan: 006 Salary Inference — Frontend + DTO Wiring

**Status**: Backend done, frontend missing. The salary inference pipeline is fully implemented but the inferred data never reaches the dashboard.

**Spec**: `specs/006-salary-inference/spec.md`
**Tasks**: `specs/006-salary-inference/tasks.md`

## What Exists

| Layer | Status |
|-------|--------|
| SQL migration `00009_salary_inference.sql` | Done — salary columns on Job + SalaryCache table |
| Go `internal/salary/` package | Done — parser, blender, LLM source, ingested source, service |
| Asynq worker (`TypeSalaryInfer`) | Done — wired in main.go |
| Enrichment trigger | Done — `enqueueSalaryInfer` after every enrichment |
| Config (`SALARY_FLOOR_USD`) | Done — in .env.example |
| Go DTO (`JobDto`) | **Missing** — only has `SalaryRaw`, not inferred fields |
| Dashboard salary display | **Missing** — only shows raw salary text |
| Floor filter UI | **Missing** |
| Settings surface | **Missing** |

## Tasks

### 1. Expose inferred salary in Go DTO

- [ ] **1.1** Add `SalaryMin`, `SalaryMax`, `SalaryCurrency`, `SalaryConfidence`, `SalarySource` to `JobDto` in `apps/api/internal/dto/dto.go`
- [ ] **1.2** Map the fields in the DTO constructor (where Job model → JobDto)
- [ ] **1.3** Regenerate shared TS types via tygo: `pnpm --filter @job-finder/shared build`

### 2. Dashboard salary display

- [ ] **2.1** In `FeedPage.tsx`, render inferred band where `salaryRaw` renders today
- [ ] **2.2** Keep `salaryRaw` displayed alongside the band (FR-024)
- [ ] **2.3** Mark confidence < 0.3 as "low confidence" visually
- [ ] **2.4** Band-less jobs fall back to current display (salaryRaw only)
- [ ] **2.5** Add vitest coverage: band, low-confidence band, no band, band+salaryRaw

### 3. Floor filter (US2)

- [ ] **3.1** Add floor predicate to `ListJobsByScore`, `ListJobsByDate`, and `CountJobs` in SQL queries
  - Floor 0 omits predicate entirely
  - NULL salaryMax must pass through (not be dropped)
- [ ] **3.2** Add below-floor toggle param to HTTP handler
- [ ] **3.3** Add `SalaryFloorUsd` to dashboard API client (`JobFilters`)
- [ ] **3.4** Add below-floor filter chip to `FeedPage.tsx` (on by default)
- [ ] **3.5** Toggle off reveals below-floor jobs with red marker
- [ ] **3.6** Vitest: chip defaults on, toggling refetches, revealed jobs carry marker

### 4. Estimate breakdown (US3 — optional)

- [ ] **4.1** Expose source breakdown on job detail endpoint
- [ ] **4.2** Add breakdown panel to `JobDetailPage.tsx`
- [ ] **4.3** List each contributing source with its own band and confidence

### 5. Verify

- [ ] **5.1** `make test-lint` passes
- [ ] **5.2** Feed shows inferred salary bands
- [ ] **5.3** Floor filter hides below-floor jobs
- [ ] **5.4** Toggle reveals hidden jobs with marker

## Dependencies
- Backend salary pipeline already complete
- Existing feed and job-detail screens
- tygo for shared type regeneration
