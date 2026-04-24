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
