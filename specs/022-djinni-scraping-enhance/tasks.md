# Tasks: Djinni Scraping Enhancement

**Input**: Design documents from `specs/022-djinni-scraping-enhance/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/api.md, quickstart.md

**Tests**: Not explicitly requested in spec. Backend regex parsing logic will have unit tests added to existing `djinni_test.go` per constitution Principle IV.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Includes exact file paths

## Path Conventions

- **Backend**: `apps/api/internal/`
- **Frontend**: `apps/dashboard/src/`
- **Shared types**: `packages/shared/src/`

---

## Phase 1: Setup

**Purpose**: Verify environment, no structural changes needed (existing project)

- [x] T001 Verify Postgres DB is accessible and goose is on the latest migration via `make up`

- [x] T002 [P] Verify all Go dependencies installed via `cd apps/api && go mod tidy`

- [x] T003 [P] Verify shared package is built via `pnpm --filter @job-finder/shared build`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: DB migration, sqlc regeneration, typed contract updates — must complete before any user story

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Database Migration

- [x] T004Create migration file `00031_djinni_scraping_enhance.sql` in `apps/api/internal/db/migrations/` adding columns: `experience_level` TEXT, `experience_min_years` INTEGER, `english_level` TEXT, `salary_estimate_raw` TEXT, `salary_estimate_min` INTEGER, `salary_estimate_max` INTEGER, `salary_estimate_currency` TEXT — all nullable

- [x] T005Apply migration via `cd apps/api && goose -dir internal/db/migrations up`

### Core Typed Contract Updates (Go Backend)

- [x] T006Regenerate sqlc code via `make sqlc-generate` to pick up new Job columns in `apps/api/internal/db/sqlcgen/models.go`

- [x] T007Update `UpdateJobDetail` SQL query in `apps/api/internal/db/queries/job.sql` to include new columns: `experience_level`, `experience_min_years`, `english_level`, `salary_estimate_raw`, `salary_estimate_min`, `salary_estimate_max`, `salary_estimate_currency` — each using `COALESCE(sqlc.narg('...'), "...")`; also add `company` with `COALESCE(NULLIF(sqlc.arg('company'), ''), "company")`

- [x] T008Regenerate sqlc code via `make sqlc-generate` to pick up updated `UpdateJobDetailParams` struct

- [x] T009[P] Add new fields to domain model `Job` struct in `apps/api/internal/domain/job.go`: `ExperienceLevel *string`, `ExperienceMinYears *int`, `EnglishLevel *string`, `SalaryEstimateRaw *string`, `SalaryEstimateMin *int`, `SalaryEstimateMax *int`, `SalaryEstimateCurrency *string`

- [x] T01- [x] T010 1[P] Add new optional fields to `NormalizedJob` struct in `apps/api/internal/dto/dto.go`: `ExperienceLevel *string`, `ExperienceMinYears *int`, `EnglishLevel *string`, `SalaryEstimateRaw *string`, `SalaryEstimateMin *int`, `SalaryEstimateMax *int`, `SalaryEstimateCurrency *string`

- [x] T01- [x] T011 1[P] Add new fields to `JobDto` struct in `apps/api/internal/dto/dto.go`: `ExperienceLevel *string`, `ExperienceMinYears *int`, `EnglishLevel *string`, `SalaryEstimateRaw *string`, `SalaryEstimateMin *int`, `SalaryEstimateMax *int`, `SalaryEstimateCurrency *string`, `SalaryIsEstimated bool`

- [x] T01- [x] T012 1Update `jobToDto()` mapping function in `apps/api/internal/jobs/service.go` to map new fields from sqlcgen `Job` to `JobDto`, with `SalaryIsEstimated` computed as true when `SalaryEstimateMin` or `SalaryEstimateMax` is non-nil

### Core Typed Contract Updates (Shared TS)

- [x] T01- [x] T013 1[P] Add new optional fields to `NormalizedJob` interface in `packages/shared/src/index.ts`: `experienceLevel?`, `experienceMinYears?`, `englishLevel?`, `salaryEstimateRaw?`, `salaryEstimateMin?`, `salaryEstimateMax?`, `salaryEstimateCurrency?`

- [x] T01- [x] T014 1[P] Add new fields to `JobDto` interface in `packages/shared/src/index.ts`: `experienceLevel?`, `experienceMinYears?`, `englishLevel?`, `salaryEstimateRaw?`, `salaryEstimateMin?`, `salaryEstimateMax?`, `salaryEstimateCurrency?`, `salaryIsEstimated` (required bool)

**Checkpoint**: Foundation ready — DB columns exist, typed contracts are defined across all 8 layers. User story implementation can now begin.

---

## Phase 3: User Story 1 - Accurate Company Name Extraction (Priority: P1) 🎯 MVP

**Goal**: Fix "Unknown" company names on Djinni listings by extracting company from detail page with correct selectors

**Independent Test**: Open a Djinni job in the dashboard (e.g., `6c800802-...`) and verify company shows "Novacore" instead of "Unknown"

### Implementation for User Story 1

- [x] T015[US1] Update `DjinniDetailPatch` struct in `apps/api/internal/jobsources/adapters/djinni.go` to add `Company string` field

- [x] T016[US1] Add company extraction to `FetchDetail()` in `apps/api/internal/jobsources/adapters/djinni.go`: try CSS selector `a[href*="/company-"]` first, fall back to `<title>` tag parsing with regex `(.+?) в (.+?) – Djinni`, fall back to empty string

- [x] T017[US1] Update `enrichDjinni()` in `apps/api/internal/enrichment/handler.go` to pass `patch.Company` to `UpdateJobDetail` params as the `company` field (using `COALESCE(NULLIF(...))` logic already in SQL from T007)

- [x] T018[US1] Add unit test for company extraction in `apps/api/internal/jobsources/adapters/djinni_test.go`: test with mock HTML containing `a[href="/jobs/company-novacore/"]`, `<title>JobTitle в CompanyName – Djinni</title>`, and empty cases

**Checkpoint**: Company names should now populate correctly for Djinni listings. Test by re-scraping the example vacancy and verifying "Novacore" appears.

---

## Phase 4: User Story 2 - Required Experience Level Extraction (Priority: P2)

**Goal**: Extract required years of experience from Djinni job descriptions and analytics metadata

**Independent Test**: Query `SELECT experience_level, experience_min_years FROM "Job" WHERE "externalId" = '774850-full-stack-developer'` and verify "2+ years" / 2

### Implementation for User Story 2

- [x] T0*19 [US2] Update `DjinniDetailPatch` struct in `apps/api/internal/jobsources/adapters/djinni.go` to add `ExperienceLevel *string` and `ExperienceMinYears *int` fields

- [x] T0*20 [US2] Add experience extraction regex patterns to `FetchDetail()` in `apps/api/internal/jobsources/adapters/djinni.go`: English pattern `(?i)(\d+)\+?\s*(?:years?|yrs?\.?)\s*(?:of\s+)?(?:(?:commercial|professional|full-stack|software)\s+)?(?:development)?\s*experience` and Ukrainian patterns `(?:від|не менше|мінімум)?\s*(\d+)\+?\s*(?:рок\S*|р\.|р)(?:\s+досвід)` and `досвід\s+(?:роботи\s+)?(?:від\s+)?(\d+)` applied to plain-text description

- [x] T0*21 [US2] Add fallback experience extraction in `FetchDetail()` in `apps/api/internal/jobsources/adapters/djinni.go`: parse `exp=N` from salary analytics URL `a[href*="/salaries/"]` when text regex produces no match

- [x] T0*22 [US2] Update `enrichDjinni()` in `apps/api/internal/enrichment/handler.go` to pass `patch.ExperienceLevel` and `patch.ExperienceMinYears` to `UpdateJobDetail` params

- [x] T0*23 [US2] Add unit test for experience extraction in `apps/api/internal/jobsources/adapters/djinni_test.go`: test patterns against "2+ years of commercial full-stack experience", "від 2 років досвіду", "досвід від 3 років", no-match cases, ambiguous cases ("experience with React" should NOT match)

**Checkpoint**: Experience level is extracted and stored from both text and analytics URL fallback.

---

## Phase 5: User Story 3 - Salary Information Extraction (Priority: P2)

**Goal**: Capture Djinni salary analytics estimates as separate fields distinct from employer-disclosed salary

**Independent Test**: Query `SELECT salary_estimate_raw, salary_estimate_min, salary_estimate_max FROM "Job" WHERE "externalId" = '774850-full-stack-developer'` and verify $1500-3000 range

### Implementation for User Story 3

- [x] T0*24 [US3] Update `DjinniDetailPatch` struct in `apps/api/internal/jobsources/adapters/djinni.go` to add `SalaryEstimateRaw *string`, `SalaryEstimateMin *int`, `SalaryEstimateMax *int`, `SalaryEstimateCurrency *string` fields

- [x] T0*25 [US3] Add salary analytics extraction to `FetchDetail()` in `apps/api/internal/jobsources/adapters/djinni.go`: select `div.salaries-info-link strong#salary-suggestion` text, then pass raw text through `ParseSalaryRaw()` from `apps/api/internal/salary/parser.go` to extract numeric min/max/currency

