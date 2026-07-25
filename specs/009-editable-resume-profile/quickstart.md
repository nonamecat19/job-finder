# Quickstart: Validate Fully Editable Resume Profile Tab

## Prerequisites

- Stack running via `make up` (or `make dev`), Postgres/Redis available via Docker Compose.
- `pnpm install` then `pnpm --filter @job-finder/shared build` run at least once (dashboard
  and tygo-generated types depend on `packages/shared` being built first, per repo convention).
- After backend DTO changes: `cd apps/api && tygo generate` (or `make tygo-generate`), then
  rebuild `packages/shared` again so the dashboard picks up the new `Resume` types. Verify
  with `make tygo-check` that generated types aren't stale.

## Scenario 1 — Build a resume from scratch (User Story 1, P1)

1. Create a new profile with no config (`POST /profiles` with only a name, no
   `rendercvYaml`), or open an existing profile with `hasConfig: false`.
2. Open the Profile tab in the dashboard — expect an empty-but-editable resume view, not
   an error state (validates FR-012, SC-001 setup condition).
3. Add one entry to each of the 9 entry types across new sections (education, experience,
   normal/project, publication, one_line/skill, bullet, numbered, reversed_numbered, text).
4. Save. Confirm via `GET /profiles/{id}/resume` that all 9 entries round-trip with the
   fields entered (validates FR-003, FR-004, data-model.md Entry shape).
5. Reload the page. Confirm all entries and their section order persisted (validates FR-006, SC-004).

## Scenario 2 — Import a config, then edit without re-uploading (User Story 2, P2)

1. Using `apps/api/internal/generation/testdata/sample_rendercv.yaml` as a known-good
   fixture, upload it via the existing config upload flow to a fresh profile.
2. Confirm every section/entry from the fixture appears correctly pre-filled and typed in
   the structured UI (validates FR-002, SC-002 zero-data-loss check — cross-reference
   fixture's 9 sections/types against the rendered forms).
3. Edit one field directly (e.g. change `experience[0].position`) and save via
   `PUT /profiles/{id}/resume` — no second file upload.
4. Confirm the change persisted and the rest of the fixture's data is untouched.
5. Attempt to re-upload a *different* config to the same (now-edited) profile. Confirm the
   client shows a replace-confirmation before calling `POST /profiles/config` (validates FR-010).

## Scenario 3 — Structural management: reorder, delete, custom sections (User Story 3, P2)

1. On a profile with 3+ sections and 3+ entries in at least one section, reorder entries
   within a section (drag or up/down buttons) and save; reload and confirm new order
   persisted (validates FR-006).
2. Reorder the sections themselves; reload and confirm new order persisted.
3. Delete a single entry — confirm a confirmation prompt appears before removal (FR-011),
   and the entry is gone after confirming.
4. Delete an entire section — same confirmation requirement, section and its entries removed.
5. Add a brand-new custom-named section (not one of the common names) and pick an entry
   type for it; add one entry; save and reload to confirm it persisted (validates FR-005).
6. Delete every section and every identity field, leaving a fully empty resume; confirm the
   UI still renders cleanly with no error (validates FR-012).

## Edge case checks

- Upload a config missing `cv.name` or with malformed YAML — confirm a specific error is
  shown and no existing data is altered.
- Upload a config containing a section/entry field the mapping layer doesn't recognize —
  confirm it's preserved and visible via the fallback editor, not dropped (FR-009).
- Attempt to save an entry with an end date before its start date — confirm inline
  validation blocks save and points at that field (FR-007).

## Automated coverage (see tasks.md for breakdown)

- `apps/api`: `go test ./internal/generation/... ./internal/profile/... ./internal/httpapi/...`
  covering `resume_mapping.go` (structured ⇄ map round-trip, order preservation,
  unrecognized-data retention) and the new `/profiles/{id}/resume` handlers.
- `apps/dashboard`: `vitest run` covering each entry form, section list reordering, and the
  confirm-dialog gating for delete/reupload.
- `make test-lint` before merge (both suites), per Constitution Principle IV.
