# Ad-hoc CV + Cover Letter Tab

## Problem

Resume/cover-letter generation currently requires a `Job` row (scraped/ingested). There's already a synchronous ad-hoc resume endpoint (`POST /documents/tailor`, backed by `Service.GenerateRendercvFromText`) used by an opencode CLI command, but it has no cover letter, no persistence, and no dashboard UI.

Goal: let the user paste raw vacancy text into the dashboard and get both a tailored resume and a cover letter, without needing a scraped Job.

## Data model

Migration `00006_adhoc_documents.sql`:

```sql
-- +goose Up
ALTER TABLE "GeneratedDocument"
  ALTER COLUMN "jobId" DROP NOT NULL,
  ADD COLUMN "company" text,
  ADD COLUMN "title" text,
  ADD COLUMN "vacancy" text;

-- +goose Down
ALTER TABLE "GeneratedDocument"
  DROP COLUMN "company",
  DROP COLUMN "title",
  DROP COLUMN "vacancy",
  ALTER COLUMN "jobId" SET NOT NULL;
```

`company`/`title`/`vacancy` are populated only for ad-hoc rows (jobId NULL); NULL for job-tied rows. Needed to render a history list without a Job join.

## Queries (`document.sql`)

- `InsertGeneratedDocument`: add nullable `jobId`, plus `company`, `title`, `vacancy` params.
- `ListAdHocDocuments :many` — `SELECT * FROM "GeneratedDocument" WHERE "jobId" IS NULL ORDER BY "createdAt" DESC`.
- Existing `GetDocumentByID`, `UpdateDocumentContent` reused unchanged (already generic).

## Backend

`generation.Service`:
- `writeCoverLetter` signature changes from `(ctx, profileText, extraNotes, job sqlcgen.Job)` to `(ctx, profileText, extraNotes, company, title, vacancyText string)`. Job-tied caller (`Generate`) passes `job.Company, job.Title, job.Description`.
- Replace `GenerateRendercvFromText` with:
  ```go
  func (s *Service) GenerateAdHoc(ctx context.Context, in AdHocInput) (resume, coverLetter dto.GeneratedDocumentDto, err error)
  ```
  Runs `tailorRendercvResume` (existing) → renders resume PDF, then `writeCoverLetter` → renders cover letter PDF via `htmlRenderer.RenderCoverLetter`, inserts two `GeneratedDocument` rows with `jobId=NULL`, `company`/`title`/`vacancy` set, `version` always 1 (no versioning across ad-hoc runs — each run is independent).

`httpapi/documents.go`:
- `DocumentGenerator` interface: swap `GenerateRendercvFromText` for `GenerateAdHoc`, add `ListAdHocDocuments(ctx) ([]dto.GeneratedDocumentDto, error)`.
- `POST /documents/tailor`: same request body (vacancy/company/title/groundingLevel/hints), now returns `{resume: GeneratedDocumentDto, coverLetter: GeneratedDocumentDto}` instead of raw yaml/pdf paths.
- `GET /documents/ad-hoc`: returns list for history.
- `/documents/{id}`, `/documents/{id}/pdf`, `PUT /documents/{id}` unchanged, work for ad-hoc docs same as job-tied since they're id-keyed.

Synchronous request/response (no asynq queue, no activity tracking) — consistent with the existing tailor endpoint's pattern. Two LLM calls + two PDF renders back-to-back, so the request can take ~30-60s; acceptable for single-user local use.

## Dashboard

- `app/shell.tsx`: add nav item `{ to: '/tailor', label: 'Tailor', icon: FileEdit }`.
- `app/routes.tsx`: add `<Route path="/tailor" element={<TailorPage />} />`.
- `features/tailor/TailorPage.tsx`:
  - Form: vacancy textarea, company input, title input, grounding level select.
  - Generate button → `POST /documents/tailor`, disabled + spinner while pending (mutation, no polling needed since it's synchronous).
  - Result section: resume card (PDF link) + cover letter card (PDF link, editable text via existing `PUT /documents/{id}` + `RenderCoverLetter` re-render, same pattern as `JobDetailPage`).
  - History list below: `GET /documents/ad-hoc`, grouped by company/title/date, each row links to its resume/cover-letter PDFs.
- `packages/shared`/tygo-generated types: `GeneratedDocumentDto` gains `company?`, `title?`, `vacancy?` fields (already has `jobId`; make optional on TS side since backend now allows null — check tygo nullable mapping for `*string` jobId).
- New `api.documents.tailor(...)`, `api.documents.listAdHoc()` client methods + `queryKeys` entries.

## Testing

- Go: unit test `GenerateAdHoc` (mock LLM/profiles/renderers, assert both rows inserted with jobId NULL); handler test for `/documents/tailor` and `/documents/ad-hoc`.
- Dashboard: component test for `TailorPage` (form submit → shows results; history list renders).
