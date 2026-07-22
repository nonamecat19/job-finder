# Plan: 009 Fit-Gap Coach — HTTP Wiring + Dashboard UI

**Status**: Backend core done, not wired. The `internal/coach/` package is fully implemented but has no HTTP handler and no dashboard UI.

## What Exists

| Layer | Status |
|-------|--------|
| Go `internal/coach/service.go` (349 lines) | Done — FitGapAssessment, GapItem, EvidenceItem, adjacency lookup, grounded rephrase |
| Go `internal/coach/types.go` | Done |
| Go `internal/coach/service_test.go` | Done |
| Go HTTP handler | **Missing** |
| Dashboard coach panel | **Missing** |
| Shared types | **Missing** |

## Tasks

### 1. Wire HTTP endpoints

- [ ] **1.1** Create `apps/api/internal/httpapi/coach.go` with:
  - `POST /api/jobs/{id}/coach/assess` — Run fit-gap assessment, return `FitGapAssessment`
  - `GET /api/jobs/{id}/coach/assessment` — Return cached assessment if exists
- [ ] **1.2** Register routes in `apps/api/internal/httpapi/router.go`
- [ ] **1.3** Wire coach service in `apps/api/cmd/server/main.go`

### 2. Expose types

- [ ] **2.1** Add `FitGapAssessmentDto` to `apps/api/internal/dto/dto.go`
- [ ] **2.2** Regenerate shared TS types via tygo

### 3. Dashboard coach panel

- [ ] **3.1** Create `apps/dashboard/src/features/job-detail/CoachPanel.tsx`
  - Shows gap summary (must-have gaps, nice-to-have gaps)
  - Shows adjacent evidence for each gap
  - Shows grounded rephrase suggestions
  - Loading, error, and empty states
- [ ] **3.2** Add `useCoachAssessment` hook
- [ ] **3.3** Add coach API client to `lib/api.ts`
- [ ] **3.4** Render `<CoachPanel>` on `JobDetailPage.tsx`
- [ ] **3.5** Vitest coverage

### 4. Verify

- [ ] **4.1** `go test ./internal/coach/...` passes
- [ ] **4.2** `make test-lint` passes
- [ ] **4.3** End-to-end: open job detail → see fit-gap assessment

## Dependencies
- Existing `internal/coach/` package
- Existing job detail page
- Existing local LLM runtime
