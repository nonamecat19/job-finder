-- name: ListSubscriptions :many
SELECT * FROM "Subscription" ORDER BY "createdAt" ASC;

-- name: ListSubscriptionsBySource :many
SELECT * FROM "Subscription" WHERE "sourceKey" = $1 ORDER BY "createdAt" ASC;

-- name: GetSubscription :one
SELECT * FROM "Subscription" WHERE "id" = $1;

-- name: ListEnabledSubscriptions :many
-- The scheduler's due-subscription query. Manual rows carry a cron they never
-- honour, so they are excluded by kind here rather than by being disabled —
-- disabled means "paused by the operator" and must keep that meaning (041 D3).
SELECT * FROM "Subscription" WHERE "enabled" AND "kind" = 'crawl' ORDER BY "createdAt" ASC;

-- name: CreateSubscription :one
INSERT INTO "Subscription" ("sourceKey", "name", "url", "enabled", "cron", "kind")
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: EnsureManualSubscription :one
-- Insert-or-return the single manual subscription for a source. The partial
-- unique index "Subscription_manual_per_source_idx" is what makes the conflict
-- target work, so two concurrent first-time adds settle on one row.
INSERT INTO "Subscription" ("sourceKey", "name", "url", "enabled", "cron", "kind")
VALUES (sqlc.arg('source_key'), 'Manual', '', true, sqlc.arg('cron'), 'manual')
ON CONFLICT ("sourceKey") WHERE "kind" = 'manual'
DO UPDATE SET "kind" = 'manual'
RETURNING *;

-- name: GetManualSubscription :one
SELECT * FROM "Subscription" WHERE "sourceKey" = $1 AND "kind" = 'manual';

-- name: CountJobsForSubscription :one
-- The FR-016 deletion guard: a subscription with vacancies attached cannot be
-- deleted or disabled, because doing so would orphan them.
SELECT count(*) FROM "Job" WHERE "subscriptionId" = $1;

-- name: ManualSubscriptionStats :one
-- FR-013 / FR-017h: how many vacancies this manual subscription added, and when
-- the most recent one landed. Derived from run records, so failed attempts are
-- counted in the history without inflating the total.
SELECT
  COALESCE(SUM("new"), 0)::bigint AS added,
  MAX("startedAt")::timestamp AS last_added_at
FROM "SourceRun"
WHERE "subscriptionId" = $1 AND "trigger" = 'manual' AND "new" > 0;

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
