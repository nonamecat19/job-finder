-- name: InsertFreshMatchNotification :one
INSERT INTO "FreshMatchNotification" (
    "jobId", "matchResultId", "profileId", "fresh"
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: CountRecentNotificationsByProfile :one
SELECT COUNT(*) FROM "FreshMatchNotification"
WHERE "profileId" = $1 AND "createdAt" > now() - make_interval(hours => $2::int);

-- name: ListRecentNotificationsByProfile :many
SELECT * FROM "FreshMatchNotification"
WHERE "profileId" = $1
ORDER BY "createdAt" DESC
LIMIT $2;

-- name: ListRecentNotificationsWithJob :many
SELECT
    n."id", n."jobId", n."matchResultId", n."profileId", n."fresh", n."seen", n."createdAt",
    j."title" AS job_title,
    j."company",
    mr."score" AS match_score
FROM "FreshMatchNotification" n
JOIN "Job" j ON j."id" = n."jobId"
JOIN "MatchResult" mr ON mr."id" = n."matchResultId"
WHERE n."profileId" = $1
ORDER BY n."createdAt" DESC
LIMIT $2;

-- name: MarkNotificationSeen :exec
UPDATE "FreshMatchNotification" SET "seen" = true
WHERE "id" = $1 AND "seen" = false;

-- name: CountUnseenNotificationsByProfile :one
SELECT COUNT(*) FROM "FreshMatchNotification"
WHERE "profileId" = $1 AND "seen" = false;
