-- name: InsertActivityRun :one
INSERT INTO "ActivityRun" ("op", "label", "jobId", "sourceKey", "queueTaskId", "meta")
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: StartActivityRun :exec
UPDATE "ActivityRun" SET "state" = 'running', "startedAt" = now() WHERE "id" = $1;

-- name: SetActivityStep :exec
UPDATE "ActivityRun" SET "step" = $2, "meta" = COALESCE(sqlc.narg('meta'), "meta") WHERE "id" = $1;

-- name: FinishActivityRunOk :exec
UPDATE "ActivityRun" SET "state" = 'succeeded', "refId" = $2, "meta" = COALESCE(sqlc.narg('meta'), "meta"), "finishedAt" = now() WHERE "id" = $1;

-- name: FinishActivityRunError :exec
UPDATE "ActivityRun" SET "state" = 'failed', "error" = $2, "finishedAt" = now() WHERE "id" = $1;

-- name: FinishActivityRunCancelled :exec
UPDATE "ActivityRun" SET "state" = 'cancelled', "error" = $2, "finishedAt" = now() WHERE "id" = $1;

-- name: GetActivityRun :one
SELECT * FROM "ActivityRun" WHERE "id" = $1;

-- name: ListActiveActivityRuns :many
SELECT * FROM "ActivityRun" WHERE "state" IN ('queued', 'running') ORDER BY "createdAt" DESC;

-- name: ListRecentActivityRuns :many
SELECT * FROM "ActivityRun" ORDER BY "createdAt" DESC LIMIT $1;

-- name: ListFailedActivityRuns :many
-- Includes "cancelled" runs (e.g. skipped because of an upstream rate limit)
-- alongside "failed" ones — both are retryable the same way.
SELECT * FROM "ActivityRun"
WHERE "state" IN ('failed', 'cancelled') AND (sqlc.narg('op')::text IS NULL OR "op" = sqlc.narg('op'))
ORDER BY "createdAt" DESC;

-- name: DeleteActivityRunsBefore :exec
DELETE FROM "ActivityRun" WHERE "createdAt" < $1;
