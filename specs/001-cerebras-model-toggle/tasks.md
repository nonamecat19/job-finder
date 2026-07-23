---
description: "Task list for Cerebras Free-Tier Model Toggle"
---

# Tasks: Cerebras Free-Tier Model Toggle

**Input**: Design documents from `/specs/001-cerebras-model-toggle/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/llm-settings.md
**Tests**: Included — Constitution Principle IV (test discipline per language) is mandatory
for this repo, and plan.md defines Go + vitest coverage.

**Organization**: Tasks grouped by user story. Foundational phase (provider + router +
settings store + DTOs) is a hard prerequisite for all stories.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- Story labels: [US1] switch-all, [US2] per-task, [US3] credential/error surfacing

## Path Conventions

Web-app monorepo: Go backend `apps/api/`, React dashboard `apps/dashboard/`, shared TS
`packages/shared/`.

---

## Phase 1: Setup (Shared Infrastructure)

- [X] T001 Add `CEREBRAS_API_KEY` (secret, optional, no default) and `CEREBRAS_BASE_URL` (default `https://api.cerebras.ai/v1`) to `apps/api/internal/config/config.go` (`Config` struct, `defaults`, `optionalKeys`) and to `apps/api/.env.example`
- [X] T002 [P] Add config coverage for the two new keys in `apps/api/internal/config/config_test.go` (default base URL applied, key empty by default)

---

## Phase 2: Foundational (Blocking Prerequisites)

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

### Data layer

- [X] T003 Create goose migration `apps/api/internal/db/migrations/00018_llm_task_setting.sql` — table `llm_task_setting` (task_key PK, provider NOT NULL default `ollama` + CHECK in (`ollama`,`cerebras`), model text, updated_at timestamptz); seed rows for `match`,`generation`,`rephrase`,`ghost`,`default` (provider `ollama`, model `''`); down drops table
- [X] T004 Add sqlc queries in `apps/api/internal/db/queries/llmsetting.sql` — `ListLlmTaskSettings`, `UpsertLlmTaskSetting(task_key, provider, model)`
- [X] T005 Run `cd apps/api && sqlc generate`; commit regenerated `apps/api/internal/db/sqlcgen/*` (CI drift gate must be clean)

### Cerebras provider

- [X] T006 [P] Port legacy Cerebras provider to `apps/api/internal/llm/cerebras.go` implementing `llm.Provider` (`ModelName`, `Complete`, `CompleteJSON`); OpenAI-compatible `POST {baseURL}/chat/completions`, Bearer auth, default model `gpt-oss-120b`, embeddings delegate to the Ollama provider; adapt to current `CompleteOptions` (Model/MaxTokens/Temp/SystemPrompt) and package error/strip-fences helpers
- [X] T007 [P] Add curated free-tier model list in `apps/api/internal/llm/models.go` — `[]CerebrasModel{ID,Label,IsDefault}` with exactly one default (`gpt-oss-120b`); confirm ids against Cerebras docs (research R2)
- [X] T008 [P] Unit tests `apps/api/internal/llm/cerebras_test.go` — request shape, choice parsing, ≥400 error mapping (401/403/429 messages), empty-choices handling (httptest server, no live call)

### Router + settings service

- [X] T009 Implement `llm.Router` in `apps/api/internal/llm/router.go` — a task-bound value implementing `llm.Provider`; resolves `{provider, model}` for its task key from an atomically-swappable in-memory snapshot; empty model → provider default; exposes a snapshot-setter for the settings service
- [X] T010 Implement settings service in `apps/api/internal/llmsettings/service.go` — load all rows into snapshot at startup, `Get()` returns current per-task settings + `CredentialConfigured`, `Update(tasks)` upserts then reloads snapshot and pushes it to the Routers; validation (known task_key, provider in enum, cerebras model in curated list or empty)
- [X] T011 [P] Unit tests `apps/api/internal/llm/router_test.go` — resolution table: each task × {ollama, cerebras}, empty-model→default, cerebras-selected-but-credential-missing → falls back to ollama
- [X] T012 [P] Unit tests `apps/api/internal/llmsettings/service_test.go` — load snapshot, upsert + reload, validation rejects bad key/provider/model (fake/queries stub)

### DTOs + wiring

