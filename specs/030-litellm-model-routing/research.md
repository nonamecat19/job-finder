# Phase 0 Research: Gateway-Owned Model Routing

## R1 — How LiteLLM expresses ordered failover

**Decision**: Model *groups* named after task keys, with ordered failover declared in `litellm_settings.fallbacks` as `{<group>: [<group>, <group>, ...]}`. Each tier is its own single-deployment group.

**Rationale**: Listing several deployments under one `model_name` makes them a load-balanced pool (default `simple-shuffle`), not a priority order — that would send traffic to OpenRouter while free tiers are healthy, violating FR-006. Fallbacks between groups are evaluated in list order and only on failure, which is exactly the requested semantics. The existing `gateway/config.yaml` puts `fallbacks` *inside* `litellm_params`, which is not the documented location and must be corrected during implementation.

**Alternatives considered**:
- Multi-deployment groups + `routing_strategy: usage-based-routing-v2` — approximates "cheapest first" but is quota/TPM driven and non-deterministic; harder to reason about and to test.
- Failover in the Go client (try gateway group A, then B) — puts model knowledge back in the app, directly contradicting the feature's goal.

**Implementation note**: verify against the pinned `main-stable` image at implementation time (`litellm --config gateway/config.yaml --detailed_debug` locally); if the running image expects a different fallbacks key shape, the contract in `contracts/gateway-config.md` is what must hold, not the literal YAML sketch.

**Verified 2026-07-31** against `ghcr.io/berriai/litellm:main-stable` (digest tagged `28b10a635296` locally) with a scratch three-tier config (`match` → `match-groq` → `local`): the proxy loaded `litellm_settings.fallbacks=[{'match': ['match-groq', 'local']}]` without error, confirming the list-of-single-key-dicts shape under `litellm_settings` (not inside `litellm_params`) is accepted by this image. A live request with a bad Cerebras key advanced to Groq automatically (`x-litellm-attempted-fallbacks: 1`, `x-litellm-model-group: match-groq`), and a request naming an unconfigured group returned HTTP 400 as required by contract C1.

## R2 — Missing provider credentials must not break startup (FR-011)

**Decision**: Always define every provider key in the litellm service environment with an empty default (`GROQ_API_KEY: ${GROQ_API_KEY:-}`), so `os.environ/GROQ_API_KEY` always resolves. A blank key then fails that deployment at request time with an auth error, and the fallback chain advances.

**Rationale**: LiteLLM resolves `os.environ/X` when loading config; an entirely undefined variable can abort startup or produce confusing errors, whereas an empty string is a normal auth failure the failover path already handles. This converts "missing credential" into "skipped tier" behaviourally, satisfying FR-007 and FR-011 without special-casing.

**Alternatives considered**: conditionally templating the config per environment (extra tooling, another moving part); pre-flight key validation in the Go app (reintroduces provider knowledge in the backend).

**Cost**: one wasted round trip per absent key on the first request of a chain. Bounded by `cooldown_time` — after `allowed_fails` failures the deployment is cooled down and skipped outright.

## R3 — JSON mode across free-tier providers vs `drop_params`

**Decision**: Keep `drop_params: true`, but only place models that support `response_format: {"type":"json_object"}` in the chains of JSON-consuming tasks (`match`, `ghost`, `rephrase`, and structured `generation` calls). Verify support per model during implementation; record verified model IDs in a comment block in `gateway/config.yaml`.

**Rationale**: `drop_params: true` silently strips unsupported params. A model without JSON mode would then return prose, `CompleteStructured` would burn its 3 attempts, and the task fails — a slow, confusing failure that never triggers gateway failover (a 200 with bad content is not a provider error). Selecting JSON-capable models up front is the only place this can be prevented.

**Alternatives considered**: `drop_params: false` — surfaces the mismatch loudly, but a single unsupported param then hard-fails the tier instead of degrading; rejected because litellm normalises other params we rely on. Prompt-only JSON coercion — weaker and already the fallback path inside `CompleteStructured`.

## R4 — Observability: which provider actually served the request (FR-012)

**Decision**: The gateway adapter reads the `x-litellm-model-name` response header as the primary source for the served model, falling back to the body's `model` field, then to `"unknown"`. It logs one structured line per request: task key, requested group, served model, latency, outcome.

**Verified 2026-07-31**: against the pinned image, the body `model` field is **not** reliably the resolved deployment model — on a primary-tier hit (no fallback triggered) the body echoed the *requested group name* (`"match"`), only turning into the resolved model string once a fallback fired. The `x-litellm-model-name` header, by contrast, always named the actual resolved model (e.g. `cerebras/gpt-oss-120b`, `groq/llama-3.3-70b-versatile`) on both the primary and fallback paths; `x-litellm-model-id` is a stable but opaque hash, not a human-readable name, so it is logged as a secondary correlation field, not the primary served-model value. This supersedes the original decision text (and contracts/task-router.md §C4), which assumed the body field alone was authoritative.

**Rationale**: Needs no extra call, no proxy admin API, and no DB. It gives an operator a grep-able answer within seconds, satisfying SC-006. Today's adapter discards the field entirely.

