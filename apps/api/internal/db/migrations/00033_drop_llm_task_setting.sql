-- +goose Up
-- Gateway-owned model routing (030-litellm-model-routing): per-task
-- provider/model selection moves out of the database entirely. Routing now
-- lives in gateway/config.yaml and environment variables only — nothing
-- reads this table anymore.
DROP TABLE IF EXISTS "LlmTaskSetting";

-- +goose Down
CREATE TABLE "LlmTaskSetting" (
  "taskKey"   text PRIMARY KEY,
  "provider"  text NOT NULL DEFAULT 'ollama' CHECK ("provider" IN ('ollama', 'cerebras', 'openrouter')),
  "model"     text NOT NULL DEFAULT '',
  "updatedAt" timestamp(3) NOT NULL DEFAULT now()
);

INSERT INTO "LlmTaskSetting" ("taskKey", "provider", "model") VALUES
  ('match', 'ollama', ''),
  ('generation', 'ollama', ''),
  ('rephrase', 'ollama', ''),
  ('ghost', 'ollama', ''),
  ('default', 'ollama', '');
