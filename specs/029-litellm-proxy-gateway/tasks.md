# Tasks: LiteLLM Proxy Gateway for Multi-Provider LLM Routing

**Feature**: 029-litellm-proxy-gateway | **Branch**: `029-litellm-proxy-gateway`
**Generated**: 2026-07-31 | **Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

## Task Summary

| Phase | Story | Task Count | Description |
|---|---|---|---|
| Phase 1 | — | 4 | Setup: config, env, gateway YAML |
| Phase 2 | — | 4 | Foundational: error extraction, gateway provider |
| Phase 3 | US1 (P1) | 7 | Single endpoint for all LLM tasks |
| Phase 4 | US2 (P1) | 2 | Per-task model selection via gateway config |
| Phase 5 | US5 (P1) | 2 | Embeddings remain on local Ollama |
| Phase 6 | US3 (P2) | 2 | Provider fallback on failure |
| Phase 7 | US4 (P3) | 1 | Cost visibility |
| Phase 8 | — | 2 | Polish & validation |
| **Total** | | **24** | |

---

## Phase 1: Setup (Infrastructure & Configuration)

**Goal**: Add config vars, env vars, and the LiteLLM proxy config file. No code changes yet.

- [X] T001 Add `GATEWAY_URL` field to `Config` struct in `apps/api/internal/config/config.go` (mapstructure tag, after existing Cerebras fields)
- [X] T002 Add `"GATEWAY_URL": ""` default entry to `defaults` map in `apps/api/internal/config/defaults.go`
- [X] T003 [P] Add `GATEWAY_URL`, `LITELLM_MASTER_KEY`, and `OPENROUTER_API_KEY` env vars to `.env.example` (in the LLM section, after Cerebras vars)
- [X] T004 [P] Create `gateway/config.yaml` with the production model mapping per `contracts/gateway-config.md` (all 5 task keys mapped to OpenRouter models, with fallback chains for match/generation/ghost)

---

## Phase 2: Foundational (Error Extraction & Gateway Provider)

**Goal**: Extract shared error classification so both `cerebras` and `gateway` can use it. Implement the gateway provider. These tasks block all user stories.

- [X] T005 Extract `classifyProviderError`, `providerErrMessage`, `rateLimitBreaker`, `rateLimitCooldown`, `maxRateLimitCooldown`, `retryAfter`, and all 6 sentinel errors + `Terminal`/`Retryable` predicates from `apps/api/internal/platform/llm/infrastructure/cerebras/errors.go` into new package `apps/api/internal/platform/llm/infrastructure/shared/errors.go`
- [X] T006 Update `apps/api/internal/platform/llm/infrastructure/cerebras/errors.go` to import and re-export from `shared/` package (preserve backward compatibility for all existing call sites)
- [X] T007 [P] Create `apps/api/internal/platform/llm/infrastructure/gateway/gateway.go` — implement `domain.Provider` interface (chat only, `Embed` delegates to Ollama), OpenAI-compatible HTTP client to proxy endpoint, using `shared.classifyProviderError` for error mapping, no circuit breaker
- [X] T008 [P] Create `apps/api/internal/platform/llm/infrastructure/gateway/gateway_test.go` — unit tests for `Complete`, `CompleteJSON`, `Embed` delegation, error classification (429/401/402/5xx), connection refused handling

---

## Phase 3: User Story 1 — Single Endpoint for All LLM Tasks (P1)

**Goal**: The gateway provider is wired into the Router and selectable from dashboard Settings. All 5 task types can route through the gateway. The LiteLLM container starts with `docker compose up`.

**Independent Test**: Configure the gateway with one provider, point the application at the gateway endpoint, trigger any AI task, and confirm the task completes through the gateway.

### Router & Facade

- [X] T009 [US1] Add `TaskProviderGateway TaskProvider = "gateway"` constant to `apps/api/internal/platform/llm/application/router.go` (alongside existing `TaskProviderOllama`/`TaskProviderCerebras`)
- [X] T010 [US1] Extend `Router` struct in `apps/api/internal/platform/llm/application/router.go` — add `gateway domain.Provider` field, update `NewRouter` signature to accept gateway param, extend `resolve()` with `case TaskProviderGateway` dispatch (fall back to Ollama when gateway is nil)
- [X] T011 [US1] Update `apps/api/internal/platform/llm/llm.go` facade — add `GatewayProvider = gateway.Provider` type alias, add `NewGateway = gateway.New` constructor, re-export `TaskProviderGateway`
- [X] T012 [US1] Update `apps/api/internal/llmsettings/domain/types.go` — change `ErrInvalidProvider` message to include `"gateway"` (from `"ollama\" or \"cerebras"` to `"ollama\", \"cerebras\", or \"gateway"`)

### Wiring

- [X] T013 [US1] Update `apps/api/cmd/server/compose.go` — in `composeLLM`, construct gateway provider when `GATEWAY_URL` is set (nil otherwise), pass it to all 5 `NewRouter` calls, add `GatewayURL` to `Config` usage
- [X] T014 [US1] Add `litellm` service to `docker-compose.yml` — `ghcr.io/berriai/litellm:main-stable` image, mount `./gateway/config.yaml:/app/config.yaml`, pass `LITELLM_MASTER_KEY` and `OPENROUTER_API_KEY` env vars, health check on `/health/liveliness`, port 4000, depends_on none (proxy is independent)

### Dashboard

- [X] T015 [US1] Update `apps/dashboard/src/features/settings/LlmSettingsCard.tsx` — add "Gateway" option to the per-task provider dropdown, set model to task key name when gateway is selected
- [X] T016 [US1] Update `packages/shared/src/index.ts` — add `"gateway"` provider constant for frontend type safety

---

