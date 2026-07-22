# Plan: 012 Outreach Generator

**Status**: Not started. Only the spec document exists. **Blocked on 004 and 007** (needs company intel signals for grounding and resolved contacts for addressing).

**Spec**: `specs/012-outreach-generator/spec.md`

## What Exists

| Layer | Status |
|-------|--------|
| Spec document | Done |
| Go package | **Missing** |
| Dashboard | **Missing** |
| Shared types | **Missing** |

## Implementation Plan

### 1. Backend

- [ ] **1.1** `apps/api/internal/outreach/types.go`:
  - `OutreachDraft` — message text, addressed contact, tone, grounding traces
  - `GroundingTrace` — claim → company-intel signal mapping
  - `Tone` enum — warm, direct, formal
- [ ] **1.2** `apps/api/internal/outreach/service.go`:
  - `GenerateDraft(ctx, jobID, contactID, tone)` — builds prompt with:
    - Contact name/title from plan 007
    - Company intel signals from plan 004
    - Selected tone instruction
    - Hard constraint: every specific claim must trace to a signal
    - Hard constraint: no send path exists
  - `ValidateDraft(draft)` — strips ungrounded claims (FR-006)
  - Length enforcement (FR-009)
- [ ] **1.3** `apps/api/internal/outreach/service_test.go`:
  - Draft addresses real contact
  - All claims trace to signals
  - No signals → generic opener (no fabrication)
  - Tone changes wording but not facts
  - Over-length draft rejected
- [ ] **1.4** HTTP endpoints:
  - `POST /api/jobs/{id}/outreach/generate` — body: `{contactId, tone}`, returns `OutreachDraft`
  - `GET /api/jobs/{id}/outreach/tones` — returns available tones
- [ ] **1.5** Register routes, wire service in main.go

### 2. Shared types

- [ ] **2.1** Add `OutreachDraftDto` to DTO layer
- [ ] **2.2** Regenerate TS types via tygo

### 3. Dashboard

- [ ] **3.1** `apps/dashboard/src/features/job-detail/OutreachPanel.tsx`:
  - Contact selector (when multiple contacts exist)
  - Tone selector (warm/direct/formal, default warm)
  - Generate button
  - Draft display (editable textarea)
  - Copy button (only action — no send)
  - Regenerate button
  - Grounding trace expandable (which signal backs each claim)
- [ ] **3.2** `useOutreachDraft` hook
- [ ] **3.3** API client in `lib/api.ts`
- [ ] **3.4** Render `<OutreachPanel>` on `JobDetailPage.tsx` (post-apply section)
- [ ] **3.5** Vitest coverage: all states, no send button exists

### 4. Hard product line verification

- [ ] **4.1** Grep for any SMTP, mailto:, form.submit(), or send path — must be zero
- [ ] **4.2** Confirm only actions are copy and regenerate
- [ ] **4.3** Confirm no job status change on draft generation

### 5. Verify

- [ ] **5.1** `go test ./internal/outreach/...` passes
- [ ] **5.2** `make test-lint` passes
- [ ] **5.3** End-to-end: job with contact + intel → generate draft → copy

## Dependencies
- **Plan 004** — Company intel signals (grounding source)
- **Plan 007** — Resolved contacts (addressee source)
- Existing local LLM runtime
- Existing job detail page