- [x] T0*26 [US3] Update `enrichDjinni()` in `apps/api/internal/enrichment/handler.go` to pass `patch.SalaryEstimateRaw`, `patch.SalaryEstimateMin`, `patch.SalaryEstimateMax`, `patch.SalaryEstimateCurrency` to `UpdateJobDetail` params

- [x] T0*27 [US3] Add unit test for salary analytics extraction in `apps/api/internal/jobsources/adapters/djinni_test.go`: test selector against mock HTML with `#salary-suggestion`, test ParseSalaryRaw integration, test case with no analytics widget present

**Checkpoint**: Salary analytics estimates are captured and stored separately from employer salary.

---

## Phase 6: User Story 4 - English Level Extraction (Priority: P3)

**Goal**: Extract English proficiency level from Djinni job descriptions

**Independent Test**: Query `SELECT english_level FROM "Job" WHERE "externalId" = '774850-full-stack-developer'` and verify "B1+"

### Implementation for User Story 4

- [x] T0*28 [US4] Update `DjinniDetailPatch` struct in `apps/api/internal/jobsources/adapters/djinni.go` to add `EnglishLevel *string` field

- [x] T0*29 [US4] Add English level extraction regex patterns to `FetchDetail()` in `apps/api/internal/jobsources/adapters/djinni.go`: patterns covering English ("English level — B1+", "English: Upper-Intermediate") and Ukrainian ("Англійська — B1", "рівень англійської: Intermediate") applied to plain-text description, extracting first match

