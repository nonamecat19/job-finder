-- name: ListStarStoriesByProfile :many
SELECT * FROM "StarStory" WHERE "profileId" = $1 ORDER BY "createdAt" ASC;

-- name: CreateStarStory :one
INSERT INTO "StarStory" (
  "profileId", "title", "situation", "task", "action", "result", "skills", "categories"
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;
