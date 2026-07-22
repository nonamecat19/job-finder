# Plan: 007 Recruiter / Hiring-Manager Resolution

**Status**: Not started. Only the SQL migration exists. **Blocked on 004 company intel backend** (needs `internal/companyintel` package for company-page fetch).

**Spec**: `specs/007-recruiter-resolution/spec.md`
**Tasks**: `specs/007-recruiter-resolution/tasks.md` (32 tasks, 6 phases)

## What Exists

| Layer | Status |
|-------|--------|
| SQL migration `00010_job_contact.sql` | Done — `JobContact` table |
| Go `internal/recruiter/` package | **Missing** |
| Go HTTP handler | **Missing** |
| Dashboard contact line/panel | **Missing** |
| Shared types | **Missing** |

## Implementation Plan

### Phase 1: Setup
- [ ] Confirm plan 004's `internal/companyintel` package has landed (gate check)
- [ ] Add `LinkedInScrapeEnabled` config field (default false)
- [ ] Capture company-page HTML fixtures

### Phase 2: Foundational
- [ ] Migration `00010_job_contact.sql` already exists — verify it's applied
- [ ] Add sqlc queries: `UpsertJobContact`, `ListJobContactsByJob`
- [ ] Update integration test truncateAll for JobContact
- [ ] Run `sqlc generate`

### Phase 3: US1 — Contact line on detail page (MVP)
- [ ] `apps/api/internal/recruiter/types.go` — `ResolvedContact` struct
- [ ] `apps/api/internal/recruiter/posting.go` — Posting-text source (LLM extraction from job description)
- [ ] `apps/api/internal/recruiter/resolve.go` — Orchestration (posting source only for MVP)
- [ ] `apps/api/internal/recruiter/posting_test.go` — Named contact, no contact, generic mailbox, Cyrillic, field traceability
- [ ] `GET /api/jobs/{id}/contacts` endpoint
- [ ] Contact line on `JobDetailPage.tsx` — headline contact or "No contact found — try Refresh"
- [ ] Vitest coverage for both states

### Phase 4: US2 — Refresh contacts
- [ ] `apps/api/internal/recruiter/companypage.go` — Company-page source (reuses plan 004 fetch)
- [ ] `apps/api/internal/recruiter/linkedin.go` — LinkedIn source (gated on env var)
- [ ] Extend `Resolve` to run all three sources with per-source error isolation
- [ ] `POST /api/jobs/{id}/contacts/refresh` endpoint
- [ ] Refresh button on detail page
- [ ] Tests: company page parse, LinkedIn skipped when disabled, one source fails, idempotent

### Phase 5: US3 — Expandable contact list
- [ ] Expandable list on detail page — all contacts with source + confidence
- [ ] Ordered best-first, deterministic tie-break
- [ ] Vitest coverage

### Phase 6: Polish
- [ ] Live smoke tests behind `//go:build live`
- [ ] Constitution Principle I audit (no send/message/apply)
- [ ] Constitution Principle II audit (no fabrication)
- [ ] `make test-lint` passes

## Dependencies
- **Plan 004 backend** — MUST land first (company-page fetch reuse)
- Existing job detail page
- Existing local LLM runtime (Ollama)
- Existing scraping service
