-- name: GetJobByID :one
SELECT * FROM "Job" WHERE "id" = $1;

-- name: GetJobByDedupeKey :one
SELECT "id" FROM "Job" WHERE "dedupeKey" = $1;

-- name: InsertJob :one
INSERT INTO "Job" (
  "dedupeKey", "sourceKey", "externalId", "title", "company", "location",
  "remote", "salaryRaw", "url", "description", "raw", "postedAt"
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: UpdateJobEmbedding :exec
UPDATE "Job" SET "embedding" = $2 WHERE "id" = $1;

-- name: UpdateJobStatus :one
UPDATE "Job" SET "status" = $2 WHERE "id" = $1
RETURNING *;

-- name: DeleteAllJobs :execrows
-- Wipes every job. FK "ON DELETE cascade" clears the dependent
-- Application / GeneratedDocument / MatchResult / Activity rows too.
DELETE FROM "Job";

-- name: ListJobsNeedingDetail :many
SELECT * FROM "Job"
WHERE "sourceKey" = $1 AND "detailScrapedAt" IS NULL
ORDER BY "ingestedAt" ASC
LIMIT $2;

-- name: UpdateJobDetail :one
UPDATE "Job" SET
  "description" = COALESCE(NULLIF(sqlc.arg('description'), ''), "description"),
  "salaryRaw" = COALESCE(sqlc.narg('salaryRaw'), "salaryRaw"),
  "location" = COALESCE(sqlc.narg('location'), "location"),
  "remote" = "remote" OR sqlc.arg('remote')::bool,
  "raw" = sqlc.arg('raw'),
  "postedAt" = COALESCE(sqlc.narg('postedAt'), "postedAt"),
  "detailScrapedAt" = now()
WHERE "id" = sqlc.arg('id')
RETURNING *;
