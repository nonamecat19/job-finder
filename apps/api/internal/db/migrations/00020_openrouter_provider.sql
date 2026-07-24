-- +goose Up
-- OpenRouter as a third chat provider, selectable per task alongside Ollama
-- and Cerebras. Only the provider CHECK changes — the OpenRouter API key is
-- env-only (OPENROUTER_API_KEY), never persisted here.
ALTER TABLE "LlmTaskSetting" DROP CONSTRAINT "LlmTaskSetting_provider_check";
ALTER TABLE "LlmTaskSetting"
  ADD CONSTRAINT "LlmTaskSetting_provider_check"
  CHECK ("provider" IN ('ollama', 'cerebras', 'openrouter'));

-- +goose Down
-- Any task left on openrouter falls back to ollama so the old CHECK holds.
UPDATE "LlmTaskSetting" SET "provider" = 'ollama', "model" = '' WHERE "provider" = 'openrouter';
ALTER TABLE "LlmTaskSetting" DROP CONSTRAINT "LlmTaskSetting_provider_check";
ALTER TABLE "LlmTaskSetting"
  ADD CONSTRAINT "LlmTaskSetting_provider_check"
  CHECK ("provider" IN ('ollama', 'cerebras'));
