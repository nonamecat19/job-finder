# Contract: `gateway/config.yaml` after 044

**Feature**: 044-litellm-only-routing · **Date**: 2026-08-12
**Enforced by**: `apps/api/internal/platform/llm/gateway_config_test.go` (job `go-test`; the `go`
path filter in `.github/workflows/api-ci.yml` already includes `gateway/**`).

This supersedes the routing contract in `specs/domains/llm-routing.md` §2.1 on three points, marked
**[CHANGED]** below. Everything not marked is unchanged and still binding.

---

## C1 — Scenario names

**C1-1**: The application sends a scenario name as `model` and nothing else. No provider, no upstream
model name. *(unchanged, 030-FR-004)*

**C1-2** **[CHANGED]**: The declared scenario set is exactly:

```
match  ghost  rephrase  recruiter  salary  outreach
generation  generation-analyze  generation-select  generation-select-premium
generation-summary  generation-summary-premium  generation-summary-fast
embed
```

`default` and `local` are removed. A request naming either MUST fail with 4xx.

**C1-3**: An unknown group fails loudly (4xx). It is never routed to a default. *(unchanged, and now
load-bearing: there is no default group left to absorb it.)*

**C1-4**: Every name above is requested by exactly one router in `cmd/server/compose.go`, or is the
`embed` group requested by `Router.Embed`. A declared-but-unrequested group is a defect, not spare
capacity.

## C2 — Chains

**C2-1** **[CHANGED]**: Every scenario resolves to a chain of **at least two tiers drawn from at
least two distinct providers**. This replaces "every chain terminates at `local`" (030-FR-008),
which is void with the local tier.

**C2-5** **[NEW 2026-08-13]**: `embed` is a temporary single-provider exception to C2-1. Cohere was
the embed primary and OpenAI its only fallback; when Cohere and Groq were removed from every chain
(2026-08-13), `embed` collapsed to one OpenAI tier with no fallback. `embed` must still declare a
`model_list` entry, but is not required to declare a `fallbacks` chain until a second embedding
provider is added back. Embedding *provenance* no longer depends on a Go-side `EMBED_MODEL_ID`
mirror: the application records the model the gateway actually served (`x-litellm-model-name`) on
each stored vector, so a model swap in this file is a config-only change with no Go rebuild.

**C2-2**: No chain is empty and no chain names an undeclared tier. *(unchanged)*

**C2-3** **[CHANGED]**: Ordering follows the class of the scenario:

| Class | Order rule |
|---|---|
| quality-writing | the quality model leads; cheaper tiers follow as fallbacks |
| economy-structured | a free tier leads; paid aggregator tiers follow |
| tool-capable | free tier leads; **every** tier tool-capable |
| embedding | primary provider leads; every tier at the declared width |

The previous blanket rule ("all free tiers precede any OpenRouter tier", 030-FR-006) is replaced by
this per-class rule. Quality-writing scenarios are *expected* to lead with OpenRouter.

**C2-4**: Chains after this feature:

```yaml
fallbacks:
  - match:                      [match-openrouter]
  - ghost:                      [ghost-openrouter]
  - rephrase:                   [rephrase-openrouter]
  - recruiter:                  [recruiter-openrouter]
  - salary:                     [salary-openrouter]
  - outreach:                   [outreach-haiku, outreach-cerebras]
  - generation:                 [generation-haiku, generation-cerebras]
  - generation-analyze:         [generation-analyze-cerebras]
  - generation-select:          [generation-select-cerebras]
  - generation-select-premium:  [generation-select-premium-haiku, generation-select-premium-cerebras]
  - generation-summary:         [generation-summary-haiku, generation-summary-cerebras]
  - generation-summary-premium: [generation-summary-premium-sonnet, generation-summary-cerebras]
  - generation-summary-fast:    [generation-summary-fast-openrouter]
```

`embed` has no fallback entry (C2-5). `recruiter` and `generation-summary-fast` each lost their
whole fallback list when Cohere and Groq were removed and gained an openrouter tier instead, so
C2-1 still holds for them.

## C3 — Tier declarations

**C3-1**: Every `api_key` is an `os.environ/…` reference. A literal fails the build. *(unchanged,
030-C4)*

**C3-2**: Every tier of `salary` declares `model_info.supports_function_calling: true`. No other
scenario is required to. *(narrowed from "every tier of `default`")*

**C3-3**: Every tier whose family deliberates declares its bound — `reasoning_effort: low` for
OpenAI/Anthropic/Google, `reasoning: {enabled: false}` for z-ai and deepseek-flash. A tier honouring
neither is not eligible. *(unchanged, 035-FR-014)*

**C3-4** **[NEW]**: Every tier of `embed` declares `output_dimension: 1024` and
`input_type: search_document`. The widths across the chain MUST be identical, and MUST equal the
application's `EMBED_DIMS`.

**C3-5**: Every tier of a JSON-consuming scenario supports `response_format`. `drop_params: true`
means a tier that does not will silently answer in prose that no fallback rescues. *(unchanged,
030-C5)*

## C4 — Global settings

Unchanged and asserted: `drop_params: true`, `num_retries: 1`, `request_timeout: 60`,
`allowed_fails: 3`, `cooldown_time: 60`, `success_callback: ["langfuse"]`,
`failure_callback: ["langfuse"]`. This feature changes none of them, so the worst-case timing
arithmetic under the adapter's 15-minute safety net holds unchanged.

**C4-1** **[NEW]**: Chains are now at most 4 tiers where they were 5. Worst case per request
therefore falls from 600s to 480s. The safety net is unchanged; the margin grows.

## C5 — Change procedure

Unchanged from 030-C6 and it must stay that way: edit `gateway/config.yaml` →
`docker compose restart litellm`. No application rebuild, no migration, no dashboard action.

**C5-1** **[CHANGED]**: The one exception is the embedding width. Changing `output_dimension` on the
`embed` chain is **not** a config-only change — it requires a migration and re-embedding. The
guardrail test asserting `output_dimension == EMBED_DIMS` is what turns that from a silent corruption
into a failed build.

## C6 — Guardrail test changes

`gateway_config_test.go`:

| Assertion | Change |
|---|---|
| chain terminates at `local` | **deleted** |
| chain has ≥2 tiers, ≥2 providers | **new** |
| no tier named `local`, no group named `default` | **new** |
| `requestedGenerationGroups` list | extended to the full scenario set of C1-2, renamed to reflect that it is no longer generation-specific |
| tool-capability declared on `default` chain | narrowed to `salary` |
| `embed` chain width + `input_type` | **new** |
| `EMBED_DIMS` mirrors the declared `output_dimension` | **new** — reads the app default from `internal/config/defaults.go` |
| reasoning-switch check | **widened**: currently scoped to `generation-*` stage deployments, now applies to **every** `openrouter/*` tier. This feature adds OpenRouter tiers to `outreach`, `salary` and `recruiter`, which the narrow check would not see — and an unbounded thinking model returns a 200 with empty content that no fallback rescues |
| literal-credential check | unchanged |

The existing "invariants accept valid / reject broken config" inline-fixture tests
(`gateway_config_test.go:485,494`) are extended to the new invariants — a guardrail that can only
ever pass guards nothing, and that principle is already established in this file.
