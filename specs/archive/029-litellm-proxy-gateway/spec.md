> **ARCHIVED — SHIPPED — FR-007 revoked by 030.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/llm-routing.md`](../../domains/llm-routing.md) — read that first.
>
> **FR-007 (Cerebras and Ollama remain selectable in dashboard Settings) is void — 030-FR-001 removed all dashboard provider/model controls.**

---
# Feature Specification: LiteLLM Proxy Gateway for Multi-Provider LLM Routing

**Feature Branch**: `029-litellm-proxy-gateway`

**Created**: 2026-07-31

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "implement LiteLLM proxy as a self-hosted gateway that routes LLM requests to different providers (OpenRouter, Cerebras, etc.) per task, replacing the current direct provider approach with a single OpenAI-compatible endpoint"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Single Endpoint for All LLM Tasks (Priority: P1)

As the operator of the self-hosted job-finder, I want all AI tasks (matching, generation, rephrase, ghost-job, salary, outreach, recruiter) to go through a single gateway endpoint, so that I can add, remove, or swap model providers without changing application code or restarting the stack.

**Why this priority**: This is the architectural foundation. Every other story (per-task model selection, cost tracking, fallback) depends on having a unified gateway that the application talks to. Without it, the operator must manage multiple provider integrations in application code.

**Independent Test**: Configure the gateway with one provider (e.g., Cerebras), point the application at the gateway endpoint, trigger any AI task, and confirm the task completes through the gateway. The application code has no knowledge of which provider served the request.

**Acceptance Scenarios**:

1. **Given** the gateway is running with at least one provider configured, **When** the application sends a chat completion request to the gateway endpoint, **Then** the gateway routes it to the configured provider and returns the response in OpenAI-compatible format.
2. **Given** the gateway is running, **When** the application sends an embeddings request, **Then** the gateway routes it to the configured embeddings provider (Ollama) and returns the response.
3. **Given** the gateway is not running or unreachable, **When** the application sends a request, **Then** the application returns a clear error indicating the gateway is unavailable, and no AI task silently hangs.
4. **Given** the gateway is running with multiple providers configured, **When** the application requests a model by name (e.g., "match", "generation"), **Then** the gateway routes to the provider assigned to that model name without the application specifying which provider to use.

---

### User Story 2 - Per-Task Model Selection via Gateway Configuration (Priority: P1)

As the operator, I want to assign each AI task to a specific model from any provider through a single configuration file, so that I can use the best quality/price model for each task (e.g., DeepSeek for matching, Qwen for generation, GPT-4o-mini for light tasks) without changing application code.

**Why this priority**: The primary motivation for the gateway is per-task model optimization. Without this, the gateway is just an unnecessary indirection layer. This story delivers the core value proposition: best model per task at minimal cost.

**Independent Test**: Configure the gateway with different models for "match" (DeepSeek V4 Pro via OpenRouter) and "generation" (Qwen 3.7 Max via OpenRouter), trigger both tasks, and verify each used its assigned model.

**Acceptance Scenarios**:

1. **Given** the gateway config maps "match" to `openrouter/deepseek/deepseek-v4-pro` and "generation" to `openrouter/qwen/qwen3.7-max`, **When** the application requests model "match", **Then** the gateway routes to DeepSeek V4 Pro via OpenRouter.
2. **Given** the gateway config maps "light-tasks" to `openrouter/openai/gpt-4o-mini`, **When** the application requests model "light-tasks" for salary estimation, outreach, recruiter extraction, and keyword rephrase, **Then** all four task types use GPT-4o-mini via OpenRouter.
3. **Given** the operator edits the gateway config to change "match" from DeepSeek to a different model, **When** the gateway reloads its configuration, **Then** subsequent match requests use the new model without an application restart.
4. **Given** the gateway config maps a model name to a provider that requires an API key, **When** the key is missing or invalid, **Then** the gateway returns a clear authentication error and the application surfaces it to the operator.

---

### User Story 3 - Provider Fallback on Failure (Priority: P2)

As the operator, I want the gateway to automatically fall back to an alternative provider when the primary provider fails (rate limit, downtime, model unavailable), so that AI tasks continue to complete without manual intervention.

**Why this priority**: Improves reliability but is not required to demonstrate the core value. The existing Cerebras provider already has a circuit breaker; this extends that pattern to the gateway level with provider-level fallback.

**Independent Test**: Configure a primary provider that will fail (e.g., wrong API key) and a fallback provider that works, trigger a task, and verify the gateway tries the primary, fails, then succeeds with the fallback.

**Acceptance Scenarios**:

1. **Given** the gateway config specifies a primary model and a fallback model for "match", **When** the primary provider returns a 429 (rate limit) or 5xx error, **Then** the gateway retries with the fallback model and returns the fallback's response.
2. **Given** the gateway config specifies a primary model with no fallback, **When** the primary provider fails, **Then** the gateway returns the original error to the application (no silent failure, no infinite retry).
3. **Given** the primary provider succeeds after a previous failure, **When** the next request arrives, **Then** the gateway routes to the primary again (fallback is not sticky).

---

### User Story 4 - Cost Visibility (Priority: P3)

As the operator, I want to see how much each AI task costs in token usage and dollar amount, so that I can monitor spending and adjust model assignments if costs exceed expectations.

**Why this priority**: Cost tracking is valuable for ongoing operation but not required for initial deployment. The gateway can be used without cost visibility; this adds operational insight.

**Independent Test**: Trigger several AI tasks, then check the gateway's cost dashboard or logs to see per-task and per-model spending.

**Acceptance Scenarios**:

1. **Given** the gateway has processed AI tasks, **When** the operator views the gateway's cost tracking interface, **Then** they see total spend, spend per model, and spend per task type.
2. **Given** the gateway processes a request, **When** the response is returned, **Then** the response headers include token usage (prompt tokens, completion tokens) and estimated cost.

---

### User Story 5 - Embeddings Remain on Local Ollama (Priority: P1)

As the operator, I want embeddings (profile and job text vectorization) to continue running on my local Ollama instance, not through the gateway's paid providers, so that embedding costs stay at zero and embedding latency stays low.

**Why this priority**: Embeddings are high-volume (every job match, every profile update) and the `nomic-embed-text` model runs well locally. Routing embeddings through paid providers would add significant cost with no quality benefit. This is a hard constraint, not an optimization.

**Independent Test**: Trigger a job matching run and verify that the embedding calls go directly to local Ollama while the chat completion calls go through the gateway.

**Acceptance Scenarios**:

1. **Given** the gateway is configured as the chat provider, **When** the application needs text embeddings, **Then** the embedding request goes directly to the local Ollama endpoint, bypassing the gateway entirely.
2. **Given** the local Ollama is unreachable, **When** the application needs embeddings, **Then** the embedding call fails with a clear error and does not fall back to the gateway or any paid provider.

---

### Edge Cases

- What happens when the gateway configuration file has a syntax error? The gateway must reject the invalid config on startup or reload and continue serving with the last known good configuration, logging the error clearly.
- What happens when a model name requested by the application is not defined in the gateway config? The gateway returns a clear "model not found" error; the application surfaces it to the operator.
- What happens when the gateway is under load with concurrent AI tasks? The gateway must handle the configured `AI_CONCURRENCY_CLOUD` level of concurrent requests without queueing or dropping requests.
- What happens when an OpenRouter credit balance runs out mid-request? The gateway receives a 402 error from OpenRouter and surfaces it as an `ErrInsufficientCredits` equivalent.
- What happens when the operator wants to keep using Cerebras directly (bypassing the gateway) for some tasks? The existing Cerebras provider remains in the codebase; the gateway is an additional provider option, not a replacement. The operator can choose per task whether to route through the gateway or use a direct provider.
- What happens when the gateway container is restarted while the application is running? The application's HTTP client must handle connection refused/timeout gracefully and retry on the next request (no persistent connection state).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST include a LiteLLM proxy gateway deployed as a container alongside the existing stack, presenting an OpenAI-compatible chat completions endpoint.
- **FR-002**: The gateway MUST support routing chat completion requests to multiple providers (OpenRouter, Cerebras, and any OpenAI-compatible provider) based on the requested model name, configured via a YAML file.
- **FR-003**: The application MUST include a new LLM provider implementation that speaks OpenAI-compatible protocol to the gateway endpoint, analogous to the existing Cerebras provider but with the gateway URL as its target.
- **FR-004**: The gateway configuration MUST allow per-task model assignment: each of the five task keys (match, generation, rephrase, ghost, default) maps to a specific provider and model ID.
- **FR-005**: The gateway MUST support optional fallback chains per model: if the primary provider fails with a retryable error (5xx, 429), the gateway tries the next provider in the chain.
- **FR-006**: Embedding requests MUST continue to go directly to the local Ollama instance, bypassing the gateway entirely. The gateway provider implementation MUST NOT handle embeddings.
- **FR-007**: The existing Cerebras and Ollama direct providers MUST remain functional and selectable in dashboard Settings alongside the new gateway provider. The operator chooses per task whether to use a direct provider or the gateway.
- **FR-008**: The gateway MUST expose cost tracking: total spend, per-model spend, and per-request token counts, accessible via the LiteLLM admin UI or API.
- **FR-009**: The gateway configuration MUST be reloadable without restarting the gateway container (hot reload on config file change or via API call).
- **FR-010**: The application's gateway provider MUST surface gateway errors (connection refused, model not found, provider auth failure, rate limit) as the existing sentinel error types so that the queue middleware and error handling behave consistently.
- **FR-011**: The gateway container MUST be included in the project's Docker Compose configuration so that `docker compose up` starts it alongside the existing services.
- **FR-012**: The gateway MUST NOT retain or log prompt/response data (zero data retention), consistent with the project's privacy requirements.

### Key Entities

- **Gateway Provider**: A new Go provider implementation (`internal/platform/llm/infrastructure/gateway/`) that implements the `llm.Provider` interface for chat completions only, speaking OpenAI-compatible protocol to the LiteLLM proxy endpoint.
- **Gateway Configuration**: A YAML file (`gateway/config.yaml`) defining model mappings (model name → provider + model ID), API keys, and fallback chains. Mounted into the gateway container as a volume.
- **Model Mapping**: A named entry in the gateway config that the application references by task key (e.g., "match", "generation"). The gateway resolves this to a specific provider and model ID.
- **Fallback Chain**: An ordered list of alternative model mappings that the gateway tries sequentially when the primary fails with a retryable error.

## Success Criteria *(mandatory)*

- **SC-001**: The operator can change which model serves a given AI task by editing a single YAML file and triggering a reload, with no application restart or code change required.
- **SC-002**: All five AI task types (match, generation, rephrase, ghost, default) complete successfully through the gateway with at least one provider configured.
- **SC-003**: When the primary provider for a task fails with a retryable error, the gateway falls back to the next provider in the chain and the task completes successfully (assuming at least one provider in the chain is healthy).
- **SC-004**: Embedding calls never touch the gateway; they go directly to local Ollama with no change in latency or behavior from the current implementation.
- **SC-005**: The operator can view per-task and per-model spending in the gateway's admin interface within 30 seconds of a request completing.
- **SC-006**: The gateway adds no more than 200ms of additional latency (median) compared to calling the underlying provider directly.
- **SC-007**: The existing Cerebras and Ollama direct providers continue to work exactly as they do today; no existing functionality is regressed.

## Assumptions

- The LiteLLM proxy container will run on the same Docker network as the API server, accessible at a fixed hostname (e.g., `http://litellm:4000`).
- OpenRouter will be the primary model provider accessed through the gateway, with Cerebras as an optional fallback or alternative.
- The gateway provider in Go will reuse the existing HTTP client patterns from the Cerebras provider (connection pooling, timeouts, error classification).
- The LiteLLM proxy's built-in cost tracking is sufficient; no custom cost tracking infrastructure is needed in the application.
- The operator is responsible for obtaining and managing API keys for each provider (OpenRouter, Cerebras, etc.) and configuring them in the gateway's environment or config file.
- The existing `AI_CONCURRENCY_CLOUD` setting applies to gateway-routed tasks (they count as cloud tasks for concurrency purposes).
- The dashboard Settings screen will gain a "Gateway" provider option alongside "Ollama" and "Cerebras" for each task, with model selection deferred to the gateway config (the dashboard does not need to list gateway models).
