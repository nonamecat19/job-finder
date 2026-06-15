-- name: MaxDocumentVersion :one
SELECT COALESCE(MAX("version"), 0)::int AS max_version
FROM "GeneratedDocument"
WHERE "jobId" = $1 AND "type" = $2;

-- name: InsertGeneratedDocument :one
INSERT INTO "GeneratedDocument" ("jobId", "type", "version", "content", "pdfPath", "model")
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListDocumentsForJob :many
SELECT * FROM "GeneratedDocument"
WHERE "jobId" = $1
ORDER BY "type" ASC, "version" DESC;

-- name: GetDocumentByID :one
SELECT * FROM "GeneratedDocument" WHERE "id" = $1;

-- name: UpdateDocumentContent :one
UPDATE "GeneratedDocument" SET "content" = $2, "pdfPath" = $3
WHERE "id" = $1
RETURNING *;