- [X] T013 Add LLM-settings DTOs to `apps/api/internal/dto/dto.go` (tygo source of truth) — `LlmSettingsResponse{CredentialConfigured bool, Tasks []LlmTaskSetting}`, `LlmTaskSetting{TaskKey,Provider,Model}`, `LlmModelsResponse{Cerebras []CerebrasModel}`, `UpdateLlmSettingsRequest{Tasks []LlmTaskSetting}`
- [X] T014 Run tygo (`cd apps/api && tygo generate` / `make generate`); commit regenerated `packages/shared/src/generated.ts` (no drift)
- [X] T015 Wire in `apps/api/cmd/server/main.go` — build Ollama + optional Cerebras (only when `CEREBRAS_API_KEY` set), construct `llmsettings.Service` (loads snapshot), create task-bound Routers for `match`/`generation`/`rephrase`/`ghost`/`default`, and inject each Router in place of `(llmProvider, cfg.ModelOr(...))` into matching/generation/ghost/salary/coach services (drop baked model-string args + `CompleteOptions.Model` overrides now owned by the Router)

**Checkpoint**: Providers, Router, persisted settings, and DTOs exist and are wired; services resolve provider/model at call time. User stories can begin.

---

## Phase 3: User Story 1 - Switch inference to Cerebras from Settings (Priority: P1) 🎯 MVP

**Goal**: Operator flips all chat tasks to Cerebras (and back) in one action from Settings; selection persists and applies without restart.

**Independent Test**: With a credential configured, "Switch all to Cerebras" → matching + generation run on Cerebras; reload + restart → still Cerebras (Scenario B).

- [X] T016 [US1] Implement handler `apps/api/internal/httpapi/llm_settings.go` — `Mount` at `/v1/settings/llm`; `GET /v1/settings/llm` (current state + `credentialConfigured`), `PUT /v1/settings/llm` (subset upsert → reload → return full state); mount in `main.go` handler list
- [X] T017 [US1] Handler tests `apps/api/internal/httpapi/llm_settings_test.go` — GET returns seeded state; PUT all-tasks→cerebras persists and returns updated state; PUT applies to newly resolved tasks (snapshot reloaded)
- [X] T018 [P] [US1] Add API client methods in `apps/dashboard/src/lib/api.ts` — `api.settings.getLlm()`, `api.settings.putLlm(body)` (types from `@job-finder/shared`)
- [X] T019 [P] [US1] Add query/mutation hooks in `apps/dashboard/src/features/settings/hooks.ts` — `useLlmSettings()` (query), `useUpdateLlmSettings()` (mutation invalidates the settings query)
- [X] T020 [US1] Create `apps/dashboard/src/features/settings/LlmSettingsCard.tsx` with a "Switch all to Cerebras" / "Switch all to Ollama" action and a display of the current active provider per task; render it from `SettingsPage.tsx` under a new "AI models" section
- [X] T021 [US1] Component test `apps/dashboard/src/features/settings/LlmSettingsCard.test.tsx` — switch-all triggers PUT with all tasks set to the target provider; UI reflects returned state

**Checkpoint**: US1 independently testable — one-action provider switch works end to end (Scenarios A & B). This is the MVP.

---

## Phase 4: User Story 2 - Per-task provider and model selection (Priority: P2)

**Goal**: Each of the four chat tasks independently assignable to Ollama or a specific Cerebras free-tier model.

**Independent Test**: Set generation→Cerebras(gpt-oss-120b), match→Ollama, save; each task runs on its assignment; persists across reload/restart (Scenario C).

- [X] T022 [US2] Add `GET /v1/settings/llm/models` to `apps/api/internal/httpapi/llm_settings.go` returning the curated Cerebras list; extend handler tests in `llm_settings_test.go`
- [X] T023 [P] [US2] Add `api.settings.llmModels()` in `apps/dashboard/src/lib/api.ts` and `useLlmModels()` hook in `apps/dashboard/src/features/settings/hooks.ts`
- [X] T024 [US2] Extend `apps/dashboard/src/features/settings/LlmSettingsCard.tsx` — per-task row matrix: provider select (Ollama/Cerebras) + Cerebras model dropdown (from `useLlmModels`, default preselected); save issues a PUT with per-task values
- [X] T025 [US2] Extend `apps/dashboard/src/features/settings/LlmSettingsCard.test.tsx` — per-task assignment PUTs correct `{taskKey,provider,model}` set; model dropdown only enabled when provider=cerebras
- [X] T026 [P] [US2] Backend validation test in `llm_settings_test.go` — PUT with unknown model id → 400; per-task mixed providers persist and resolve correctly

