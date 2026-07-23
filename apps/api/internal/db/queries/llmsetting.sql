-- name: ListLlmTaskSettings :many
SELECT * FROM "LlmTaskSetting" ORDER BY "taskKey";

-- name: UpsertLlmTaskSetting :one
INSERT INTO "LlmTaskSetting" (
  "taskKey", "provider", "model"
) VALUES (
  $1, $2, $3
)
ON CONFLICT ("taskKey") DO UPDATE SET
  "provider" = EXCLUDED."provider",
  "model" = EXCLUDED."model",
  "updatedAt" = now()
RETURNING *;
