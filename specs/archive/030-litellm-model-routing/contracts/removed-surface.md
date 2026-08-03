# Contract: Removed Surface (deletion checklist)

Everything listed here MUST be absent when the feature is done. Each line is independently verifiable with a grep.

## HTTP API (FR-002)

| Endpoint | Status |
|----------|--------|
| `GET /v1/settings/llm` | removed — 404 |
| `PUT /v1/settings/llm` | removed — 404 |
| `GET /v1/settings/llm/models` | removed — 404 |

`app.LlmSettings.Mount` is dropped from the mount list in `apps/api/cmd/server/servers.go`, and the `LlmSettings` field from the `App` struct.

## Go backend

- `apps/api/internal/llmsettings/` — whole tree (`llmsettings.go`, `domain/`, `application/`, `interfaces/http/`, all tests).
- `apps/api/internal/platform/llm/infrastructure/cerebras/` — whole package (client, curated `Models`, `errors.go` re-export shim, unit + live tests).
- `apps/api/internal/platform/llm/application/router.go` — `SnapshotHolder`, `RouterSnapshot`, `TaskSetting`, `TaskProvider` and its constants.
- `apps/api/internal/platform/llm/llm.go` — Cerebras and snapshot re-exports (see [task-router.md](./task-router.md) C3).
- `apps/api/internal/dto/settings.go` — `LlmTaskSettingDto`, `LlmSettingsResponseDto`, `UpdateLlmSettingsRequestDto`, `CerebrasModelDto`, `LlmModelsResponseDto`. `AiFeatureSettingDto` stays.
- `apps/api/internal/config/config.go` + `defaults.go` — `CerebrasAPIKey`, `CerebrasBaseURL`, their default entry and secret-list entry; `config_test.go` cases that assert them.
- `apps/api/cmd/server/compose.go` — `llmsettings` imports, `llmHandles.Settings`/`SettingsHandler`, the holder wiring, the Cerebras leg of `NewProviders`.
- `apps/api/internal/db/queries/llmsetting.sql` and the regenerated `sqlcgen/llmsetting.sql.go` + `LlmTaskSetting` model.
- `infrastructure/shared` rate-limit breaker — only if no caller survives the Cerebras deletion.

## Database

- Migration `00033_drop_llm_task_setting.sql` drops `"LlmTaskSetting"` (down recreates + reseeds).
- No remaining reference to `LlmTaskSetting` outside `apps/api/internal/db/migrations/` and `specs/`.

## Dashboard + shared types (FR-001)

- `apps/dashboard/src/features/settings/LlmSettingsCard.tsx` and `LlmSettingsCard.test.tsx` — deleted.
- `SettingsPage.tsx` — the `AI models` tile removed; `AI features` and `Danger zone` tiles unchanged. `SettingsPage.test.tsx` updated to assert the tile is gone.
- `features/settings/hooks.ts` — `useLlmSettings`, `useLlmModels`, `useUpdateLlmSettings`.
- `lib/api.ts` — `settings.getLlm`, `settings.putLlm`, `settings.llmModels` and the now-unused DTO imports.
- `lib/queryKeys.ts` — the whole `llmSettings` key group.
- `features/status/StatusPage.tsx` — provider-specific copy ("an upstream provider (Cerebras)") genericised to "an upstream AI provider".
- `packages/shared/src/index.ts` — `LlmTaskSettingDto`, `LlmSettingsResponseDto`, `CerebrasModelDto`, `LlmModelsResponseDto` and their doc comments; `packages/shared/src/generated.ts` regenerated, `dist/` rebuilt.

## Documentation / environment

- `.env.example` — `CEREBRAS_API_KEY`/`CEREBRAS_BASE_URL` block and all prose describing dashboard-selectable providers/models; add `GROQ_API_KEY`, `COHERE_API_KEY`, and a note that provider keys are consumed by the litellm container only (FR-015).
- Any README/AGENTS text describing the Settings "AI models" card.

## Verification greps

```bash
rg -n "LlmTaskSetting|llmsettings|CerebrasModel|settings/llm" apps packages --glob '!node_modules'   # migrations only
rg -n "SnapshotHolder|TaskProviderCerebras|IsSupportedCerebrasModel" apps                            # no hits
rg -n "CEREBRAS_API_KEY" apps .env.example                                                            # no app hits; env doc only
```
