# Contract: the embedding path

**Feature**: 044-litellm-only-routing · **Date**: 2026-08-12

Embeddings become an ordinary gateway call. This document states the wire contract, the error
contract and the two invariants that make a wrong answer here loud instead of silent.

---

## E1 — Request

```
POST {GATEWAY_URL}/embeddings
Authorization: Bearer {LITELLM_MASTER_KEY}
Content-Type: application/json

{"model": "embed", "input": ["<text, truncated to 8000 chars>"]}
```

**E1-1**: `model` is the scenario name `embed`. Never a provider or upstream model name — same rule
as chat (C1-1).

**E1-2**: The application sends **no** `dimensions`, no `input_type` and no `encoding_format`. All
three are properties of the deployment, declared in `gateway/config.yaml` (C3-4). A per-call
dimension is a way for one caller to write an incompatible vector into a shared column, and a
per-call `input_type` is a way to embed one side of a symmetric comparison as a query
(research.md R2).

**E1-3**: The 8000-character truncation is preserved from the Ollama implementation
(`ollama.go:300`, `strutil.Truncate`). It is applied by the caller as it is today
(`matching/application/service.go:87`) and by the adapter as a backstop.

**E1-4**: Observability metadata rides the same helper as chat — `existing_trace_id` from the
context, `generation_name: "embed"`, `tags: ["embed"]`. This is what makes FR-004 true.

## E2 — Response

```json
{"model": "...", "data": [{"index": 0, "embedding": [ ... ]}],
 "usage": {"prompt_tokens": 0, "total_tokens": 0}}
```

**E2-1**: The adapter reads `data[0].embedding`. An empty `data` array is
`ErrInvalidResponse` — the same class the chat path returns for zero choices.

**E2-2** **(the load-bearing one)**: The returned vector's length MUST equal the configured
`EMBED_DIMS`. A mismatch is an error, never a stored value. Without this check, a deployment retuned
to another width surfaces as a Postgres type error per job at best, and as vectors from two spaces
in one column at worst.

**E2-3**: Served model and usage are reported through the same hooks as chat —
`domain.ReportServedModel`, `domain.ReportUsage`, read from `x-litellm-model-name`,
`x-litellm-response-cost` and `x-litellm-attempted-fallbacks`. One structured log line per request,
same fields as `logServed`.

## E3 — Errors

Classified through `infrastructure/shared.ClassifyProviderError`, identical to the chat path:

| Status | Sentinel |
|---|---|
| 429 | `ErrRateLimited` |
| 401 / 403 | `ErrCredentialRejected` |
| 402 | `ErrInsufficientCredits` |
| 404 / 400 / 422 | `ErrModelUnavailable` |
| 5xx | `ErrProviderUnavailable` |
| transport failure | `ErrProviderUnavailable` |

**E3-1**: There is no fallback to a locally computed vector, no zero vector, and no cached previous
vector. An embedding failure fails its task and retries under the existing worker policy. A
substituted vector would be a Principle II fabrication wearing a number.

## E4 — Storage invariants

**E4-1**: Every write of `"Job"."embedding"` also writes `"Job"."embedModel"` with the model the
gateway actually served, captured from `x-litellm-model-name` via `llm.WithServedModelCapture`.
Same for `"Profile"` (already true). There is no Go-side `EMBED_MODEL_ID` mirror: the served model
is authoritative provenance, so a model swap in `gateway/config.yaml` is a config-only change.

**E4-2** **(FR-020)**: A row whose `embedModel` differs from the currently served value is treated
as **unembedded**. It is re-embedded on next use and never compared against a current vector.
Implemented as a predicate, not a convention.

**E4-3**: `"Job"."embeddingHash"` keeps its 019 meaning — unchanged content skips re-embedding. It
is nulled once by the migration and then behaves exactly as before. This is the only thing making
the lazy re-embed affordable, so it is not to be "simplified away" alongside the local provider.

## E5 — Go surface

```go
// infrastructure/gateway
func (g *Provider) Embed(ctx context.Context, text string) ([]float32, error)

// application
func (r *Router) Embed(ctx context.Context, text string) ([]float32, error) // → gateway, model "embed"
```

**E5-1**: `gateway.New` loses its `ollama domain.Provider` parameter. `Embed` is a real
implementation, not a delegation (`gateway.go:466` today).

**E5-2**: `domain.Provider` is unchanged — `Embed(ctx, string) ([]float32, error)` keeps its
signature, so `profile.NewService`, `matching`, and every hand-written test fake compile untouched
except where they constructed an Ollama provider.

**E5-3**: `profile.NewService` takes `llm.Provider` and is handed a router. It no longer carries an
`embedModel` string; the provenance value written on each embedding is the served model captured
from the gateway's response (`llm.WithServedModelCapture`), not a model to request.

## E6 — Tests

| Test | Asserts |
|---|---|
| `gateway/embed_golden_test.go` | exact request body: key set is `{model, input}` and nothing else (E1-2). A stray `dimensions` key fails. |
| `gateway/embed_test.go` | happy path, empty `data`, wrong-length vector (E2-2), each error status → sentinel |
| `platform/llm/application/router_test.go` | `Embed` routes to the gateway under `embed`; no local provider exists to route to |
| integration | a real proxy round-trip: same text twice → identical vector; a similar pair scores above a dissimilar pair (research.md R2's silent-failure check) |
