-- name: ListJobSources :many
SELECT * FROM "JobSource" ORDER BY "key";

-- name: GetJobSourceByKey :one
SELECT * FROM "JobSource" WHERE "key" = $1;

-- name: UpsertJobSource :exec
INSERT INTO "JobSource" ("key", "kind", "config")
VALUES ($1, $2, $3)
ON CONFLICT ("key") DO UPDATE SET "kind" = EXCLUDED."kind";

-- name: SetJobSourceEnabled :exec
UPDATE "JobSource" SET "enabled" = $2 WHERE "key" = $1;

-- name: SetJobSourceConfig :exec
UPDATE "JobSource" SET "config" = $2 WHERE "key" = $1;

-- name: SetJobSourceHealthy :exec
UPDATE "JobSource" SET "healthy" = $2 WHERE "key" = $1;

-- name: ListEnabledJobSources :many
SELECT * FROM "JobSource" WHERE "enabled" = true;
