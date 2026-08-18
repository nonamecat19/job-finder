-- name: CreateRun :one
INSERT INTO generation_runs (
    profile_id, job_id, vacancy_company, vacancy_title, vacancy_text,
    master_snapshot, master_content_hash, shape_config, grounding_level, summary_option_id
) VALUES (
    $1, sqlc.narg('job_id'), sqlc.narg('vacancy_company'), sqlc.narg('vacancy_title'), $2,
    $3, $4, $5, $6, sqlc.narg('summary_option_id')
)
RETURNING *;

-- name: GetRun :one
SELECT * FROM generation_runs WHERE id = $1;

-- name: GetRunForUpdate :one
-- Row lock for every mutation on this run (item toggle/edit/reorder, rerun,
-- export): these read the current state and write it back, so concurrent
-- requests against the same run must serialize (rest-api.md's "every write
-- takes a row-level SELECT ... FOR UPDATE on the run first").
SELECT * FROM generation_runs WHERE id = $1 FOR UPDATE;

-- name: ListRunsByProfile :many
-- jobId is optional: NULL matches every run for the profile, matching the
-- `GET /v1/generations?jobId=&limit=` contract (rest-api.md).
SELECT * FROM generation_runs
WHERE profile_id = $1
  AND (sqlc.narg('job_id')::uuid IS NULL OR job_id = sqlc.narg('job_id'))
ORDER BY created_at DESC
LIMIT $2;

-- name: SetRunState :exec
UPDATE generation_runs SET state = $2, updated_at = now() WHERE id = $1;

-- name: SetRunAnalysis :exec
-- Persists the VacancyAnalysis once at run start so a per-section rerun
-- (FR-021) does not re-analyse (data-model.md §1).
UPDATE generation_runs SET analysis = $2, updated_at = now() WHERE id = $1;

-- name: SetRunExport :exec
-- Covers every export state transition: rendering (both NULL), exported
-- (document_id set, report NULL) and blocked (document_id NULL, report set).
UPDATE generation_runs SET
    export_document_id = sqlc.narg('export_document_id'),
    export_status = $2,
    export_report = sqlc.narg('export_report'),
    updated_at = now()
WHERE id = $1;

-- name: DeleteRun :exec
DELETE FROM generation_runs WHERE id = $1;

-- name: CreateSections :many
-- Batch insert for one run's sections (one summary, one per master
-- experience entry, one skills). Arrays are position-aligned and zipped via
-- parallel unnest() calls in a subquery select list rather than the
-- multi-argument unnest(a, b, ...) table form, which sqlc's analyzer cannot
-- resolve (same pattern as job.sql's BulkInsertJobs and tailoring.sql's
-- CreateProposals); PostgreSQL evaluates them in lockstep either way.
-- entry_key/entry_label are passed as "" for summary/skills sections and
-- normalized to NULL here.
INSERT INTO generation_sections (run_id, kind, entry_key, entry_label, position, target_count)
SELECT sqlc.arg('run_id')::uuid, k, NULLIF(ek, ''), NULLIF(el, ''), p, tc
FROM (
    SELECT
        unnest(sqlc.arg('kinds')::text[]) AS k,
        unnest(sqlc.arg('entry_keys')::text[]) AS ek,
        unnest(sqlc.arg('entry_labels')::text[]) AS el,
        unnest(sqlc.arg('positions')::int[]) AS p,
        unnest(sqlc.arg('target_counts')::int[]) AS tc
) AS x
RETURNING *;

-- name: ListSectionsByRun :many
SELECT * FROM generation_sections WHERE run_id = $1 ORDER BY position;

-- name: UpdateSectionEnabled :one
-- The per-run "disable this section" switch (PATCH .../sections/{sectionId}):
-- excludes the section from export/preview via Assemble without touching its
-- items, so re-enabling it restores exactly the selection it had.
UPDATE generation_sections SET enabled = $2 WHERE id = $1 RETURNING *;

-- name: SetSectionState :exec
UPDATE generation_sections SET
    state = $2,
    error = sqlc.narg('error'),
    fallback_used = $3
