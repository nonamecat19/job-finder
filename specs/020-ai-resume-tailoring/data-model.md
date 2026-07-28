# Data Model — Feature 020: Constrained AI Resume Tailoring

## Existing entities reused (no schema change)

- **`profiles`** — `id uuid pk`, `name`, `document jsonb`, `rendercv_config jsonb`, `rendercv_yaml text`, `extra_notes`, `embedding vector`, `embed_model`, timestamps. Master resume lives in `rendercv_config` (parsed `RendercvMaster = map[string]any`). Read-only from tailoring's PoV.
- **`jobs`** — target job posting (or ad-hoc vacancy snapshot stored on `generated_documents` for ad-hoc runs). Read-only.
- **`generated_documents`** — re-used unmodified as the **PDF artifact store**: every successful `/export-pdf` writes a row with `job_id` (or `NULL` for ad-hoc), `type='resume'`, `content` = the *final accepted* `RendercvMaster` jsonb, `pdf_path`, `model`, timestamps. The dashboard retrieves via the existing `GET /api/documents/{id}/pdf`. No new columns.
- **`activities`** — one activity row per tailoring run (reuses the `generate` queue/activity recorder). No schema change.

## New tables

### `tailored_drafts`

A draft is a single review-lifecycle unit for tailoring a profile's resume against one job (or one ad-hoc vacancy). Re-runs create a new draft row seeded from the prior draft's `baseline` (research R7).

| Column | Type | Notes |
|--------|------|-------|
| `id` | `uuid` pk | default `gen_random_uuid()` |
| `profile_id` | `uuid` not null | fk `profiles(id)` on delete cascade |
| `job_id` | `uuid` null | fk `jobs(id)`; NULL when ad-hoc (paste-vacancy) — see `vacancy_*` |
| `vacancy_company` | `text` null | ad-hoc only |
| `vacancy_title` | `text` null | ad-hoc only |
| `vacancy_text` | `text` null | ad-hoc only |
| `baseline` | `jsonb not null` | the persisted `RendercvMaster` snapshot the proposal generator diffs against; updated in place on each acceptance |
| `baseline_content_hash` | `text not null` | sha256 of master `rendercv_config` at draft creation — detects drift if the user hand-edits the master mid-run |
| `state` | `text not null check state in ('drafting','proposing','review','finalized','expounded','abandoned')` | state machine (below) |
| `parent_draft_id` | `uuid` null | fk `tailored_drafts(id)`; set on a re-run draft, points to the prior draft it was seeded from |
| `model` | `text not null` | the LLM model that produced the proposals (audit; `llm.Router`-resolved) |
| `activity_id` | `uuid null` | the activity row tracking this run (for queue/progress UI) |
| `export_document_id` | `uuid null` | fk `generated_documents(id)`; set after a successful `/export-pdf` |
| `export_status` | `text null check export_status in ('pending','fitting','fit','blocked','error')` | null until first export attempt |
| `export_feedback` | `jsonb null` | populated only when `export_status='blocked'`: `{blocks: [{field, suggestion}]}` for the actionable message |
| `created_at` | `timestamptz not null default now()` | |
| `updated_at` | `timestamptz not null default now()` | trigger-maintained |

**Unique**: `unique (profile_id, coalesce(job_id, '00000000-0000-0000-0000-000000000000'))` — only one **active** draft per (profile, job); re-runs supersede by setting prior draft `state='abandoned'` before insert. (Ad-hoc drafts are similarly unique per `(profile_id, vacancy_company, vacancy_title)` when `job_id` is null, keyed on the ad-hoc identity.)

**State transitions** (`state`):
```
drafting  ──run enqueued──▶  proposing ──proposals persisted──▶  review
review    ──accept-all OR finalize button──▶  finalized
review    ──user abandons──▶  abandoned
finalized ──user re-runs──▶  (new sibling draft with parent_draft_id=this)
finalized ──export-pdf success──▶  finalized (export_document_id set)
```
`expounded` reserved for a future "expand this bullet" flow; not used in v1.

### `edit_proposals`

One row per atomic, reviewable change produced by the proposal generator (research R1/R3).