- [x] T0*30 [US4] Update `enrichDjinni()` in `apps/api/internal/enrichment/handler.go` to pass `patch.EnglishLevel` to `UpdateJobDetail` params

- [x] T0*31 [US4] Add unit test for English level extraction in `apps/api/internal/jobsources/adapters/djinni_test.go`: test against "English level — B1+", "English: Upper-Intermediate", "Англійська — B2", "рівень англійської: Advanced", no-match case, and description with multiple language mentions (should extract first English match)

**Checkpoint**: English proficiency level is extracted and stored from job descriptions in both English and Ukrainian.

---

## Phase 7: User Story 5 - Enhanced Job Detail Dashboard Components (Priority: P3)

**Goal**: Display new fields (experience, English level, salary estimate) as metadata chips and tiles on the job detail page

**Independent Test**: Open any Djinni job in the dashboard and verify experience level badge, English level badge, and salary estimate card render when data is present, and gracefully omit when absent

### Implementation for User Story 5

- [x] T032[P] [US5] Create `ExperienceBadge` component in `apps/dashboard/src/features/job-detail/components/ExperienceBadge.tsx`: renders a `Chip` with experience level text and optional years count; only renders when `job.experienceLevel` is non-null

- [x] T033[P] [US5] Create `EnglishLevelBadge` component in `apps/dashboard/src/features/job-detail/components/EnglishLevelBadge.tsx`: renders a `Chip` with English level text (e.g., "B1+"); only renders when `job.englishLevel` is non-null

- [x] T034[P] [US5] Create `SalaryEstimateCard` component in `apps/dashboard/src/features/job-detail/components/SalaryEstimateCard.tsx`: renders a tile in DashboardGrid with formatted salary range and "Estimated" label; only renders when `job.salaryIsEstimated` is true; if `job.salaryMin`/`job.salaryMax` are also present (employer salary), show the estimate alongside as supplementary

- [x] T035[US5] Update `JobMeta` component in `apps/dashboard/src/features/job-detail/JobDetailPage.tsx` to render `ExperienceBadge` and `EnglishLevelBadge` alongside existing metadata chips (sourceKey, status)

- [x] T036[US5] Update `JobDetailPage` component in `apps/dashboard/src/features/job-detail/JobDetailPage.tsx` to include `SalaryEstimateCard` in the DashboardGrid tile layout (alongside CompanyIntel, Contact, etc.)

- [x] T037[US5] Update dashboard hooks in `apps/dashboard/src/features/job-detail/hooks.ts` if needed to ensure new fields are fetched by the job detail query (TanStack Query should pick up new fields automatically from API response, but verify `DetailedJob` type includes new fields)

**Checkpoint**: All new metadata fields are rendered on the job detail page. Absent fields gracefully omit their components.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Regenerate types, run full test suite, validate end-to-end

- [x] T038Run `tygo generate` from `apps/api/` to regenerate `packages/shared/src/generated.ts` with new Go DTO fields

- [x] T039Rebuild shared package via `pnpm --filter @job-finder/shared build`

- [x] T040[P] Verify Go backend compiles via `cd apps/api && go build ./...`

