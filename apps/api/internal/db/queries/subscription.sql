-- name: ListSubscriptions :many
SELECT * FROM "Subscription" ORDER BY "createdAt" ASC;

-- name: ListSubscriptionsBySource :many
SELECT * FROM "Subscription" WHERE "sourceKey" = $1 ORDER BY "createdAt" ASC;

-- name: GetSubscription :one
SELECT * FROM "Subscription" WHERE "id" = $1;

-- name: ListEnabledSubscriptions :many
SELECT * FROM "Subscription" WHERE "enabled" ORDER BY "createdAt" ASC;

-- name: CreateSubscription :one
INSERT INTO "Subscription" ("sourceKey", "name", "url", "enabled", "cron")
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateSubscription :one
UPDATE "Subscription" SET
  "name" = COALESCE(sqlc.narg('name'), "name"),
  "url" = COALESCE(sqlc.narg('url'), "url"),
  "enabled" = COALESCE(sqlc.narg('enabled'), "enabled"),
  "cron" = COALESCE(sqlc.narg('cron'), "cron")
WHERE "id" = sqlc.arg('id')
RETURNING *;

-- name: DeleteSubscription :exec
DELETE FROM "Subscription" WHERE "id" = $1;

-- name: TouchSubscriptionLastRun :exec
UPDATE "Subscription" SET "lastRunAt" = now() WHERE "id" = $1;

-- name: ClaimSubscriptionRun :one
-- Compare-and-swap on "lastRunAt", the subscription twin of
-- ClaimSavedSearchRun: the scheduler claims a due subscription before
-- enqueueing its ingest task, so concurrent schedulers can't both scrape it.
UPDATE "Subscription" SET "lastRunAt" = now()
WHERE "id" = sqlc.arg('id')
  AND "lastRunAt" IS NOT DISTINCT FROM sqlc.narg('expected_last_run_at')::timestamp
RETURNING "id";
