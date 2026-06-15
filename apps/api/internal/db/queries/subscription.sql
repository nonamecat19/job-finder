-- name: ListSubscriptions :many
SELECT * FROM "Subscription" ORDER BY "createdAt" ASC;

-- name: ListSubscriptionsBySource :many
SELECT * FROM "Subscription" WHERE "sourceKey" = $1 ORDER BY "createdAt" ASC;

-- name: GetSubscription :one
SELECT * FROM "Subscription" WHERE "id" = $1;

-- name: CreateSubscription :one
INSERT INTO "Subscription" ("sourceKey", "name", "url", "enabled")
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateSubscription :one
UPDATE "Subscription" SET
  "name" = COALESCE(sqlc.narg('name'), "name"),
  "url" = COALESCE(sqlc.narg('url'), "url"),
  "enabled" = COALESCE(sqlc.narg('enabled'), "enabled")
WHERE "id" = sqlc.arg('id')
RETURNING *;

-- name: DeleteSubscription :exec
DELETE FROM "Subscription" WHERE "id" = $1;

-- name: TouchSubscriptionLastRun :exec
UPDATE "Subscription" SET "lastRunAt" = now() WHERE "id" = $1;
