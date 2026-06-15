-- name: ListProfiles :many
SELECT * FROM "Profile" ORDER BY "updatedAt" DESC;

-- name: GetProfile :one
SELECT * FROM "Profile" WHERE "id" = $1;

-- name: GetDefaultProfile :one
SELECT * FROM "Profile" ORDER BY "updatedAt" DESC LIMIT 1;

-- name: CreateProfile :one
INSERT INTO "Profile" ("name", "document", "extraNotes")
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateProfile :exec
UPDATE "Profile" SET
  "name" = COALESCE(sqlc.narg('name'), "name"),
  "document" = COALESCE(sqlc.narg('document'), "document"),
  "updatedAt" = now()
WHERE "id" = sqlc.arg('id');

-- name: UpdateProfileExtraNotes :exec
UPDATE "Profile" SET "extraNotes" = $2, "updatedAt" = now() WHERE "id" = $1;

-- name: DeleteProfile :exec
DELETE FROM "Profile" WHERE "id" = $1;

-- name: UpdateProfileEmbedding :exec
UPDATE "Profile" SET "embedding" = $2, "embedModel" = $3 WHERE "id" = $1;

-- name: ProfileHasEmbedding :one
SELECT ("embedding" IS NOT NULL) AS has FROM "Profile" WHERE "id" = $1;

-- name: ProfileSimilarity :one
SELECT (1 - ("embedding" <=> $2))::float8 AS similarity FROM "Profile" WHERE "id" = $1;
