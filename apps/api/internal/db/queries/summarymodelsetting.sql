-- name: GetSummaryModelSetting :one
SELECT * FROM "SummaryModelSetting" WHERE "id" = 'default';

-- name: UpdateSummaryModelSetting :one
UPDATE "SummaryModelSetting"
SET "optionId" = $1,
    "updatedAt" = now()
WHERE "id" = 'default'
RETURNING *;
