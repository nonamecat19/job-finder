# Implementation Plan: Djinni Scraping Enhancement

**Branch**: `022-djinni-scraping-enhance` | **Date**: 2026-07-28 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/022-djinni-scraping-enhance/spec.md`

## Summary

Enhance the Djinni job scraper to extract four additional data points from job detail pages: company name (fixing "Unknown" regressions), required years of experience, English proficiency level, and salary analytics estimates. New fields flow through the full typed contract pipeline (DB → Go domain → DTO → shared TS types) and are rendered as metadata components on the job detail dashboard.

## Technical Context

**Language/Version**: Go 1.23+ (backend `apps/api`), TypeScript/React 18+ (dashboard `apps/dashboard`)

**Primary Dependencies**: goquery (HTML parsing), sqlc (typed DB access), goose (migrations), asynq (background job enrichment), TanStack Query (frontend data fetching), HeroUI (dashboard components)

**Storage**: PostgreSQL (migration 00031 adds columns to `Job` table)

**Testing**: `go test` (backend unit + integration), `vitest` (dashboard)

**Target Platform**: Linux/Docker (dev via docker-compose, prod via docker-compose.prod.yml)

**Project Type**: Web application (Go HTTP API + React/Vite SPA)

**Performance Goals**: No change to existing performance — scraping is async (asynq queue), dashboard renders new fields inline with existing data

**Constraints**: Follow existing code patterns (goquery scraping, enrichment handler switch-case, COALESCE-based DB update, tile-based dashboard layout). No new dependencies. Scraping must fail gracefully on individual fields.

**Scale/Scope**: Single source adapter (Djinni), ~6 new DB columns, ~4 new dashboard components, 1 migration file

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| **I. No Auto-Apply** | ✅ Pass | Feature is read-only scraping; no application submission involved |
| **II. Grounded Generation** | ✅ Pass | No LLM-generated content; fields are directly extracted from source HTML |
| **III. Typed Contracts** | ✅ Pass (with action) | New fields must be added to: DB migration → sqlc model (`models.go`), domain model (`job.go`), `NormalizedJob` adapter contract, `DjinniDetailPatch`, `UpdateJobDetail` SQL, `JobDto` DTO, tygo regenerated output, `packages/shared/src/index.ts` (hand-maintained), `packages/shared/src/generated.ts` (tygo auto). All must stay in sync per convention. |
| **IV. Test Discipline** | ✅ Pass (with action) | New regex patterns require unit tests in `djinni_test.go`; enrichment handler changes require handler test updates; dashboard components need vitest rendering tests. `make test-lint` must pass before merge. |
| **V. Local-First** | ✅ Pass | Scraping is external by design (constitution explicitly treats scraping sources as best-effort/unstable). No external AI APIs used. |

**Gate Result**: All principles pass. Actions noted for implementation.

## Project Structure

### Documentation (this feature)

```text
specs/022-djinni-scraping-enhance/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── api.md           # API response contract changes
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── db/
│   │   ├── migrations/00031_djinni_scraping_enhance.sql    # NEW: new columns on Job
│   │   └── queries/job.sql                                  # MODIFY: UpdateJobDetail
│   ├── jobsources/adapters/
│   │   ├── djinni.go                    # MODIFY: FetchDetail, DjinniDetailPatch, regex
│   │   └── djinni_test.go               # MODIFY: new test cases
│   ├── enrichment/handler.go            # MODIFY: enrichDjinni applies new fields
│   ├── domain/job.go                    # MODIFY: new domain fields
│   └── dto/dto.go                       # MODIFY: JobDto new fields
│
packages/shared/src/
├── index.ts                             # MODIFY: JobDto + NormalizedJob new fields
└── generated.ts                         # REGENERATE: via tygo

apps/dashboard/src/features/job-detail/
├── JobDetailPage.tsx                    # MODIFY: new metadata components
├── hooks.ts                             # MODIFY: query picks up new fields
└── components/
    ├── ExperienceBadge.tsx              # NEW
    ├── EnglishLevelBadge.tsx            # NEW
    └── SalaryEstimateCard.tsx           # NEW
```

**Structure Decision**: Standard multi-app layout. Backend changes flow through the existing adapter → enrichment handler → DB pipeline. Frontend follows existing `JobDetailPage` tile-based component pattern.

## Complexity Tracking

> No constitution violations to justify.
