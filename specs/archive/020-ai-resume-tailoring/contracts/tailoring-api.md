# Contract: Tailoring REST API

Surface area for the dashboard's tailoring review flow. All paths mount under `/api` (the existing `httpapi.NewRouter` base path). Auth, CORS, requestId middleware are inherited unchanged from `httpapi.NewRouter`.

## Routes (new `internal/httpapi/tailoring.go` `TailoringHandler.Mount`)

| Method | Path | Purpose | Spec ref |
|--------|------|---------|----------|
| POST   | `/api/tailoring`                         | Enqueue a new tailoring run (creates draft, enqueues `generate` job, returns draft + activity id) | US1, FR-004, FR-013 |
| GET    | `/api/tailoring/{draftId}`               | Fetch a draft: state, baseline summary, all proposals grouped by status | FR-011, FR-010 |
| POST   | `/api/tailoring/{draftId}/proposals/{proposalId}` | Accept or reject one proposal; body `{action:"accept"\|"reject"}` | FR-004/005/006 |
| POST   | `/api/tailoring/{draftId}/finalize`      | Mark the draft `finalized` (validates no `pending` proposals remain) | US2 prereq |
| POST   | `/api/tailoring/{draftId}/export-pdf`    | Start single-page fitter; returns `{documentId?:, status: "pending"|"fit"|"blocked", feedback?: ExportBlock[]}` synchronous (fit ≤ 5s) or short-poll via `/export-status` | FR-007, FR-008, SC-002/003/006 |
| GET    | `/api/tailoring/{draftId}/export-status`  | Short-poll the in-progress fitter (idempotent) | FR-013 |
| DELETE | `/api/tailoring/{draftId}`                | Abandon the draft (`state='abandoned'`) | cleanup |
| POST   | `/api/tailoring/{draftId}/rerun`          | Create a sibling draft seeded from the current draft's baseline, enqueue a new run | FR-010, clarification Q4 |

## Request / response shapes

All request/response bodies are JSON. Error responses use the existing `{error: string, path?: string, message?: string}` shape already returned by `httpapi` (see `profiles.go` validation-error formatting). Dates are ISO-8601 UTC strings.

### POST /api/tailoring

**Request body** — `TailorResumeRequestDto`:
```json
{ "profileId": "<uuid>",
  "jobId": "<uuid>",
  "vacancy": null }
```
or for ad-hoc:
```json
{ "profileId": "<uuid>",
  "jobId": null,
  "vacancy": { "company": "Acme", "title": "Staff Engineer", "text": "..." } }
```

**Response 202** — `{draftId: <uuid>, activityId: <uuid>}`.
**Response 400** — invalid profile/job id, profile has no master content, or a draft is already `review`/`finalized` for the same `(profile, job)` (caller must `/rerun` instead).
**Response 409** — profile's master `rendercv_config` checksum mismatch with an existing draft (master was edited mid-review); caller should abandon and start fresh.

### GET /api/tailoring/{draftId}

**Response 200** — `TailoredDraftDto` (see `data-model.md` DTOs). `proposals` is the full list (UI groups by status); `dropped` proposals are not included.
**Response 404** — draft not found or not owned by the caller (single-user repo: always owned).

### POST /api/tailoring/{draftId}/proposals/{proposalId}

**Request body** — `{action: "accept" | "reject"}`.

**Response 200** — the updated `EditProposalDto`. Accepting mutates `tailored_drafts.baseline` in the same tx so subsequent polls reflect the new baseline. Rejecting restores the baseline value for that field (no baseline change).
**Response 409** — proposal already in a terminal state, or draft is `finalized`/`abandoned`.
**Response 422** — proposal `status='dropped'` (grounding-suppressed); cannot be accepted.

### POST /api/tailoring/{draftId}/finalize

**Response 200** — `{state: "finalized"}`. Pre-condition: zero `pending` proposals. The handler does **not** auto-finalize; the user must click.

### POST /api/tailoring/{draftId}/export-pdf

**Request body** — `{}` (draft id is in the path). Pre-condition: draft `state='finalized'`.

**Response 200** — synchronous fit:
```json
{ "status": "fit",
  "documentId": "<uuid>",
  "exportStatus": "fit" }
```
or **blocked** (fit attempt exhausted density ladder):
```json
{ "status": "blocked",
  "exportStatus": "blocked",
  "feedback": [
    {"field": "experience:Acme:3", "suggestion": "shorten or drop this bullet to fit on one page"},
    {"field": "skill_group:Cloud", "suggestion": "consider removing this skill group to gain ~40mm"}
  ] }
```
or **in-progress** when the fitter exceeds a 5s synchronous budget:
```json
{ "status": "pending", "exportStatus": "pending" }
```
Client polls `/export-status` until `fit` or `blocked`.

**Response 409** — draft not `finalized`, or earlier export already `fit` (caller fetches existing `documentId`), or draft has `pending` proposals.

### GET /api/tailoring/{draftId}/export-status

**Response 200** — `{exportStatus: "pending"|"fitting"|"fit"|"blocked"|"error", documentId?, feedback?}`. Idempotent.

### POST /api/tailoring/{draftId}/rerun

**Response 202** — `{draftId: <new uuid>, activityId: <uuid>}`. The prior draft is left `finalized`; the new draft's `baseline` is seeded from the prior's current `baseline` (research R7) — already-accepted edits are NOT re-surfaced as new proposals (clarification Q4).

### DELETE /api/tailoring/{draftId}

**Response 204** — draft set `abandoned`. Pending proposals kept for audit but no longer surfaced in the active review.

## Concurrency & idempotency

- All write endpoints take a row-level `SELECT ... FOR UPDATE` on the draft before mutating (`GetDraftForUpdate`). Accept/reject is idempotent on terminal-state rows (caller may retry on network flakiness without double-applying — returns 409 if already terminal).
- `POST /api/tailoring` for the same `(profileId, jobId)` while an active draft exists returns 400 — caller must use `/rerun` instead.
- `/export-pdf` is idempotent: a draft that already produced a `fit` returns the cached `documentId`; a draft that was `blocked` may be retried only after the user accepts a `skill_group_remove`/`skill_change` proposal that frees space.

## Activity / worker wiring (reused, not new contract)

- `POST /api/tailoring` enqueues an asynq `TypeGenerate` task with an extended `GeneratePayload` carrying the new `tailoringDraftID uuid` field (wire-nullable so existing callers ignore it). The existing `generation.Handler.ProcessTask` detects this field and dispatches to `tailoring.Service.RunProposals` instead of the merged-resume path.
- The activity recorder writes one activity row per run; the existing `GET /api/activity` routes already expose queued/running state and progress, and the dashboard's existing activity polling will surface the indeterminate progress indicator required by FR-013 without a new endpoint.

## Backwards compatibility

- `GeneratePayload` gains one `*uuid.UUID` field; old callers leave it nil and the merged-resume path is unchanged.
- `generated_documents` rows produced by `POST /export-pdf` look identical to rows produced by the legacy path — the existing `GET /api/documents` listing and `GET /api/documents/{id}/pdf` download work unchanged.
- The legacy `POST /api/documents/tailor` ad-hoc merged-resume endpoint is NOT removed by this feature; it remains for any non-review-GUI consumer. Spec 020's flow is additive.