**Alternatives considered**: querying the proxy's `/spend/logs` or admin UI (external dependency for a debugging need); a new dashboard surface (explicitly out of scope).

## R5 — Admission-gate provider class without per-task settings (FR-013)

**Decision**: `Router.ProviderClass()` returns `hosted` when the router was constructed with a gateway provider, otherwise it delegates to `ollama.IsHosted()` (existing loopback/private-host + API-key heuristic). The class is fixed at construction.

**Rationale**: `queue.ClassResolver` only needs local-vs-hosted to size concurrency (019-ai-job-throughput). A gateway-routed task is hosted by definition — its first four tiers are remote and even the local tier is reached over the proxy. Nothing about the class depends on which upstream litellm chose, which is what FR-013 requires.

**Alternatives considered**: inferring class from the served model name in the previous response (stateful, racy, wrong for the first request); exposing class from the proxy (new coupling).

## R6 — Dropping `"LlmTaskSetting"` safely

**Decision**: New migration `00033_drop_llm_task_setting.sql`. Up: `DROP TABLE IF EXISTS "LlmTaskSetting";`. Down: recreate the table with the 00020-era CHECK plus the five seeded rows, so `goose down` restores a schema the previous binary can run against.

**Rationale**: Repo convention is unique, sequential goose versions (constitution) and reversible migrations. Data loss is acceptable and intended — the rows are operator preferences with no historical value, and FR-003 requires that nothing reads them again.

**Alternatives considered**: leaving the table orphaned (violates FR-003's "removed" and leaves sqlc models generating dead types); soft-deprecating the endpoints (rejected in the spec's assumptions — the dashboard was the only consumer).

## R7 — Fate of the Cerebras Go adapter

**Decision**: Delete `internal/platform/llm/infrastructure/cerebras/` (client, curated `Models` list, live smoke test) and the `CEREBRAS_API_KEY` / `CEREBRAS_BASE_URL` config fields. Keep `infrastructure/shared/` (error taxonomy) and re-export its errors directly from `llm.go`; drop the rate-limit breaker from `shared` only if no caller remains after the deletion.

**Rationale**: Cerebras remains in use — as a *litellm* deployment, reached through the gateway. A second direct client path would be exactly the duplicated model logic this feature removes. Worker handlers depend on `llm.ErrRateLimited` & co., which already originate in `shared`; only the alias source changes.

**Alternatives considered**: keeping the adapter as a "gateway bypass" — no requirement calls for it, and it would keep `CEREBRAS_API_KEY` semantics split between the app and the proxy.

## R8 — Free-tier ordering and model selection

**Decision**: Default order Cerebras → Groq → Cohere → OpenRouter → Ollama, uniform across tasks. Exact model IDs are verified against each provider's live model list during implementation and pinned in the config with a comment naming the verification date; the spec only fixes the *tier* order (FR-006, Assumptions).

**Rationale**: Cerebras and Groq offer the fastest free inference and are already proven in this codebase (Cerebras had a first-class adapter); Cohere's free tier is rate-limited more aggressively, so it sits third as a cushion rather than a workhorse. Model catalogs churn quarterly, so hard-coding IDs in the spec would date it — the config file is the right home, which is the point of the feature.

**Alternatives considered**: per-task tier ordering (e.g. Groq first for `match` because latency dominates) — deferred; the config supports it with no code change if measurements justify it later.

**Verified 2026-07-31** against each provider's live catalog (using the keys already present in `.env`):
- Cerebras (`GET /v1/models`): `gpt-oss-120b` present — same model the deleted Cerebras adapter defaulted to (`json_object`-capable, matches OpenAI-compatible tool/JSON conventions Cerebras documents for this model).
- Groq (`GET /openai/v1/models`): `llama-3.3-70b-versatile` present with `supported_features: [tools, json_mode]`.
- Cohere (`GET /v1/models`): `command-r-08-2024` present with `features: [json_mode, json_schema, ...]`.
- OpenRouter (`GET /api/v1/models`): the three models already referenced in `gateway/config.yaml` (`deepseek/deepseek-v4-pro`, `qwen/qwen3.7-max`, `openai/gpt-4o-mini`) are all still listed and all report `response_format` in `supported_parameters`.

These four IDs are pinned into `gateway/config.yaml` with this verification date.

## R9 — Where the local tier points

**Decision**: The terminal deployment in every chain is an `ollama`-family deployment whose `api_base` comes from the litellm container's environment (the compose-internal Ollama service), with the Ollama key passed through when the deployment is Ollama Cloud.

**Rationale**: Satisfies FR-008 inside the proxy, mirrors the app-side fallback in FR-009, and keeps a single source of truth for the Ollama endpoint. The current deployment's `LLM_MODEL` is an Ollama Cloud model, so the key must be forwarded or the local tier fails auth.

**Alternatives considered**: omitting a local tier inside the proxy and relying solely on the app-side fallback — a gateway that is *up* but has exhausted every hosted tier would then fail the task, breaking FR-008/SC-005.
