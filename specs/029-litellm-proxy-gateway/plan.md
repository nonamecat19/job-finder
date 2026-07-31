# Implementation Plan: LiteLLM Proxy Gateway for Multi-Provider LLM Routing

**Branch**: `029-litellm-proxy-gateway` | **Date**: 2026-07-31 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/029-litellm-proxy-gateway/spec.md`

## Summary

Deploy a LiteLLM proxy container alongside the existing stack that presents a single
OpenAI-compatible endpoint. The Go backend gains a new `GatewayProvider` (identical shape
to the existing Cerebras provider — OpenAI-compatible `/chat/completions`) pointed at the
proxy. The proxy's YAML config maps each task key (`match`, `generation`, `rephrase`,
`ghost`, `default`) to a specific provider+model (e.g., `openrouter/deepseek/deepseek-v4-pro`
for matching, `openrouter/qwen/qwen3.7-max` for generation). The existing Router and
llmsettings infrastructure is extended with a third `TaskProvider` value (`"gateway"`),
and the dashboard Settings screen gains a "Gateway" option alongside "Ollama" and
"Cerebras". Embeddings stay on local Ollama — the GatewayProvider delegates `Embed()` to
the Ollama provider exactly as Cerebras does.

Technical approach: add a `gateway` package under `infrastructure/` that implements
`domain.Provider` for chat only, reusing the Cerebras provider's HTTP patterns and error
classification. Extend `TaskProvider` with `TaskProviderGateway`. Add a `GATEWAY_URL`
config var. Wire the gateway provider into `composeLLM` alongside Ollama and Cerebras.
The LiteLLM container and its config file are added to `docker-compose.yml`.

## Technical Context

**Language/Version**: Go 1.2x (apps/api), TypeScript/React 18 + Vite (apps/dashboard),
YAML (LiteLLM proxy config)

**Primary Dependencies**: chi router, sqlc (pgx/v5), viper config, asynq;
TanStack Query, Tailwind, tygo (Go→TS types), `@job-finder/shared`;
LiteLLM proxy (Docker image: `ghcr.io/berriai/litellm:main-stable`)

**Storage**: PostgreSQL. No new tables — the existing `llm_task_setting` table already
stores per-task provider/model; we add a third provider value (`"gateway"`). No migration
needed (the `provider` column is `text`, not an enum).

**Testing**: `go test` (gateway provider unit tests, router tests with gateway leg);
`vitest` (Settings UI with gateway option); env-gated live smoke test (gated behind
`GATEWAY_URL` being reachable); `make test-lint` for cross-app.

**Target Platform**: Linux server via Docker Compose; single self-hosted operator

**Project Type**: Web application (Go API + React dashboard + LiteLLM proxy container)

**Performance Goals**: Gateway provider adds negligible latency beyond the proxy hop
(<200ms median). Proxy runs on the same Docker network as the API server. No change to
task throughput.

**Constraints**: Embeddings stay on local Ollama (FR-006). Existing Cerebras and Ollama
direct providers remain functional (FR-007). Gateway is opt-in — no `GATEWAY_URL` means
the gateway provider is nil and all tasks fall back to Ollama/Cerebras as before.

**Scale/Scope**: 1 operator, 5 task keys, 1 new Go provider package, 1 new Docker
service, 1 YAML config file, 1 new `TaskProvider` constant, dashboard Settings update.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply**: Unaffected — no submission path touched. ✅
- **II. Grounded Generation**: Unaffected — same prompts/pipeline; only the backing
  provider/model changes. Grounding guarantees are provider-independent. ✅
- **III. Typed Contracts Across Boundaries**: New DTO fields for the gateway provider
  value defined in `apps/api/internal/dto` and regenerated to
  `packages/shared/src/generated.ts` via tygo. No new DB schema. No hand-duplicated
  types. ✅ (enforced in tasks)
- **IV. Test Discipline Per Language**: `go test` for gateway provider + extended router;
  `vitest` for Settings UI with gateway option; env-gated live smoke test; existing
  Cerebras/Ollama tests must continue to pass. ✅
- **V. Local-First, Self-Hosted by Default**: ⚠ **Deviation, justified.** The LiteLLM
  proxy is a self-hosted container (Principle V compliant), but it routes to external
  hosted APIs (OpenRouter, Cerebras). This is compatible because it is strictly **opt-in
  and off by default** — fresh installs run 100% on local Ollama, and with no
  `GATEWAY_URL` configured the gateway provider is nil. The operator explicitly enables
  it by setting `GATEWAY_URL` and providing API keys in the proxy config. Embeddings
  remain local. See Complexity Tracking. ✅ (documented)

Gate result: **PASS** (one documented deviation, no unjustified violations).

## Project Structure

### Documentation (this feature)

```text
specs/029-litellm-proxy-gateway/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── gateway-config.md  # LiteLLM proxy config contract
└── tasks.md             # /speckit-tasks output (not created here)
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── platform/llm/
│   │   ├── infrastructure/
│   │   │   └── gateway/
│   │   │       ├── gateway.go          # NEW: GatewayProvider (OpenAI-compat to proxy)
│   │   │       └── gateway_test.go     # NEW: unit tests
│   │   ├── application/
│   │   │   └── router.go              # CHANGED: add TaskProviderGateway, gateway leg
│   │   └── llm.go                     # CHANGED: re-export GatewayProvider, NewGateway
│   ├── llmsettings/
│   │   └── domain/
│   │       └── types.go               # CHANGED: ErrInvalidProvider accepts "gateway"
│   ├── config/
│   │   ├── config.go                  # CHANGED: add GATEWAY_URL
│   │   └── defaults.go               # CHANGED: add GATEWAY_URL default (empty)
│   └── cmd/server/
│       └── compose.go                 # CHANGED: wire gateway provider into composeLLM
│
├── gateway/
│   └── config.yaml                    # NEW: LiteLLM proxy model routing config
│
├── docker-compose.yml                 # CHANGED: add litellm service
├── .env.example                       # CHANGED: add GATEWAY_URL, OPENROUTER_API_KEY

apps/dashboard/src/
├── features/settings/
│   ├── LlmSettingsCard.tsx            # CHANGED: add "Gateway" provider option
│   └── hooks.ts                       # CHANGED: handle gateway provider value

packages/shared/src/
├── generated.ts                       # REGEN via tygo
└── index.ts                           # CHANGED: add gateway provider constant
```

**Structure Decision**: Existing monorepo web-app layout. The gateway provider follows
the exact same pattern as `infrastructure/cerebras/` — same interface, same HTTP client
shape, same error classification. The LiteLLM proxy config lives at repo root
`gateway/config.yaml` and is mounted into the container. No new database migration —
the existing `llm_task_setting.provider` column is `text` and accepts any value.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| External AI provider routing (Principle V) | User requires per-task model selection across multiple providers (OpenRouter, Cerebras) for best quality/price | Single-provider approach (Cerebras only) cannot satisfy the spec's core value proposition: best model per task. The LiteLLM proxy is self-hosted (Principle V compliant) and the external providers are opt-in behind operator-supplied API keys. |
| Additional Docker service (LiteLLM proxy) | The proxy is the architectural foundation for multi-provider routing without application code changes | Embedding provider logic in the Go backend would require a new provider implementation for every external API (OpenRouter, Groq, etc.), violating the spec's goal of "change models without code changes". The proxy is the established pattern for this. |