- [x] T041[P] Verify dashboard builds via `pnpm --filter @job-finder/dashboard build`

- [x] T042Run full test suite via `make test-lint` and fix any failures

- [x] T043Validate using quickstart.md scenarios: scrape the example Djinni job, query DB for new fields, hit the API endpoint, and visually verify dashboard rendering

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — environment verification only
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **User Stories (Phases 3-7)**: All depend on Foundational phase completion
  - US1 (Phase 3): No dependencies on other user stories
  - US2 (Phase 4): No dependencies on other user stories (modifies same files as US1 — merge-friendly)
  - US3 (Phase 5): No dependencies on other user stories (modifies same files as US1/US2 — merge-friendly)
  - US4 (Phase 6): No dependencies on other user stories (modifies same files as US1-US3 — merge-friendly)
  - US5 (Phase 7): No strict dependency on US2-US4 (can render null), but value depends on data from those stories
- **Polish (Phase 8)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: Can start after Foundational — completely independent
- **US2 (P2)**: Can start after Foundational — independently testable via DB queries
- **US3 (P2)**: Can start after Foundational — independently testable via DB queries
- **US4 (P3)**: Can start after Foundational — independently testable via DB queries
- **US5 (P3)**: Can start after Foundational — frontend components work with null data, full value after US2-US4

### Backend File Conflict Note

US1-US4 all modify the same files (`djinni.go`, `enrichment/handler.go`). When working sequentially, each phase builds on the previous one in the same files. The tasks are ordered to minimize merge conflicts:
1. US1 adds `Company` field → to struct, FetchDetail, handler
2. US2 appends `ExperienceLevel`, `ExperienceMinYears` → same struct, FetchDetail, handler  
3. US3 appends `SalaryEstimate*` fields → same struct, FetchDetail, handler
4. US4 appends `EnglishLevel` → same struct, FetchDetail, handler

### Within Each User Story

- Backend stories (US1-US4): Struct update → extraction logic (FetchDetail) → handler update → tests
- Frontend story (US5): All components [P] can be created in parallel → integrate into JobDetailPage

### Parallel Opportunities

- T032, T033, T034 (US5 dashboard components) can all run in parallel — they are separate files
- T009, T010, T011 (Phase 2 domain/model/DTO) can run in parallel — separate Go structs
- T013, T014 (Phase 2 shared TS) can run in parallel
- T040, T041 (Phase 8 build verification) can run in parallel
- US5 can begin in parallel with US2-US4 since dashboard components handle null gracefully

---

## Parallel Example: User Story 5

```bash
# Launch all dashboard components together:
Task: "Create ExperienceBadge component in apps/dashboard/src/features/job-detail/components/ExperienceBadge.tsx"
Task: "Create EnglishLevelBadge component in apps/dashboard/src/features/job-detail/components/EnglishLevelBadge.tsx"
Task: "Create SalaryEstimateCard component in apps/dashboard/src/features/job-detail/components/SalaryEstimateCard.tsx"
```

## Parallel Example: Phase 2 Core Typed Contracts

```bash
# Launch Go struct updates together:
Task: "Add new fields to domain model Job struct in apps/api/internal/domain/job.go"
Task: "Add new fields to NormalizedJob in apps/api/internal/dto/dto.go"
Task: "Add new fields to JobDto in apps/api/internal/dto/dto.go"

# Launch shared TS updates together:
Task: "Add new fields to NormalizedJob in packages/shared/src/index.ts"
Task: "Add new fields to JobDto in packages/shared/src/index.ts"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1 (company name fix)
4. **STOP and VALIDATE**: Re-scrape the example job, verify "Novacore" shows in dashboard
5. Deploy if ready — company fix alone is user-visible improvement

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. Add US1 → Test independently → Company names are correct (MVP!)
3. Add US2 → Test independently → Experience levels show in DB
4. Add US3 → Test independently → Salary estimates in DB
5. Add US4 → Test independently → English levels in DB
6. Add US5 → Test independently → All fields visible on dashboard
7. Polish → `make test-lint` passes, quickstart validated

### Quickest Path to User Value

Phases 1-3 (company fix) can be delivered independently. Phases 4-6 store data in DB that the dashboard can consume. Phase 7 makes it visible.

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Backend stories (US1-US4) modify same files sequentially — each builds on previous
- US5 (dashboard) can run in parallel with US2-US4 since components handle null data
- Commit after each phase or logical group
- Follow constitution Principle III: keep all 8 typed-contract layers in sync
- Existing `ParseSalaryRaw()` in `apps/api/internal/salary/parser.go` is reused — no new salary parsing needed
