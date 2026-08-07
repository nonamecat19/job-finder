# Phase 1 Contracts: Split-Model Resume Generation

Four interfaces change: the gateway routing contract, the internal Go stage seam, the HTTP surface,
and the DTOs crossing to the dashboard.

## 1. Gateway routing contract (`gateway/config.yaml`)

Three new task keys. The application sends the key as the `model` field and learns nothing about
providers (030-FR-004).

```yaml
model_list:
  # --- generation-analyze (economy) ---
  - model_name: generation-analyze
    litellm_params:
      model: openrouter/google/gemini-2.5-flash-lite
      reasoning_effort: low
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: generation-analyze-cerebras
    litellm_params: {model: cerebras/gpt-oss-120b, api_key: os.environ/CEREBRAS_API_KEY}

  # --- generation-select (economy; premium tier reachable by escalation) ---
  - model_name: generation-select
    litellm_params:
      model: openrouter/google/gemini-2.5-flash-lite
      reasoning_effort: low
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: generation-select-premium        # escalation target, FR-007
    litellm_params:
      model: openrouter/anthropic/claude-sonnet-5
      reasoning_effort: low
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: generation-select-premium-haiku
    litellm_params:
      model: openrouter/anthropic/claude-haiku-4.5
      reasoning_effort: low
      api_key: os.environ/OPENROUTER_API_KEY

  # --- generation-summary (premium) ---
  - model_name: generation-summary
    litellm_params:
      model: openrouter/anthropic/claude-sonnet-5
      reasoning_effort: low
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: generation-summary-haiku
    litellm_params:
      model: openrouter/anthropic/claude-haiku-4.5
      reasoning_effort: low
      api_key: os.environ/OPENROUTER_API_KEY

litellm_settings:
  fallbacks:
    - generation-analyze: [generation-analyze-cerebras, generation-analyze-cohere, local]
    - generation-select:  [generation-select-cerebras, generation-select-cohere, local]
    - generation-select-premium: [generation-select-premium-haiku, local]
    - generation-summary: [generation-summary-haiku, generation-summary-cerebras, local]
```

The escalation key needs its own chain for the same reason every other key does: an escalation
fired during an Anthropic outage must still land somewhere, and Constitution V requires that
somewhere to be the local model. A key declared without a chain terminates on a hosted provider,
which is the failure this rule exists to prevent.

**Invariants** (each verifiable by reading the file):

1. Every task key — including `generation-select-premium` — appears in `fallbacks` and its chain
   terminates at the shared `local` Ollama deployment (FR-011).
2. Every deployment declares a reasoning bound — `reasoning_effort` or `reasoning: {enabled: false}`
   (FR-014). A deployment without one is a configuration error.
3. Every `api_key` is an `os.environ/…` reference; no literal key (030-C4).
4. `generation` is retained, serving the on-demand cover letter.
5. Adding, removing or repointing any of the above is a file edit plus
   `docker compose restart litellm` — no rebuild, no migration (FR-016).

**Request contract per stage** — unchanged in shape from today: `POST {GATEWAY_URL}/chat/completions`,
`model` = task key, `stream: false`, `response_format` = strict `json_schema` derived from the
stage's Go type, `max_completion_tokens` per stage (analyze 8192, select 16384, summary 2048).

## 2. Internal Go seam

`Service` holds one provider today. It gains one per stage; each is an ordinary `*llm.Router`
differing only in task key, so an unconfigured gateway still routes every stage to Ollama.

```go
// apps/api/cmd/server/compose.go
type GenerationRouters struct {
    Analyze *llm.Router   // "generation-analyze"
    Select  *llm.Router   // "generation-select"
    Premium *llm.Router   // "generation-select-premium" — escalation only
    Summary *llm.Router   // "generation-summary"
    Cover   *llm.Router   // "generation" — on-demand cover letter
}

// apps/api/internal/generation/application
func NewService(
    q domain.Repository, profiles domain.ProfileStore,
    htmlRenderer *infrastructure.HtmlPdfRenderer, rendercv *infrastructure.RenderCvRenderer,
    routers GenerationRouters,
    genModel, masterPath, defaultLevel string, shape ShapeProvider,
) *Service
```

Stage functions in `rendercv_llm.go`, each taking the provider for its own stage:

```go
func analyzeVacancy(ctx, lc llm.Provider, model, vacancy string, hints *domain.VacancyHints) (domain.VacancyAnalysis, error)
func selectContent(ctx, lc llm.Provider, model string, master, analysis, level, prevViolations, cfg) (domain.TailoredSelection, error)
func writeSummary(ctx, lc llm.Provider, model string, brief domain.SummaryBrief) (domain.TailoredSummary, error)
func expandContent(ctx, lc llm.Provider, model string, …) (domain.TailoredSelection, error)   // no summary
func condenseContent(ctx, lc llm.Provider, model string, …) (domain.TailoredSelection, error) // no summary
```

`SummaryBrief` is the trimmed premium input (research R3): vacancy analysis, derived years figure,
selected highlights, leading skill groups. **Not** the master profile.

Completeness verifier, pure and independently testable:

```go
// apps/api/internal/generation/domain/rendercv_completeness.go
func VerifyCompleteness(master, merged domain.RendercvMaster,
    analysis domain.VacancyAnalysis, cfg domain.ShapeConfig) CompletenessReport
```

## 3. HTTP surface

**Unchanged**: `POST /api/documents/tailor` — same request body, same 200 shape. The `coverLetter`
field of the response becomes `null` (FR-013); the resume field gains provenance (§4).

**New**: `POST /api/documents/{id}/cover-letter`

| | |
|---|---|
| Path param | `id` — an existing resume document |
| Body | none |
| 200 | `GeneratedDocumentDto` for the cover letter, stored against the same job/vacancy |
| 404 | resume id unknown |
| 409 | referenced document is not a resume |
| Served by | task key `generation` |
| Versioning | existing `(jobId, type, version)` uniqueness applies; a repeat request increments version rather than duplicating (US4 scenario 3) |

**Unchanged**: `GET /api/documents/{id}`, `PUT /api/documents/{id}`, `GET /api/documents/{id}/pdf`.

Job-triggered background generation (`interfaces/worker`) produces a resume only (FR-013).

## 4. DTO contract (`packages/shared`, generated via tygo)

`GeneratedDocumentDto` gains:

```ts
summaryModel: string | null;        // model that wrote the summary
summarySubstituted: boolean;        // drives the FR-012 marker
selectionEscalated: boolean;        // selection re-run on premium
stageCostUsd: number | null;        // measured, not estimated (FR-017)
```

Dashboard contract: when `summarySubstituted` is true, `TailorPage` renders a visible marker on the
resume result surface stating the summary came from a fallback (FR-012, clarified answer). No
dialog, no interruption. The activity record carries the same fact for audit.

**Forbidden across all four contracts**: any provider name, upstream model identifier or credential
reaching application code, API responses, or the dashboard. `summaryModel` is the *served* model
label already recorded on documents today (`GeneratedDocument.model`), which is an observed
response value, not a routing input.
