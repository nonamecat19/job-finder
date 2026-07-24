-- name: ListSavedSearches :many
SELECT * FROM "SavedSearch" ORDER BY "createdAt" ASC;

-- name: ListEnabledSavedSearches :many
SELECT * FROM "SavedSearch" WHERE "enabled" = true;

-- name: GetSavedSearch :one
SELECT * FROM "SavedSearch" WHERE "id" = $1;

-- name: CreateSavedSearch :one
INSERT INTO "SavedSearch" ("name", "query", "cron", "enabled")
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateSavedSearch :one
UPDATE "SavedSearch" SET
  "name" = COALESCE(sqlc.narg('name'), "name"),
  "query" = COALESCE(sqlc.narg('query'), "query"),
  "cron" = COALESCE(sqlc.narg('cron'), "cron"),
  "enabled" = COALESCE(sqlc.narg('enabled'), "enabled")
WHERE "id" = sqlc.arg('id')
RETURNING *;

-- name: DeleteSavedSearch :exec
DELETE FROM "SavedSearch" WHERE "id" = $1;

-- name: TouchSavedSearchLastRun :exec
UPDATE "SavedSearch" SET "lastRunAt" = now() WHERE "id" = $1;

-- name: ClaimSavedSearchRun :one
-- Compare-and-swap on "lastRunAt", used by the scheduler to claim a due
-- search before enqueueing anything for it. Two schedulers racing on the same
-- due search both pass the same expected value; row-level locking serializes
-- the UPDATEs, so the second one matches no row and returns pgx.ErrNoRows
-- instead of double-scraping every source.
--
-- "IS NOT DISTINCT FROM" rather than "=" because a never-run search has a
-- NULL "lastRunAt", and "NULL = NULL" is NULL, not true.
UPDATE "SavedSearch" SET "lastRunAt" = now()
WHERE "id" = sqlc.arg('id')
  AND "lastRunAt" IS NOT DISTINCT FROM sqlc.narg('expected_last_run_at')::timestamp
RETURNING "id";
