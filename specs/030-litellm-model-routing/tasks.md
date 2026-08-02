---

description: "Task list for 030-litellm-model-routing"
---

# Tasks: Gateway-Owned Model Routing

**Input**: Design documents from `/specs/030-litellm-model-routing/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/)

**Tests**: Included. Constitution Principle IV makes per-language suites mandatory, and [contracts/task-router.md](./contracts/task-router.md) §C5 lists explicit test obligations.

**Organization**: Grouped by user story. Foundational phase deliberately stops *using* the settings context before User Story 1 deletes it, so the Go build stays green at every checkpoint.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 / US2 / US3 from spec.md
- Exact file paths included in every task

## Path Conventions

Web app monorepo: `apps/api/` (Go), `apps/dashboard/` (React), `packages/shared/` (TS types), `gateway/` (proxy config), repo-root `docker-compose.yml` / `.env.example`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Pin the facts the gateway config depends on, and make every provider key reachable inside the proxy container.

- [X] T001 Verify current free-tier model IDs and `response_format: json_object` support for Cerebras, Groq (`groq/`), Cohere (`cohere_chat/`) and the OpenRouter models already referenced in `gateway/config.yaml`; record the verified IDs plus verification date in `specs/030-litellm-model-routing/research.md` under R8 (blocks T020, per research R3/R8)
- [X] T002 Confirm the `litellm_settings.fallbacks` key shape accepted by the pinned `ghcr.io/berriai/litellm:main-stable` image by running it locally against a scratch config (`litellm --config … --detailed_debug`); note the confirmed shape in `specs/030-litellm-model-routing/research.md` under R1
- [X] T003 [P] Add `GROQ_API_KEY` and `COHERE_API_KEY` to the litellm service environment in `docker-compose.yml` with empty defaults (`${VAR:-}`), alongside `CEREBRAS_API_KEY`, `OLLAMA_URL` and `OLLAMA_KEY`, per contracts/gateway-config.md §C4
- [X] T004 [P] Update `.env.example`: add `GROQ_API_KEY` and `COHERE_API_KEY`, delete the `CEREBRAS_API_KEY`/`CEREBRAS_BASE_URL` block and all prose describing dashboard-selectable providers/models, and document that provider keys are consumed by the litellm container only (FR-015)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Replace the snapshot-driven Router with a static task Router and stop wiring the settings context, so the backend routes by task key alone.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T005 Rewrite `apps/api/internal/platform/llm/application/router.go` as a static task Router per contracts/task-router.md §C1–C2: `NewRouter(taskKey string, gateway, local domain.Provider, localModel string)`, keeping `Complete`/`CompleteJSON`/`Embed`/`ModelName`/`ProviderClass`; delete `SnapshotHolder`, `RouterSnapshot`, `TaskSetting`, `TaskProvider` and its constants
- [X] T006 [P] Rewrite `apps/api/internal/platform/llm/application/router_test.go` to cover contracts/task-router.md §C5: nil-gateway routes chat to Ollama with the per-task local model and reports `IsHosted()`-derived class; non-nil gateway routes with `Model == taskKey` and reports `hosted`; `Embed` always hits Ollama; an explicit caller `CompleteOptions.Model` is never overwritten
- [X] T007 Update the facade `apps/api/internal/platform/llm/llm.go` per contracts/task-router.md §C3: drop the Cerebras and snapshot re-exports, re-point `ErrRateLimited`/`ErrCredentialRejected`/`ErrInsufficientCredits`/`ErrModelUnavailable`/`ErrProviderUnavailable`/`ErrInvalidResponse`/`Terminal`/`Retryable` at `infrastructure/shared`, and change `NewProviders` to return `(*OllamaProvider, *GatewayProvider, error)`
- [X] T008 Delete `apps/api/internal/platform/llm/infrastructure/cerebras/` in full (`cerebras.go`, `models.go`, `errors.go`, `cerebras_test.go`, `cerebras_live_test.go`) per research R7
- [X] T009 [P] Remove `CerebrasAPIKey`/`CerebrasBaseURL` from `apps/api/internal/config/config.go`, the `CEREBRAS_BASE_URL` default and `CEREBRAS_API_KEY` secret entry from `apps/api/internal/config/defaults.go`, and the corresponding assertions from `apps/api/internal/config/config_test.go`
- [X] T010 Rework `composeLLM` in `apps/api/cmd/server/compose.go`: build Ollama + optional gateway, construct the five task Routers with their `LLM_MODEL_*` values, and remove `llmsettings.NewService`, the holder wiring, and `llmHandles.Settings`/`SettingsHandler`; leave `queueClassResolvers` unchanged (it keeps consuming `*llm.Router`)
- [X] T011 Remove the `LlmSettings` field from the `App` struct and `app.LlmSettings.Mount` from the mount list in `apps/api/cmd/server/servers.go`, and update the stale Cerebras comment on the AI-concurrency block
- [X] T012 Verify the backend compiles and unit tests pass with the settings context now unused: `cd apps/api && go build ./... && go test ./...`

**Checkpoint**: Backend routes every chat task through the gateway (or Ollama when `GATEWAY_URL` is empty) with zero reads of persisted settings. `internal/llmsettings` still exists but is dead code.

---

## Phase 3: User Story 1 - Operator stops managing models in the dashboard (Priority: P1) 🎯 MVP

**Goal**: Delete the AI-model settings surface end to end — dashboard tile, HTTP endpoints, DTOs, bounded context, and DB table.

**Independent Test**: Settings page shows no provider/model controls while "AI features" and "Danger zone" still work; `GET/PUT /v1/settings/llm` and `GET /v1/settings/llm/models` return 404; `\d "LlmTaskSetting"` finds no relation; a match and a generation still complete.

### Tests for User Story 1

- [X] T013 [P] [US1] Update `apps/dashboard/src/features/settings/SettingsPage.test.tsx` to assert the "AI models" tile is absent and the "AI features" and "Danger zone" tiles still render
- [X] T014 [P] [US1] Delete `apps/dashboard/src/features/settings/LlmSettingsCard.test.tsx`

### Implementation for User Story 1

- [X] T015 [US1] Add `apps/api/internal/db/migrations/00033_drop_llm_task_setting.sql` — Up: `DROP TABLE IF EXISTS "LlmTaskSetting";`; Down: recreate the table with the 00020-era provider CHECK and re-seed the five task rows at `provider='ollama', model=''` (data-model.md §5)
- [X] T016 [US1] Delete `apps/api/internal/db/queries/llmsetting.sql` and regenerate typed DB access (`sqlc generate`), removing `apps/api/internal/db/sqlcgen/llmsetting.sql.go` and the `LlmTaskSetting` model from `sqlcgen/models.go`
- [X] T017 [US1] Delete `apps/api/internal/llmsettings/` in full (`llmsettings.go`, `domain/`, `application/`, `interfaces/http/` and all their tests)
- [X] T018 [P] [US1] Remove `LlmTaskSettingDto`, `LlmSettingsResponseDto`, `UpdateLlmSettingsRequestDto`, `CerebrasModelDto` and `LlmModelsResponseDto` from `apps/api/internal/dto/settings.go`, keeping `AiFeatureSettingDto`
- [X] T019 [P] [US1] Remove the matching interfaces and doc comments from `packages/shared/src/index.ts`, regenerate `packages/shared/src/generated.ts` (tygo) and rebuild the package (`pnpm --filter @job-finder/shared build`)
- [X] T020 [P] [US1] Delete `apps/dashboard/src/features/settings/LlmSettingsCard.tsx` and remove its import and the `AI models` `<Tile>` from `apps/dashboard/src/features/settings/SettingsPage.tsx`
- [X] T021 [P] [US1] Remove `useLlmSettings`, `useLlmModels` and `useUpdateLlmSettings` from `apps/dashboard/src/features/settings/hooks.ts`
- [X] T022 [P] [US1] Remove `settings.getLlm`, `settings.putLlm`, `settings.llmModels` and the now-unused DTO imports from `apps/dashboard/src/lib/api.ts`, and the `llmSettings` key group from `apps/dashboard/src/lib/queryKeys.ts`
- [X] T023 [P] [US1] Genericise the provider-specific copy in `apps/dashboard/src/features/status/StatusPage.tsx` ("an upstream provider (Cerebras)" → "an upstream AI provider")
- [X] T024 [US1] Run the deletion verification greps from contracts/removed-surface.md and confirm the only surviving `LlmTaskSetting` references are in `apps/api/internal/db/migrations/` and `specs/`
- [X] T025 [US1] Run `make test` (go + vitest) and confirm both suites pass with the deleted tests gone

**Checkpoint**: The settings surface is gone, the schema is clean, and AI tasks still run.

---

## Phase 4: User Story 2 - Free-tier providers first, aggregator as backup (Priority: P1)

**Goal**: Ordered failover in the proxy — Cerebras → Groq → Cohere → OpenRouter → Ollama — with per-request visibility of which model served.

**Independent Test**: With all keys healthy a free-tier model serves every task; blanking each free-tier key in turn shifts traffic down the chain without user-visible failure; blanking every hosted key lands on Ollama; `GATEWAY_URL=` empty serves from Ollama directly.

### Tests for User Story 2

- [X] T026 [P] [US2] Add unit tests in `apps/api/internal/platform/llm/infrastructure/gateway/gateway_test.go` covering contracts/task-router.md §C4: the served model is read from the response `model` field, a missing/unparsable field yields `served_model=unknown` without an error, and error classification is unchanged

### Implementation for User Story 2

- [X] T027 [US2] Rewrite `gateway/config.yaml` per contracts/gateway-config.md §C3: one public group per task key on a free-tier deployment, per-task tier groups for Groq / Cohere / OpenRouter, a shared terminal `local` Ollama deployment (`api_base` and key from env), and `litellm_settings.fallbacks` listing each chain in order; move `fallbacks` out of `litellm_params` (research R1) and pin the T001-verified model IDs with a dated comment
- [X] T028 [US2] Set `drop_params: true`, `num_retries`, `request_timeout` (below the adapter's 120 s client timeout), `allowed_fails` and `cooldown_time` in `gateway/config.yaml` per contracts/gateway-config.md §C3
- [X] T029 [US2] Capture the served model in `apps/api/internal/platform/llm/infrastructure/gateway/gateway.go`: parse `model` from the chat-completion response, read `x-litellm-model-id` when present, and emit one structured log line per request with task key, requested group, served model, duration and outcome (FR-012)
- [X] T030 [US2] Validate the happy path against a running stack: `make up`, trigger a match and a generation, and confirm `served_model` names the first free-tier model (quickstart Scenario 2)
- [X] T031 [US2] Validate failover by blanking each free-tier key in turn and recreating only the litellm container, confirming each task still succeeds and `served_model` advances one tier at a time down to the OpenRouter model (quickstart Scenario 3)
- [X] T032 [US2] Validate the terminal tier by blanking every hosted key and confirming tasks complete on the Ollama deployment (quickstart Scenario 4, FR-008)
- [X] T033 [US2] Validate the app-side fallback with `GATEWAY_URL=` empty: matches and generations succeed against Ollama directly and `GET /api/v1/activity/queues` still reports an admission-gate class for every LLM queue (quickstart Scenario 5, FR-009/FR-013)
- [X] T034 [US2] Validate that embeddings are untouched — re-embed a profile and confirm no gateway request is logged for it (quickstart Scenario 7, FR-014)

**Checkpoint**: Routing prefers free tiers, fails over automatically, and never dead-ends.

---

## Phase 5: User Story 3 - Routing is changeable in one place (Priority: P2)

**Goal**: Make `gateway/config.yaml` the self-explanatory single edit point for model changes.

**Independent Test**: Change one task's primary model, restart only the litellm container, and observe the new model serving that task while other tasks are unchanged and no application container was rebuilt.

- [X] T035 [US3] Add a header comment block to `gateway/config.yaml` documenting the group-naming convention, the mandatory chain order (free tiers → OpenRouter → local), the JSON-capability requirement from contracts/gateway-config.md §C5, and the edit-then-`docker compose restart litellm` workflow
- [X] T036 [US3] Confirm in `gateway/config.yaml` that every task key the backend can request (`match`, `generation`, `rephrase`, `ghost`, `default`) has a group and a complete chain, and that a request naming an unknown group fails loudly rather than defaulting (contracts/gateway-config.md §C1)
- [X] T037 [US3] Validate the change workflow end to end: edit one task's primary model, `docker compose restart litellm`, re-run that task, confirm the new `served_model` and that other tasks are unaffected with no application redeploy (quickstart Scenario 6, SC-003)

**Checkpoint**: An operator can retarget any task from one file in under five minutes.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T038 [P] Remove the rate-limit breaker from `apps/api/internal/platform/llm/infrastructure/shared/errors.go` (and its tests) only if no caller survives the Cerebras deletion; otherwise leave it and note the surviving caller
- [X] T039 [P] Update `README.md` and `AGENTS.md`/`CLAUDE.md` sections describing the Settings "AI models" card or dashboard-selectable providers, replacing them with a pointer to `gateway/config.yaml`
- [X] T040 Run the full quickstart (`specs/030-litellm-model-routing/quickstart.md`, Scenarios 1–7) against a clean `make up` stack
- [X] T041 Run `make test-lint` from the repo root (`Makefile`) as the cross-app done gate required by Constitution Principle IV

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies; T001 blocks T027, T002 blocks T027
- **Foundational (Phase 2)**: independent of Phase 1, but BLOCKS all user stories
- **US1 (Phase 3)**: depends on Phase 2 (compose/servers must stop using `llmsettings` before the package is deleted)
- **US2 (Phase 4)**: depends on Phase 2 (Router must send the task key) and Phase 1 (verified model IDs, container env)
- **US3 (Phase 5)**: depends on US2's config existing
- **Polish (Phase 6)**: depends on all desired stories

### User Story Dependencies

- **US1** and **US2** are independent of each other once Phase 2 is done — US1 is pure deletion (Go + TS + DB), US2 is pure configuration + one adapter file. Different developers can take them simultaneously.
- **US3** is a documentation/verification layer over US2's config.

### Within Each Story

- Tests before or alongside the implementation they cover (T006 with T005, T013–T014 before T020, T026 before T029)
- Backend deletions before dashboard deletions is *not* required — they touch disjoint files
- Migration (T015) before sqlc regeneration (T016) before package deletion (T017)
- Config rewrite (T027–T028) before any runtime validation (T030–T034)

### Parallel Opportunities

- Phase 1: T003 and T004 in parallel; T001 and T002 are independent investigations
- Phase 2: T006 and T009 in parallel with each other; T005 → T007 → T008 are sequential (same symbols)
- Phase 3: T018, T019, T020, T021, T022, T023 all touch different files and run in parallel after T017
- Phase 6: T038 and T039 in parallel

---

## Parallel Example: User Story 1

```bash
# After T017 (llmsettings package deleted), launch the surface removals together:
Task: "Remove Llm*/Cerebras* DTOs from apps/api/internal/dto/settings.go"
Task: "Remove Llm*/Cerebras* interfaces from packages/shared/src/index.ts and regenerate"
Task: "Delete LlmSettingsCard.tsx and drop the AI models tile from SettingsPage.tsx"
Task: "Remove the Llm hooks from apps/dashboard/src/features/settings/hooks.ts"
Task: "Remove settings.getLlm/putLlm/llmModels and the llmSettings query keys"
Task: "Genericise the Cerebras copy in apps/dashboard/src/features/status/StatusPage.tsx"
```

---

## Implementation Strategy

### MVP First

1. Phase 1 (Setup) — cheap, unblocks the config work
2. Phase 2 (Foundational) — static Router; **critical**, blocks everything
3. Phase 3 (US1) — the visible win: settings surface gone, nothing broken
4. **STOP and VALIDATE**: quickstart Scenario 1 plus a live match and generation

### Incremental Delivery

1. Setup + Foundational → backend routes by task key alone
2. US1 → dashboard and schema clean (MVP, demo-able)
3. US2 → free-tier-first chains with failover (the cost/robustness win)
4. US3 → one-file routing changes documented and proven
5. Polish → docs, dead-code sweep, full quickstart, `make test-lint`

### Risk Notes

- **T027 is the highest-risk task**: a model that lacks `json_object` support degrades to prose instead of failing over (research R3). Verify capability in T001 before writing the chain, and prove it in T030.
- **T017 will not compile** unless T010/T011 landed first — that ordering is why the compose/servers edits sit in Foundational rather than US1.
- Blanking keys in T031–T032 must recreate only the litellm container; recreating `api` also re-reads `GATEWAY_URL` and muddies the result.

---

## Notes

- [P] tasks touch different files with no incomplete dependencies
- Every phase ends at a green build: `go build ./... && go test ./...` for backend phases, `make test` for full checkpoints
- Commit per task or per logical group; the deletion tasks are individually revertible
- Deleted features take their tests with them — that is expected, not a coverage regression
