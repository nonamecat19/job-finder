-- name: CreateDraft :one
INSERT INTO tailored_drafts (
    profile_id, job_id, vacancy_company, vacancy_title, vacancy_text,
    baseline, baseline_content_hash, state, parent_draft_id, model, activity_id
) VALUES (
    $1, sqlc.narg('job_id'), sqlc.narg('vacancy_company'), sqlc.narg('vacancy_title'), sqlc.narg('vacancy_text'),
    $2, $3, $4, sqlc.narg('parent_draft_id'), $5, sqlc.narg('activity_id')
)
RETURNING *;

-- name: GetDraft :one
SELECT * FROM tailored_drafts WHERE id = $1;

-- name: GetDraftForUpdate :one
-- Row lock for the accept/reject/finalize/skill-edit mutation path: these
-- read the current baseline, mutate it, and write it back, so concurrent
-- requests against the same draft must serialize (research/data-model.md).
SELECT * FROM tailored_drafts WHERE id = $1 FOR UPDATE;

-- name: ListDraftsByProfileJob :many
-- Mirrors the unique(profile_id, coalesce(job_id, '0000...')) constraint on
-- tailored_drafts so the service can find the current active draft for a
-- (profile, job) pair, including the ad-hoc case where job_id is NULL.
SELECT * FROM tailored_drafts
WHERE profile_id = $1
  AND COALESCE(job_id, '00000000-0000-0000-0000-000000000000'::uuid)
    = COALESCE(sqlc.narg('job_id')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
ORDER BY created_at DESC;

-- name: AbandonDraft :exec
-- Used when a re-run supersedes the prior active draft (data-model.md
-- state transitions: review -> abandoned).
UPDATE tailored_drafts SET state = 'abandoned', updated_at = now() WHERE id = $1;

-- name: SetDraftState :exec
UPDATE tailored_drafts SET state = $2, updated_at = now() WHERE id = $1;

-- name: UpdateDraftBaseline :exec
-- Called on every proposal acceptance and every per-skill edit to an
-- existing group; baseline is the full RendercvMaster snapshot re-marshaled
-- by the caller.
UPDATE tailored_drafts SET baseline = $2, updated_at = now() WHERE id = $1;

-- name: SetDraftExport :exec
-- Called on a successful single-page export: records the produced
-- generated_documents row and marks the export fit; clears any stale
-- blocked-export feedback from a prior attempt.
UPDATE tailored_drafts SET
    export_document_id = $2,
    export_status = 'fit',
    export_feedback = NULL,
    updated_at = now()
WHERE id = $1;

-- name: SetDraftExportStatus :exec
-- Used for the pending/blocked/error export states, which don't produce a
-- document. feedback is only non-NULL when status = 'blocked'.
UPDATE tailored_drafts SET
    export_status = $2,
    export_feedback = sqlc.narg('feedback'),
    updated_at = now()
WHERE id = $1;

-- name: CreateProposals :many
-- Batch insert for one draft's proposals. Arrays are position-aligned and
-- zipped via parallel unnest() calls in a subquery select list rather than
-- the multi-argument unnest(a, b, ...) table form, which sqlc's analyzer
-- cannot resolve (same pattern as job.sql's BulkInsertJobs); PostgreSQL
-- evaluates them in lockstep either way. dropped_reason is passed as ""
-- for non-dropped rows and normalized to NULL here.
INSERT INTO edit_proposals (
    draft_id, field_type, field_key, before_value, after_value,
    traceability, status, dropped_reason
)
SELECT
    sqlc.arg('draft_id')::uuid, ft, fk, bv, av, tr, st, NULLIF(dr, '')
FROM (
    SELECT
        unnest(sqlc.arg('field_types')::text[]) AS ft,
        unnest(sqlc.arg('field_keys')::text[]) AS fk,
        unnest(sqlc.arg('before_values')::text[]) AS bv,
        unnest(sqlc.arg('after_values')::text[]) AS av,
        unnest(sqlc.arg('traceabilities')::jsonb[]) AS tr,
        unnest(sqlc.arg('statuses')::text[]) AS st,
        unnest(sqlc.arg('dropped_reasons')::text[]) AS dr
) AS x
RETURNING *;

-- name: ListProposalsByDraft :many
SELECT * FROM edit_proposals WHERE draft_id = $1 ORDER BY created_at;

-- name: ListPendingProposals :many
-- Backed by edit_proposals_draft_status_idx; the export path uses this to
-- verify zero pending proposals remain before issuing a PDF.
SELECT * FROM edit_proposals WHERE draft_id = $1 AND status = 'pending' ORDER BY created_at;

-- name: SetProposalStatus :exec
-- accepted_at/rejected_at are mutually exclusive: the caller passes now()
-- for exactly one (matching status) and NULL for the other.
UPDATE edit_proposals SET
    status = $2,
    accepted_at = sqlc.narg('accepted_at'),
    rejected_at = sqlc.narg('rejected_at')
WHERE id = $1;
