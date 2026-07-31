# Research: LiteLLM Proxy Gateway

**Feature**: 029-litellm-proxy-gateway
**Date**: 2026-07-31

## R1: LiteLLM Proxy Docker Deployment

**Decision**: Use `ghcr.io/berriai/litellm:main-stable` image, configured via a mounted `config.yaml` and environment variables for API keys.

**Rationale**:
- Official Docker image with health check support (`/health/liveliness` endpoint)
- Configuration via mounted YAML file + env vars — no database needed for our use case (we don't need the admin UI's model-in-DB feature)
- The proxy presents an OpenAI-compatible `/v1/chat/completions` endpoint on port 4000
- No external dependencies beyond the container itself (no separate DB, no Redis)

**Alternatives considered**:
- Running LiteLLM as a Python process directly — rejected: adds Python dependency to the stack; Docker is already the deployment model
- Using the full docker-compose from LiteLLM (with PostgreSQL, Prometheus) — rejected: overkill for single-operator use; we only need the proxy, not the admin DB
- OpenRouter direct API (no proxy) — rejected: spec requires multi-provider routing through one endpoint; OpenRouter alone doesn't cover Cerebras direct access

**Key configuration pattern**:
```yaml
model_list:
  - model_name: match
    litellm_params:
      model: openrouter/deepseek/deepseek-v4-pro
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: generation
    litellm_params:
      model: openrouter/qwen/qwen3.7-max
      api_key: os.environ/OPENROUTER_API_KEY
```

The `model_name` is what the Go backend sends as the `model` field in its OpenAI-compatible request. LiteLLM resolves it to the actual provider+model.

## R2: Error Mapping from LiteLLM Proxy to Go Sentinel Errors

**Decision**: The gateway Go provider reuses the existing `classifyProviderError` function from `cerebras/errors.go` (which is already provider-agnostic — it takes a `provider` string parameter). The gateway provider classifies HTTP status codes from the proxy the same way Cerebras does.

