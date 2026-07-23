-- +goose Up
-- Cerebras free-tier model toggle (001-cerebras-model-toggle): one row per
-- chat task, holding which LLM provider/model it resolves to at call time.
-- No credential is stored here — the Cerebras API key is env-only (never
-- persisted, never returned to the browser). Seeded with all five task keys
-- so a read is never missing a row.
CREATE TABLE "LlmTaskSetting" (
  "taskKey"   text PRIMARY KEY,
  "provider"  text NOT NULL DEFAULT 'ollama' CHECK ("provider" IN ('ollama', 'cerebras')),
  "model"     text NOT NULL DEFAULT '',
  "updatedAt" timestamp(3) NOT NULL DEFAULT now()
);

INSERT INTO "LlmTaskSetting" ("taskKey", "provider", "model") VALUES
  ('match', 'ollama', ''),
  ('generation', 'ollama', ''),
  ('rephrase', 'ollama', ''),
  ('ghost', 'ollama', ''),
  ('default', 'ollama', '');

-- +goose Down
DROP TABLE IF EXISTS "LlmTaskSetting";