WHERE id = $1;

-- name: DeleteSectionItems :exec
-- Used by a section rerun (FR-021): the section's items are deleted and
-- recreated, with the caller re-applying matched selections/edits/positions
-- from the deleted rows before this call (data-model.md §4).
DELETE FROM generation_items WHERE section_id = $1;

-- name: CreateItems :many
-- Batch insert for one section's items. Same zipped-unnest pattern as
-- CreateSections. source_index is passed as -1 for origin='ai' rows (which
-- have no source index) and normalized to NULL here; edited_text is passed
-- as "" for rows with no edit and normalized to NULL here.
INSERT INTO generation_items (section_id, origin, source_index, source_text, edited_text, rank, position, selected)
SELECT sqlc.arg('section_id')::uuid, o, NULLIF(si, -1), st, NULLIF(et, ''), r, p, sel
FROM (
    SELECT
        unnest(sqlc.arg('origins')::text[]) AS o,
        unnest(sqlc.arg('source_indexes')::int[]) AS si,
        unnest(sqlc.arg('source_texts')::text[]) AS st,
        unnest(sqlc.arg('edited_texts')::text[]) AS et,
        unnest(sqlc.arg('ranks')::int[]) AS r,
        unnest(sqlc.arg('positions')::int[]) AS p,
        unnest(sqlc.arg('selected_flags')::bool[]) AS sel
) AS x
RETURNING *;

-- name: ListItemsByRun :many
-- Joins to the run through the section so the handler can build the whole
-- workspace (GET /v1/generations/{runId}) with one query per table. Ordered
-- by section position then item position, matching the "items are returned
-- in position order" response rule (rest-api.md).
SELECT gi.* FROM generation_items gi
JOIN generation_sections gs ON gs.id = gi.section_id
WHERE gs.run_id = $1
ORDER BY gs.position, gi.position;

-- name: GetItemForUpdate :one
SELECT * FROM generation_items WHERE id = $1 FOR UPDATE;

-- name: UpdateItemSelection :exec
-- Covers the toggle and reorder halves of PATCH .../items/{itemId}. Both
-- fields are optional in the request ("any subset"), so each is left
-- unchanged when its narg is NULL.
UPDATE generation_items SET
    selected = COALESCE(sqlc.narg('selected'), selected),
    position = COALESCE(sqlc.narg('position'), position),
    updated_at = now()
WHERE id = $1;

-- name: UpdateItemText :exec
-- FR-009/FR-015 at the query boundary: this is only ever called for an
-- origin='ai' item, enforced by the CHECK constraint and re-checked by the
-- handler before calling it (403 for a profile-origin item).
UPDATE generation_items SET edited_text = $2, updated_at = now() WHERE id = $1;

-- name: UpdateItemDroppedEntries :exec
-- The per-skill half of PATCH .../items/{itemId}: the entries of a skill
-- group's `details` the user switched off, replaced wholesale so the write is
-- idempotent and order-free. Only ever called for a profile-origin item in a
-- skills section, checked by the handler.
UPDATE generation_items SET
    dropped_entries = sqlc.arg('dropped_entries')::text[],
    updated_at = now()
WHERE id = $1;

-- name: ReorderSectionItems :exec
-- Whole-section reorder in one call (PATCH .../sections/{sectionId}/order).
-- item_ids and positions are position-aligned arrays.
UPDATE generation_items AS gi SET
    position = x.p,
    updated_at = now()
FROM (
    SELECT
        unnest(sqlc.arg('item_ids')::uuid[]) AS id,
        unnest(sqlc.arg('positions')::int[]) AS p
) AS x
WHERE gi.id = x.id AND gi.section_id = sqlc.arg('section_id')::uuid;

-- name: MarkItemsUnavailable :exec
-- FR-022: called when the master changed and these items' source_index no
-- longer resolves against the current profile. Never deletes the row — an
-- unavailable item is still shown, per data-model.md §4.
UPDATE generation_items SET unavailable = true, updated_at = now()
WHERE id = ANY(sqlc.arg('item_ids')::uuid[]);