## Phase 4: User Story 2 — Per-Task Model Selection via Gateway Configuration (P1)

**Goal**: Each task key routes to its assigned model in the proxy config. Changing the config and reloading takes effect without application restart.

**Independent Test**: Configure different models for "match" and "generation", trigger both tasks, verify each used its assigned model.

- [X] T017 [US2] Verify per-task routing end-to-end — trigger each of the 5 task types through the gateway and confirm proxy logs show the correct provider+model per `contracts/gateway-config.md`
- [X] T018 [US2] Test config hot reload — edit `gateway/config.yaml` to change a model mapping, run `docker compose restart litellm`, trigger the task, verify the new model is used

---

## Phase 5: User Story 5 — Embeddings Remain on Local Ollama (P1)

**Goal**: Embedding calls never touch the gateway. They go directly to local Ollama regardless of which provider is selected for chat tasks.

**Independent Test**: Trigger a job matching run and verify embedding calls go to local Ollama while chat calls go through the gateway.

- [X] T019 [US5] Verify `GatewayProvider.Embed()` in `apps/api/internal/platform/llm/infrastructure/gateway/gateway.go` delegates to the injected Ollama provider (no HTTP call to proxy)
- [X] T020 [US5] Verify `Router.Embed()` in `apps/api/internal/platform/llm/application/router.go` still hardcodes `r.ollama.Embed()` — no change needed, confirm no regression

---

## Phase 6: User Story 3 — Provider Fallback on Failure (P2)

**Goal**: When the primary provider fails, the gateway falls back to the next provider in the chain.

**Independent Test**: Configure a primary that fails and a fallback that works, trigger a task, verify fallback succeeds.

- [X] T021 [US3] Add `fallbacks` entries to `gateway/config.yaml` for `match`, `generation`, and `ghost` model entries (fallback to `cerebras/gpt-oss-120b`) per `contracts/gateway-config.md` example
- [X] T022 [US3] Test fallback behavior — temporarily use an invalid OpenRouter key, trigger a task with fallback configured, verify the request falls through to Cerebras and completes

---

## Phase 7: User Story 4 — Cost Visibility (P3)

**Goal**: The operator can see per-task and per-model spending.

**Independent Test**: Trigger several AI tasks, check the gateway's cost tracking.

- [X] T023 [US4] Document cost tracking access — add a note in `docs/docs/ai/llm-abstraction.md` (or the gateway config contract) explaining how to view spend via `docker compose logs litellm` (LiteLLM logs token counts and cost per request) and via the proxy's `/spend/logs` endpoint

---

## Phase 8: Polish & Cross-Cutting Concerns

**Goal**: Ensure no regressions, all tests pass, quickstart validation succeeds.

- [X] T024 Run `make test-lint` — confirm all Go tests pass (gateway provider, router with gateway leg, existing Cerebras/Ollama tests), ESLint passes, no regressions
- [X] T025 Run the quickstart validation checklist from `quickstart.md` — verify all 9 items pass end-to-end

---

## Dependencies

```
Phase 1 (Setup) ─────────────────────────────────────────────────────────────┐
     │                                                                        │
     ▼                                                                        │
Phase 2 (Foundational: error extraction + gateway provider) ──┐               │
     │                                                        │               │
     ▼                                                        │               │
Phase 3 (US1: Single endpoint) ◄──────────────────────────────┘               │
     │                                                                        │
     ├──▶ Phase 4 (US2: Per-task model selection) ── depends on US1          │
     ├──▶ Phase 5 (US5: Embeddings stay local) ── depends on US1             │
     │                                                                        │
     ▼                                                                        │
Phase 6 (US3: Fallback) ── depends on US1+US2                                │
     │                                                                        │
     ▼                                                                        │
Phase 7 (US4: Cost visibility) ── depends on US1                             │
     │                                                                        │
     ▼                                                                        │
Phase 8 (Polish) ── depends on all                                           │
```

- **US1 blocks US2, US5, US3, US4** — the gateway must be wired before per-task selection, embeddings verification, fallback, or cost tracking can be tested
- **US2 and US5 are independent** — can be implemented in parallel after US1
- **US3 depends on US2** — fallback chains are configured per model mapping
- **US4 is independent of US2/US3/US5** — cost tracking works as soon as US1 is done

## Parallel Execution Opportunities

### Within Phase 1 (all independent)
```
T001 ─┬─ T003 (different files)
T002 ─┤
T004 ─┘
```

### Within Phase 2
```
T005 → T006 (T006 depends on T005)
T007 → T008 (T008 depends on T007)
T005+T006 and T007+T008 can run in parallel (different packages)
```

### Within Phase 3
```
T009 → T010 → T011 (sequential: router → facade)
T012 (independent of T009-T011, different package)
T013 (depends on T011)
T014 (independent of Go changes, different file)
T015 → T016 (dashboard → shared types)
T013+T014 and T015+T016 can run in parallel
```

### After Phase 3
```
Phase 4 (US2) ─┬─ parallel
Phase 5 (US5) ─┘
```

## Implementation Strategy

### MVP (User Stories 1 + 2 + 5, all P1)

1. **Phase 1** (T001-T004): Config and gateway YAML — ~15 min
2. **Phase 2** (T005-T008): Error extraction + gateway provider — ~45 min
3. **Phase 3** (T009-T016): Wire everything together — ~45 min
4. **Phase 4** (T017-T018): Verify per-task routing — ~15 min
5. **Phase 5** (T019-T020): Verify embeddings bypass — ~10 min
6. **Phase 8** (T024-T025): Test and validate — ~15 min

**MVP total: ~2.5 hours**

### Full Feature

Add Phase 6 (US3, fallback) and Phase 7 (US4, cost visibility) — ~30 min additional.

**Full total: ~3 hours**
