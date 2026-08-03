# Contract: Task Router (internal Go)

**Package**: `internal/platform/llm/application`, re-exported by `internal/platform/llm`
**Replaces**: the `SnapshotHolder` / `RouterSnapshot` / `TaskSetting` routing introduced by 001-cerebras-model-toggle.

## C1 — Construction

```go
func NewRouter(taskKey string, gateway, local domain.Provider, localModel string) *Router
```

- `gateway` is nil when `GATEWAY_URL` is empty; `local` (Ollama) is never nil.
- `localModel` is the task's `LLM_MODEL_*` value, empty meaning "fall back to `LLM_MODEL`".
- Immutable after construction. There is no setter, no holder, no atomic swap, and no code path that changes routing at runtime.

## C2 — Behaviour

| Method | Contract |
|--------|----------|
| `Complete` / `CompleteJSON` | Route to `gateway` when non-nil with `CompleteOptions.Model = taskKey`; otherwise route to `local` with `Model = localModel`. An explicit caller-supplied `Model` still wins (existing `withModel` semantics preserved). |
| `Embed` | Always `local`, regardless of chat routing (FR-014). |
| `ModelName` | `taskKey` when gatewayed (that is what the proxy resolves), else the effective local model. |
| `ProviderClass` | `hosted` when `gateway != nil`; else `local.IsHosted()` (FR-013). Satisfies `queue.ClassResolver` unchanged. |

`Router` continues to satisfy `domain.Provider`, so `matching`, `generation`, `ghostjob`, `salary`, `keyword`, `recruiter`, and `outreach` call sites are untouched apart from `composeLLM`'s construction arguments.

## C3 — Deleted from the facade

`llm.SnapshotHolder`, `llm.NewSnapshotHolder`, `llm.RouterSnapshot`, `llm.TaskSetting`, `llm.TaskProvider` + its three constants, `llm.CerebrasProvider`, `llm.NewCerebras`, `llm.CerebrasModel(s)`, `llm.IsSupportedCerebrasModel`, `llm.DefaultCerebrasModel`.

`llm.ErrRateLimited`, `ErrCredentialRejected`, `ErrInsufficientCredits`, `ErrModelUnavailable`, `ErrProviderUnavailable`, `ErrInvalidResponse`, `Terminal`, `Retryable` are **kept**, now aliasing `infrastructure/shared` directly instead of `infrastructure/cerebras`. Worker handlers that branch on these keep compiling unchanged.

`llm.NewProviders` returns `(*OllamaProvider, *GatewayProvider, error)` — the Cerebras leg is gone.

## C4 — Gateway adapter additions (FR-012)

- Read the served model from, in order: the `x-litellm-model-name` response header, then the body's `model` field, then `"unknown"`. Log one structured line per request: `task`, `requested_group`, `served_model`, `duration_ms`, `outcome`.
- Log `x-litellm-model-id` when the proxy sends it (an opaque hash — a correlation field, not the served-model name).
- **Verified 2026-07-31**: the body `model` field alone is unreliable — on a primary-tier hit (no fallback) the pinned image echoes the *requested group name*, not the resolved model, only becoming the resolved model once a fallback fires. `x-litellm-model-name` is authoritative on both paths.
- Logging MUST NOT change error classification or add a failure mode: an unparsable/absent field is logged as `served_model=unknown`, never an error.

## C5 — Test obligations

- `NewRouter` with a nil gateway routes chat to Ollama with the per-task local model; `ProviderClass` reflects `IsHosted()`.
- `NewRouter` with a gateway routes chat to the gateway with `Model == taskKey` and reports `hosted`.
- `Embed` hits Ollama in both configurations.
- An explicit `CompleteOptions.Model` from a caller is not overwritten.
- Gateway adapter logs the served model taken from the response body and tolerates a missing field.
