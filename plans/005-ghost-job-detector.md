# Plan: 005 Ghost-Job Detector

**Status**: Not started. Only the SQL migration exists.

**Spec**: `specs/005-ghost-job-detector/spec.md`
**Tasks**: `specs/005-ghost-job-detector/tasks.md` (45 tasks, 7 phases)

## What Exists

| Layer | Status |
|-------|--------|
| SQL migration `00008_job_signal.sql` | Done — `JobSignal` table |
| Go `internal/ghostjob/` package | **Missing** |
| Go HTTP handler | **Missing** |
| Dashboard ghost badge/panel | **Missing** |
| Shared types | **Missing** |

## Implementation Plan

### Phase 1: Schema (already done)
- Migration `00008_job_signal.sql` exists with `JobSignal` table, unique constraint, FK cascade, index

### Phase 2: Query surface
- [ ] Create `apps/api/internal/db/queries/jobsignal.sql` with:
  - `UpsertJobSignal` — INSERT ON CONFLICT upsert
  - `GetJobSignal` — single row by jobId + kind
  - `ListJobSignalsByJobIds` — batch query for feed
  - `CountRepostsByDedupeKey` — count appearances across ingestion runs
  - `CountCrossBoardDuplicates` — distinct sourceKeys in 60-day window
  - `CountAlwaysHiringByCompany` — unprogressed jobs from same company in 90 days
- [ ] Run `sqlc generate`

### Phase 3: Types and measurement
- [ ] `apps/api/internal/ghostjob/types.go` — `GhostJobResult`, `GhostSignals`, validation
- [ ] `apps/api/internal/ghostjob/ports.go` — Repository interface
- [ ] `apps/api/internal/ghostjob/simhash.go` — Description similarity hashing
- [ ] `apps/api/internal/ghostjob/signals.go` — Signal measurement from SQL queries
- [ ] `apps/api/internal/ghostjob/signals_test.go` — All edge cases (nil signals, unknown company, etc.)
- [ ] `apps/api/internal/ghostjob/simhash_test.go`

### Phase 4: US1 — Feed badge (MVP)
- [ ] `apps/api/internal/ghostjob/service.go` — Measure signals → LLM → upsert
- [ ] `apps/api/internal/ghostjob/service_test.go` — Fake LLM provider tests
- [ ] Add `JobSignalDto` to `apps/api/internal/dto/dto.go`
- [ ] Wire ghost signal into job list payload (batch query)
- [ ] Register ghost service in `main.go`, invoke after ingestion
- [ ] Regenerate shared TS types (tygo)
- [ ] Add `GhostBadge` component to dashboard
- [ ] Render `<GhostBadge>` next to `<ScoreBadge>` in feed

### Phase 5: US2 — Detail breakdown panel
- [ ] Expose full ghost signal on job-detail payload
- [ ] Add ghost breakdown panel to `JobDetailPage.tsx`
- [ ] Handle unknown signals, low confidence, never-scored states

### Phase 6: US3 — Manual re-score
- [ ] `POST /api/jobs/{id}/ghost-score` endpoint
- [ ] Refresh button on detail page

### Phase 7: Polish
- [ ] Live smoke test behind `//go:build live`
- [ ] `make test-lint` passes
- [ ] Constitution Principle I audit (no hide/filter/reorder)

## Dependencies
- Existing job deduplication identity
- Existing application status history
- Existing local LLM runtime (Ollama)
- Existing feed and job-detail screens