**Checkpoint**: US2 works on top of US1 — per-task mix of Ollama/Cerebras with model choice (Scenario C).

---

## Phase 5: User Story 3 - Understand and recover from provider/credential problems (Priority: P3)

**Goal**: Clear feedback when Cerebras is unusable (missing/invalid credential, quota/rate limit); no silent failures.

**Independent Test**: No key set + task=Cerebras → Settings shows "credential not configured", task keeps running on Ollama; forced Cerebras error surfaces on task status; key never in response/logs (Scenarios E & F).

- [X] T027 [US3] Enforce missing-credential fallback in `apps/api/internal/llm/router.go` — cerebras-selected + no credential → resolve to Ollama and mark the fallback reason; ensure `CredentialConfigured` reflected in `GET` response
- [X] T028 [US3] Ensure Cerebras runtime errors (401/403/429/model) propagate through the existing task error/status surface with actionable messages; confirm the API key is never written to logs (log provider/model/status only) — cover in `apps/api/internal/llm/cerebras_test.go` / router test
- [X] T029 [P] [US3] LlmSettingsCard credential-missing banner in `apps/dashboard/src/features/settings/LlmSettingsCard.tsx` — when `credentialConfigured=false`, show a warning near any Cerebras-assigned task and note it stays on Ollama until a key is configured
- [X] T030 [P] [US3] Extend `apps/dashboard/src/features/settings/LlmSettingsCard.test.tsx` — banner shows when `credentialConfigured=false`; hidden when true

**Checkpoint**: US3 complete — robustness and operator feedback for the Cerebras path (Scenarios E & F).

---

## Phase 6: Polish & Cross-Cutting

- [X] T031 [P] Add env-gated live Cerebras smoke test (e.g. `apps/api/internal/llm/cerebras_live_test.go`) — real `/chat/completions` call when `CEREBRAS_API_KEY` set, `t.Skip` otherwise (mirrors existing live_test gating)
- [X] T032 [P] Document the toggle + `CEREBRAS_API_KEY`/`CEREBRAS_BASE_URL` in README / relevant docs; note embeddings stay on Ollama and Ollama remains the default
- [X] T033 Run `make test-lint` (go test + vitest + lint) and confirm sqlc/tygo produce no drift; run quickstart Scenarios A–F

---

## Dependencies & Execution Order

- **Setup (T001–T002)** → **Foundational (T003–T015)** → user stories.
- Within Foundational: T003→T004→T005 (migration→queries→gen); T006/T007/T008 parallel; T009 needs T006/T007; T010 needs T004/T005/T009; T013→T014; T015 needs T009/T010.
- **US1 (T016–T021)**: T016 needs Foundational; T017 needs T016; T018/T019 parallel after DTOs (T014); T020 needs T018/T019; T021 needs T020.
- **US2 (T022–T026)**: builds on US1 UI + endpoints. T022→T023→T024→T025; T026 parallel with UI.
- **US3 (T027–T030)**: T027/T028 backend (Router/provider from Foundational); T029/T030 UI on top of US1 card.
- **Polish (T031–T033)**: after the stories it validates.

Story independence: US1 is a standalone MVP. US2 and US3 each extend US1's card/endpoints but are independently testable increments.

## Parallel Opportunities

- Setup: T002 alongside T001 follow-up.
- Foundational: **T006, T007, T008** together; **T011, T012** together (after their subjects).
- US1: **T018, T019** together.
- US3: **T029, T030** together.
- Polish: **T031, T032** together.

## Implementation Strategy

1. **MVP** = Setup + Foundational + US1 (T001–T021): Cerebras support + one-action switch, persisted, no restart. Delivers the core ask.
2. **Increment 2** = US2 (T022–T026): per-task provider/model matrix.
3. **Increment 3** = US3 (T027–T030): credential/error surfacing.
4. **Harden** = Polish (T031–T033): live smoke, docs, full lint/drift gate + quickstart.
