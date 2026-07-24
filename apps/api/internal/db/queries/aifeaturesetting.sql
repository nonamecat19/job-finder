-- name: ListAiFeatureSettings :many
SELECT * FROM "AiFeatureSetting" ORDER BY "featureKey";

-- name: GetAiFeatureSetting :one
SELECT * FROM "AiFeatureSetting" WHERE "featureKey" = $1;

-- name: UpdateAiFeatureSetting :one
UPDATE "AiFeatureSetting"
SET "enabled" = $2, "threshold" = $3, "updatedAt" = now()
WHERE "featureKey" = $1
RETURNING *;
