# Contract: configuration surface after 044

**Feature**: 044-litellm-only-routing · **Date**: 2026-08-12

---

## K1 — Required application configuration

| Key | Rule |
|---|---|
| `GATEWAY_URL` | REQUIRED. Empty or unset → startup error naming the key. |
| `LITELLM_MASTER_KEY` | REQUIRED. Same. Remains the one secret the application container is permitted (`config_test.go:141`). |

**K1-1**: Validation lives in `internal/config`, in the shape `queue.validateLiveness` already
uses: an error naming the key, returned from load, fatal at `cmd/server`.

**K1-2**: Startup validates that the gateway is **configured**, never that it is **reachable**. The
`litellm` service keeps having no `depends_on`, and the application gains none on it (036-C3-2,
asserted in `internal/config/config_test.go`). An unreachable gateway fails tasks, not boots.

**K1-3**: Startup MUST NOT attempt an AI request as a health probe. `/readyz` semantics are
untouched by this feature.

**K1-4**: The AI requirement applies to **binaries that do AI work**, not to every caller of
`config.Load()`. Three callers do no inference — `cmd/seed/main.go:42`, `internal/db/capacity_test.go:15`
and the `live_test.go` files — and making a fixture loader demand a gateway URL would be a bug, not a
guarantee. `Load()` validates the AI surface; a sibling entry point that skips that validation serves
the non-AI callers. Which callers get which is an explicit list, not an inference from context.

## K2 — Changed keys

| Key | Before | After |
|---|---|---|
| `EMBED_DIMS` | `768`, read by nothing | `1024`, asserted against every returned vector (E2-2) and against `gateway/config.yaml` by the guardrail test |
| `EMBED_MODEL` | model name sent to Ollama | **removed** — no Go-side model string remains; embedding provenance is the served model captured from `x-litellm-model-name` (embeddings.md E4-1) |
| `AI_CONCURRENCY_CLOUD` | one of two concurrency settings | the only one. Name kept deliberately: renaming breaks every deployed `.env` to improve a word. |

## K3 — Removed keys

```
OLLAMA_URL  OLLAMA_KEY  OLLAMA_KEEP_ALIVE  EMBED_URL
LLM_MODEL  LLM_MODEL_MATCH  LLM_MODEL_GENERATION  LLM_MODEL_REPHRASE  LLM_MODEL_GHOST
LLM_MODEL_GENERATION_ANALYZE  LLM_MODEL_GENERATION_SELECT
LLM_MODEL_GENERATION_PREMIUM  LLM_MODEL_GENERATION_SUMMARY
AI_CONCURRENCY_LOCAL
```

**K3-1**: Removed from `config.Config`, from `internal/config/defaults.go`, from both compose files
and from `.env.example` in one change. A key removed from the struct but left in defaults is a lie
the next reader believes.

**K3-2**: `Config.ModelOr` and `Config.GenerationModelOr` (`config.go:131`, `:142`) are deleted —
their only inputs are gone.

**K3-3**: `LLM_MAX_IDLE_CONNS_PER_HOST` is **kept**. It tunes the HTTP client that now carries
*more* traffic, not less.

## K4 — Gateway-container credentials (unchanged location)

| Key | Change |
|---|---|
| `CEREBRAS_API_KEY`, `OPENROUTER_API_KEY` | unchanged |
| `GROQ_API_KEY`, `COHERE_API_KEY` | **removed** 2026-08-13 — no tier routes through Groq or Cohere |
| `OPENAI_API_KEY` | **new**, optional — the `embed` chain (single provider since Cohere's removal, gateway-config.md C2-5) |
| `OLLAMA_URL`, `OLLAMA_KEY` | **removed** from the `litellm` service environment |
| `LANGFUSE_*` | unchanged, and still never passed to the application container |

**K4-1**: Every one keeps its `${VAR:-}` empty default, so a missing credential never aborts config
load — it fails that tier's auth at request time and the chain advances (030-FR-011).

## K5 — Compose

**K5-1**: `docker-compose.prod.yml` gains a `litellm` service: same image, same read-only mount of
`gateway/config.yaml`, same Python healthcheck, same absence of `depends_on`. Prod and dev route
through one artifact (FR-022).

**K5-2**: The prod application service's `# --- local model pinning ---` block is deleted entirely,
and its `# --- LLM access ---` comment — which currently reads *"OLLAMA_KEY is the local-first path
(Principle V), which the app must be able to reach with no gateway at all"* — is rewritten. That
comment is a load-bearing description of a guarantee this feature removes; leaving it would
misdescribe the system to the next operator.

**K5-3**: Neither compose file exposes an Ollama service, and no volume, network or healthcheck
references one.

## K6 — `.env.example`

**K6-1**: The LLM block is rewritten around: required gateway settings, the gateway-only provider
credentials, embedding width and provenance, concurrency and timeouts.

**K6-2** **(FR-024)**: It carries a plain statement, not a footnote, that prompt content — profile
data, resume content, posting text — is sent to third-party providers on every AI request, and that
there is no configuration under which it is not. This replaces the current file's framing, which
opens *"LLM — Ollama only"*.

## K7 — Documentation surfaces updated in the same change

| File | Change |
|---|---|
| `specs/domains/llm-routing.md` | §1–§3 amended in place; superseded rules (local terminal tier, local-first fallback, `default` scenario, embeddings excluded from gateway and from observability in §7.3) marked where they are stated |
| `.specify/memory/constitution.md` | Principle V rewritten; version → **2.0.0**; sync-impact header records the reasoning |
| `docs/docs/ai/llm-abstraction.md`, `docs/docs/ai/overview.md` | the two-provider description becomes one |
| `AGENTS.md` | if it names the local-first path, corrected |

**K7-1**: The constitution amendment is MAJOR because a principle is redefined, not clarified.
Amending it in the same change as the code is the constitution's own governance rule, not an
optional courtesy.
