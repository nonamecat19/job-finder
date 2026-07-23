-- name: GetAutoGenerateSetting :one
SELECT * FROM "AutoGenerateSetting" WHERE "id" = 'default';

-- name: UpdateAutoGenerateSetting :one
UPDATE "AutoGenerateSetting"
SET "enabled" = $1, "threshold" = $2, "updatedAt" = now()
WHERE "id" = 'default'
RETURNING *;
