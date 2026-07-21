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