**Rationale**:
- LiteLLM proxy returns standard HTTP status codes from the underlying provider (429 for rate limits, 401/403 for auth failures, 402 for payment required, 5xx for provider unavailability)
- The existing `classifyProviderError(provider, status, body)` function in `cerebras/errors.go` already handles all these cases generically
- The gateway provider can call this function directly (it's in the same `cerebras` package, or we can extract it to a shared location)
- The proxy's error response body follows OpenAI format: `{"error": {"message": "..."}}`, which `providerErrMessage` already parses

**Alternatives considered**:
- Duplicating error classification in the gateway package — rejected: violates DRY; the taxonomy is identical
- Extracting error classification to a shared `llmerrors` package — considered but adds refactoring scope; the gateway provider can import `cerebras.classifyProviderError` directly since it's already a public function

**Decision update**: Extract `classifyProviderError`, `providerErrMessage`, `rateLimitBreaker`, and the sentinel errors to a new `infrastructure/shared/` package so both `cerebras` and `gateway` can use them without circular imports. The `cerebras` package re-exports them for backward compatibility.

## R3: Gateway Provider Implementation Pattern

**Decision**: The gateway provider follows the exact same pattern as `cerebras.Provider` — implements `domain.Provider`, handles `Complete`/`CompleteJSON` via OpenAI-compatible `/chat/completions`, delegates `Embed` to Ollama.

**Rationale**:
- The Cerebras provider is already an OpenAI-compatible client — the gateway provider is identical except for the base URL and auth header
- Both use the same `chatRequest`/`chatResponse` structs, same HTTP client pattern, same error classification
- The only differences: (a) base URL points to the proxy, (b) auth header uses a configurable API key (the proxy's master key, not a provider key), (c) no per-provider circuit breaker (the proxy handles rate limiting internally)

**Key differences from Cerebras provider**:
- No `rateLimitBreaker` — the proxy handles rate limiting and fallback internally
- No `modelName` field — the model is always passed per-request via `CompleteOptions.Model` (set by the Router from the task setting)
- The `apiKey` is the LiteLLM master key (`LITELLM_MASTER_KEY`), not a provider-specific key

**Alternatives considered**:
- Making the Cerebras provider generic enough to handle both — rejected: the Cerebras provider has Cerebras-specific behavior (circuit breaker, model list validation) that doesn't apply to the gateway
- Using a shared base struct with provider-specific wrappers — rejected: over-engineering for two nearly-identical providers; duplication is acceptable at this scale

## R4: Router Extension for Third Provider

**Decision**: Add `TaskProviderGateway = "gateway"` to the existing `TaskProvider` enum. The `Router` gains a third provider leg (`gateway domain.Provider`). The `resolve()` method checks the task setting and dispatches to the matching provider.

**Rationale**:
- The Router already supports two providers (Ollama, Cerebras) with a clean dispatch pattern
- Adding a third follows the same pattern: new constant, new field, new case in `resolve()`
- The `llmsettings` domain already validates provider values against known constants — adding `"gateway"` is a one-line change
- No database migration needed — the `provider` column is `text`, not an enum

**Changes needed**:
1. `application/router.go`: Add `TaskProviderGateway`, add `gateway` field to `Router`, extend `resolve()`
2. `llmsettings/domain/types.go`: Update `ErrInvalidProvider` message to include `"gateway"`
3. `llm.go` facade: Re-export `TaskProviderGateway`, add `GatewayProvider` type alias
4. `cmd/server/compose.go`: Wire gateway provider into `composeLLM`

**Alternatives considered**:
- Making the Router fully dynamic (provider registry pattern) — rejected: over-engineering for 3 providers; the current static dispatch is simple and testable
- Using a single "remote" provider that the proxy abstracts — rejected: the spec requires existing direct providers to remain selectable alongside the gateway

## R5: LiteLLM Proxy Configuration for Per-Task Models

**Decision**: The proxy config maps each task key to a specific provider+model. The Go backend sends the task key as the `model` field in the chat completion request. The proxy resolves it.

**Rationale**:
- The Router already sets `CompleteOptions.Model` to the task's configured model name
- When the task is set to provider `"gateway"` with model `"match"`, the Router dispatches to the gateway provider with `Model: "match"`
- The gateway provider sends `{"model": "match", ...}` to the proxy
- The proxy's `model_list` maps `"match"` → `openrouter/deepseek/deepseek-v4-pro`

**Config file structure** (`gateway/config.yaml`):
```yaml
general_settings:
  master_key: os.environ/LITELLM_MASTER_KEY

model_list:
  - model_name: match
    litellm_params:
      model: openrouter/deepseek/deepseek-v4-pro
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: generation
    litellm_params:
      model: openrouter/qwen/qwen3.7-max
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: rephrase
    litellm_params:
      model: openrouter/openai/gpt-4o-mini
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: ghost
    litellm_params:
      model: openrouter/deepseek/deepseek-v4-pro
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: default
    litellm_params:
      model: openrouter/openai/gpt-4o-mini
      api_key: os.environ/OPENROUTER_API_KEY

litellm_settings:
  drop_params: true
  set_verbose: false
```

**Fallback chains** (optional, per spec FR-005):
```yaml
  - model_name: match
    litellm_params:
      model: openrouter/deepseek/deepseek-v4-pro
      api_key: os.environ/OPENROUTER_API_KEY
      fallbacks:
        - cerebras/gpt-oss-120b
```

## R6: Embeddings Bypass

**Decision**: The gateway provider's `Embed()` method delegates to the Ollama provider, exactly as Cerebras does. The Router's `Embed()` already always routes to Ollama regardless of the task's chat provider.

**Rationale**:
- The Router's `Embed()` method (line 147 of `router.go`) already hardcodes `r.ollama.Embed(ctx, text)` — it never dispatches to Cerebras
- The gateway provider follows the same pattern: `Embed()` calls the injected Ollama provider
- No changes needed to the Router or any embedding call site
- This satisfies FR-006 without any new code

## R7: Dashboard Settings Integration

**Decision**: Add `"gateway"` as a third provider option in the Settings UI's per-task provider selector. When gateway is selected, the model field is set to the task key name (e.g., `"match"`), which the proxy resolves.

**Rationale**:
- The existing Settings UI already has a provider dropdown per task (Ollama / Cerebras)
- Adding a third option follows the same pattern
- The model field for gateway tasks is the task key name — the operator doesn't need to see proxy-level model names in the dashboard (those are in the proxy config)
- The `llmsettings` PUT endpoint already accepts arbitrary provider strings (validated against known constants)

**Changes needed**:
1. `LlmSettingsCard.tsx`: Add "Gateway" to the provider dropdown options
2. `hooks.ts`: Handle `"gateway"` provider value in the update mutation
3. `packages/shared/src/index.ts`: Add gateway provider constant for the frontend
