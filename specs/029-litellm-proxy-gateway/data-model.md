# Data Model: LiteLLM Proxy Gateway

**Feature**: 029-litellm-proxy-gateway
**Date**: 2026-07-31

## 1. No New Database Schema

This feature adds no new tables, columns, or migrations. The existing `llm_task_setting` table already stores per-task provider and model assignments as `text` columns. The new `"gateway"` provider value is a valid string in the existing `provider` column.

### Existing Table (unchanged)

```sql
-- From migration 00018_llm_task_setting.sql (feature 001)
CREATE TABLE llm_task_setting (
    task_key TEXT PRIMARY KEY,
    provider TEXT NOT NULL DEFAULT 'ollama',
    model    TEXT NOT NULL DEFAULT ''
);
```

### New Valid Provider Values

| Provider | Stored As | Meaning |
|---|---|---|
| Ollama | `"ollama"` | Route to local Ollama provider |
| Cerebras | `"cerebras"` | Route to Cerebras direct API |
| Gateway | `"gateway"` | Route to LiteLLM proxy (new) |

### New Valid Model Values for Gateway Provider

When provider is `"gateway"`, the model field stores the task key name (e.g., `"match"`, `"generation"`). The LiteLLM proxy resolves this to the actual provider+model via its `config.yaml`.

## 2. Go Domain Types

### TaskProvider (extended)

```go
// application/router.go
const (
    TaskProviderOllama   TaskProvider = "ollama"
    TaskProviderCerebras TaskProvider = "cerebras"
    TaskProviderGateway  TaskProvider = "gateway"  // NEW
)
```

### Router (extended)

```go
type Router struct {
    taskKey  string
    holder   *SnapshotHolder
    ollama   domain.Provider
    cerebras domain.Provider // nil when no Cerebras credential
    gateway  domain.Provider // nil when no GATEWAY_URL configured  // NEW
}
```

### resolve() dispatch logic (extended)

```go
func (r *Router) resolve() (domain.Provider, string) {
    snap := r.holder.Load()
    setting, ok := snap.Tasks[r.taskKey]
    if !ok {
        return r.ollama, ""
    }
    switch setting.Provider {
    case TaskProviderGateway:
        if r.gateway != nil {
            return r.gateway, setting.Model
        }
        return r.ollama, setting.Model // fallback when gateway unavailable
    case TaskProviderCerebras:
        if r.cerebras != nil {
            return r.cerebras, setting.Model
        }
        return r.ollama, setting.Model
    default:
        return r.ollama, setting.Model
    }
}
```

## 3. Config Types

### New Config Fields

```go
// config/config.go
type Config struct {
    // ... existing fields ...
    GatewayURL string `mapstructure:"GATEWAY_URL"` // NEW: LiteLLM proxy endpoint
}
```

### Defaults

```go
// config/defaults.go
var defaults = map[string]any{
    // ... existing defaults ...
    "GATEWAY_URL": "", // empty = gateway unavailable
}
```

## 4. Gateway Provider

```go
// infrastructure/gateway/gateway.go
type Provider struct {
    http    *http.Client
    baseURL string
    apiKey  string
    ollama  domain.Provider
}

func New(baseURL, apiKey string, ollama domain.Provider) (*Provider, error) {
    if baseURL == "" {
        return nil, errors.New("gateway: baseURL is required")
    }
    return &Provider{
        http:    &http.Client{Timeout: 120 * time.Second},
        baseURL: baseURL,
        apiKey:  apiKey,
        ollama:  ollama,
    }, nil
}

func (g *Provider) ModelName() string { return "gateway" }

func (g *Provider) Complete(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
    // OpenAI-compatible /chat/completions to g.baseURL
}

func (g *Provider) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
    // Same as Complete but with response_format: {type: json_object}
}

func (g *Provider) Embed(ctx context.Context, text string) ([]float32, error) {
    return g.ollama.Embed(ctx, text) // FR-006: embeddings stay on Ollama
}
```

## 5. State Transitions

### Provider Selection State Machine

```
                    ┌─────────┐
                    │ Ollama  │ (default, always available)
                    └────┬────┘
                         │
              ┌──────────┼──────────┐
              ▼          ▼          ▼
         ┌─────────┐ ┌─────────┐ ┌─────────┐
         │ Ollama  │ │Cerebras │ │ Gateway │
         │ (local) │ │ (cloud) │ │ (proxy) │
         └─────────┘ └────┬────┘ └────┬────┘
                          │           │
                    No key?     No URL?
                          │           │
                          ▼           ▼
                      ┌─────────┐ ┌─────────┐
                      │ Ollama  │ │ Ollama  │
                      │(fallback)│ │(fallback)│
                      └─────────┘ └─────────┘
```

- **Ollama → Cerebras**: Operator sets `CEREBRAS_API_KEY` env var, selects Cerebras in Settings
- **Ollama → Gateway**: Operator sets `GATEWAY_URL` env var, selects Gateway in Settings
- **Cerebras → Gateway**: Operator changes per-task provider in Settings (both available simultaneously)
- **Gateway → Ollama**: Operator clears `GATEWAY_URL` or selects Ollama in Settings
- **Any → Ollama (fallback)**: Provider unavailable (no key/URL configured) → Router falls back to Ollama

## 6. Validation Rules

| Rule | Enforced By |
|---|---|
| Provider must be `"ollama"`, `"cerebras"`, or `"gateway"` | `llmsettings/domain/types.go` `ErrInvalidProvider` |
| Gateway model must be a known task key or empty | Proxy config validation (LiteLLM returns "model not found") |
| Gateway URL must be set for gateway provider to be available | `composeLLM` — nil gateway when `GATEWAY_URL` is empty |
| Embeddings never route through gateway | `Router.Embed()` hardcodes Ollama; `GatewayProvider.Embed()` delegates to Ollama |
