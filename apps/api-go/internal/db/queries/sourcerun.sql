-- name: InsertSourceRun :one
INSERT INTO "SourceRun" ("sourceId", "searchId")
VALUES ($1, $2)
RETURNING *;

-- name: FinishSourceRunOk :exec
UPDATE "SourceRun" SET "finishedAt" = now(), "ok" = true, "found" = $2, "new" = $3
WHERE "id" = $1;

-- name: FinishSourceRunError :exec
UPDATE "SourceRun" SET "finishedAt" = now(), "ok" = false, "error" = $2
WHERE "id" = $1;

-- name: RecentSourceRunsForSource :many
SELECT "ok" FROM "SourceRun"
WHERE "sourceId" = $1 AND "ok" IS NOT NULL
ORDER BY "startedAt" DESC
LIMIT $2;

-- name: RecentRunsJoined :many
SELECT
  sr."id" AS id,
  js."key" AS source_key,
  sr."searchId" AS search_id,
  sr."startedAt" AS started_at,
  sr."finishedAt" AS finished_at,
  sr."ok" AS ok,
  sr."found" AS found,
  sr."new" AS new,
  sr."error" AS error
FROM "SourceRun" sr
JOIN "JobSource" js ON js."id" = sr."sourceId"
ORDER BY sr."startedAt" DESC
LIMIT $1;