| Column | Type | Notes |
|--------|------|-------|
| `id` | `uuid` pk | |
| `draft_id` | `uuid not null` | fk `tailored_drafts(id)` on delete cascade |
| `field_type` | `text not null check field_type in ('summary','experience_highlights','skill_change','skill_group_add','skill_group_remove')` | partitions the review UI |
| `field_key` | `text not null` | disambiguator within field_type: `summary` → `"summary"`; `experience_highlights` → `"<company>::<bullet-index>"`; `skill_change` → `"<group-label>::<skill-token>"`; `skill_group_add` → `"<new-group-label>"`; `skill_group_remove` → `"<existing-group-label>"` |
| `before_value` | `text not null` | the baseline value at proposal time (string; for highlights this is the bullet text; for skill group add it's `""`) |
| `after_value` | `text not null` | the proposed value (string; for skill-group-remove it's `""`) |
| `traceability` | `jsonb not null` | `{source: "master"|"job_posting", path: e.g. "cv.sections.experience[Acme].highlights[2]" \| "job.required_skills"}` — required by FR-003/SC-005 audit |
| `status` | `text not null default 'pending' check status in ('pending','accepted','rejected','dropped')` | `dropped` = suppressed by grounding (research R8); never surfaced |
| `dropped_reason` | `text null` | set when `status='dropped'` — `"grounding_violation"` etc. |
| `accepted_at` | `timestamptz null` | audit |
| `rejected_at` | `timestamptz null` | audit |
| `created_at` | `timestamptz not null default now()` | |

**Index**: `create index edit_proposals_draft_status_idx on edit_proposals (draft_id, status)` — the dashboard pulls `pending` proposals for the review surface; the export path verifies no `pending` remain before issuing a PDF.

**Constraints / validation**:
- A draft in `state='finalized'` must have zero `edit_proposals` in `status='pending'` — enforced by the finalize service method (not a hard FK constraint, to keep accept/reject flexible).
- Field-level proposal shape invariants live in the Go `tailoring.proposals` package (per-field validators), not in DB CHECKs, so they keep step with the LLM schema.

## DTO shapes (Go `internal/dto/tailoring.go`, mirrored in `packages/shared/src/index.ts`)

```go
type EditProposalDto struct {
    ID           string             `json:"id"`
    DraftID      string             `json:"draftId"`
    FieldType    string             `json:"fieldType"`    // summary|experience_highlights|skill_change|skill_group_add|skill_group_remove
    FieldKey     string             `json:"fieldKey"`
    BeforeValue  string             `json:"beforeValue"`
    AfterValue   string             `json:"afterValue"`
    Traceability TraceabilityDto    `json:"traceability"`
    Status       string             `json:"status"`      // pending|accepted|rejected|dropped
    DroppedReason *string           `json:"droppedReason,omitempty"`
    AcceptedAt   *string            `json:"acceptedAt,omitempty"`
    RejectedAt   *string            `json:"rejectedAt,omitempty"`
}
type TraceabilityDto struct {
    Source string `json:"source"` // master|job_posting
    Path   string `json:"path"`
}
type TailoredDraftDto struct {
    ID                string             `json:"id"`
    ProfileID         string             `json:"profileId"`
    JobID             *string            `json:"jobId,omitempty"`
    VacancyCompany    *string            `json:"vacancyCompany,omitempty"`
    VacancyTitle       *string            `json:"vacancyTitle,omitempty"`
    State             string             `json:"state"`
    ParentDraftID     *string            `json:"parentDraftId,omitempty"`
    Model             string             `json:"model"`
    ActivityID        *string            `json:"activityId,omitempty"`
    ExportStatus      *string            `json:"exportStatus,omitempty"`
    ExportFeedback    []ExportBlockDto   `json:"exportFeedback,omitempty"`
    ExportDocumentID  *string            `json:"exportDocumentId,omitempty"`
    BaselineSummary   BaselineSummaryDto `json:"baselineSummary"`  // namings + skill-group labels + company list — NOT the full baseline blob
    Proposals         []EditProposalDto  `json:"proposals"`         // pending + rejected + accepted (UI groups)
    CreatedAt         string             `json:"createdAt"`
    UpdatedAt         string             `json:"updatedAt"`
}
type BaselineSummaryDto struct {
    ProfileName  string   `json:"profileName"`
    SkillGroups  []string  `json:"skillGroups"`
    Companies    []string  `json:"companies"`
}
type ExportBlockDto struct {
    Field      string `json:"field"`      // "experience:Acme:3" | "skill_group:Cloud" | "summary"
    Suggestion string `json:"suggestion"` // "shorten or drop this bullet to fit on one page"
}
type TailorResumeRequestDto struct {
    ProfileID string  `json:"profileId"`
    JobID     *string `json:"jobId,omitempty"`
    Vacancy   *AdhocVacancyDto `json:"vacancy,omitempty"` // for ad-hoc
}
type AdhocVacancyDto struct {
    Company string `json:"company"`
    Title   string `json:"title"`
    Text    string `json:"text"`
}
type ExportPdfRequestDto struct {
    DraftID string `json:"draftId"`
}
```

`baselineSummary` is a *projection* of `tailored_drafts.baseline` to avoid shipping the full master config to the client on every poll; the full baseline only leaves the server on `/export-pdf` (server-side rendering).

## sqlc queries (`internal/db/queries/tailoring.sql`)

Sketch (final names validated by `make sqlc-check`):
- `CreateDraft`, `GetDraft`, `GetDraftForUpdate` (FOR UPDATE), `ListDraftsByProfileJob`, `AbandonDraft`, `SetDraftState`, `UpdateDraftBaseline`, `SetDraftExport`, `SetDraftExportStatus`
- `CreateProposals` (batch insert), `ListProposalsByDraft`, `ListPendingProposals`, `SetProposalStatus` (accept/reject)

## Migration migration-numbering note

Constitution: goose version numbers MUST be unique and sequential. The largest migration in `apps/api/internal/db/migrations/` today is `00023_ai_feature_setting.sql`. Audit at write-time; the new file will be `00030_tailoring_drafts.sql` unless intervening migrations land in the meantime — verify against `ls apps/api/internal/db/migrations/`, pick the next free integer, never reuse.