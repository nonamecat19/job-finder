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
