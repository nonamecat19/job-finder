# Split-Model Resume Generation

**Date**: 2026-08-07
**Status**: Design, awaiting approval
**Related**: `specs/033-resume-gen-strictness`, `specs/034-resume-model-choice`, `specs/domains/llm-routing.md`

## Problem

One LLM call does all the tailoring work today. `selectAndTailor` writes the summary,
reorders skill groups, picks highlights, and rephrases them, in a single structured call
against a single model. That forces one model choice to serve four jobs with different
requirements, and the run is priced at the most expensive one.

Measurement (2026-08-07, one vacancy, identical master profile and prompt, seven models in
parallel) shows what that costs:

| model | $/call | sec | ungrounded skills | summary |
|---|---|---|---|---|
| claude-sonnet-5 | 0.0414 | 19.4 | 0 | good |
| claude-haiku-4.5 | 0.0204 | 61.0 | 0 | good |
| glm-4.7 | 0.0048 | 40.4 | 0 | **fabricated** |
| gemini-2.5-flash-lite | 0.0013 | 6.0 | 0 | not evaluated separately |

Full-pipeline runs land at ~$0.113 (sonnet-5), ~$0.056 (haiku-4.5), ~$0.021 (glm-4.7). The
cheap model is 5x cheaper and fails on exactly one thing: the summary, the only part of the
output that is written rather than selected. Everything else — ranking skill tokens, picking
the top highlights per job, rewording them within a word-overlap bound — the cheap models do
as well as the expensive ones, verbatim-grounded and complete.

So: stop paying premium rates for ranking. Buy the premium model for the 200 tokens that
need it.

## Approach

Split the single tailoring call into three stages, each addressed as its own gateway task
key, each backed by the cheapest model that does its job well.

| stage | task key | model | $ | sec |
|---|---|---|---|---|
| vacancy analysis | `generation-analyze` | gemini-2.5-flash-lite | 0.0003 | 3 |
| selection + rephrase | `generation-select` | gemini-2.5-flash-lite | 0.0013 | 6 |
| summary | `generation-summary` | claude-sonnet-5 | 0.0080 | 5 |
| page fit (conditional) | `generation-select` | gemini-2.5-flash-lite | 0.0013 | 6 |
| **total** | | | **~$0.011** | **~20s** |

**10x cheaper and 3x faster than today's full-sonnet run, with the summary still written by
sonnet.** The premium call is cheap because its prompt is small: a summary needs the vacancy
analysis, the derived years figure, the selected highlights and the top skill groups — not
the 17KB master profile the selection stage requires.

Cover letter generation leaves the always-on path and becomes on-demand, removing a fourth
call from every run.

### Alternatives rejected

**Three-tier** (mid-tier model for rephrasing and page fitting, ~$0.05, ~50s). Buys better
bullet wording. The measurements show bullet wording is not the failure — median highlight
overlap is 1.0 for most models, meaning they already reproduce the master's wording. Paying
for a fix nobody needs.

**Adaptive escalation** (everything cheap, escalate the summary to premium only when the
verifier flags it, ~$0.012 average). Cheapest on average, but the failure it must detect is
the one that is hardest to detect: glm-4.7's bad summary was fabricated, not malformed. A
good later optimisation once summary verification is proven; a bad foundation to build on.

## Architecture

### Routing: new task keys, not model identities

The application asks the gateway for a task and never names a model (030-FR-004). That rule
is what makes this design cheap to implement: the split is three new **task keys**, each with
its own ordered chain in `gateway/config.yaml`, and the application learns nothing new about
providers.

```yaml
model_list:
  - model_name: generation-analyze          # tier 1: cheap
    litellm_params:
      model: openrouter/google/gemini-2.5-flash-lite
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: generation-select
    litellm_params: {model: openrouter/google/gemini-2.5-flash-lite, ...}
  - model_name: generation-summary          # tier 1: premium
    litellm_params:
      model: openrouter/anthropic/claude-sonnet-5
      reasoning_effort: low
      api_key: os.environ/OPENROUTER_API_KEY
  # … free-tier fallbacks per key, then the shared `local` deployment

litellm_settings:
  fallbacks:
    - generation-analyze: [generation-analyze-cerebras, ..., local]
    - generation-select:  [generation-select-cerebras, ..., local]
    - generation-summary: [generation-summary-haiku, ..., local]
```

Every chain still terminates at the local Ollama deployment (030-FR-008, Constitution V).
The existing `generation` key is retained for the on-demand cover letter.

**Reasoning control is part of the routing contract, not an app concern.** A thinking model
that is not told to stop thinking spends its entire output budget on reasoning and returns
empty content — this is what breaks resume generation today. Each deployment carries the
switch its provider honours: `reasoning_effort: low` for OpenAI and Anthropic models,
`reasoning: {enabled: false}` for z-ai and deepseek-flash models. deepseek-v4-pro honours
neither and is therefore not eligible for any chain tier without a 32k output budget.

### Application seam

`Service` holds one LLM client today. It gains one `*llm.Router` per stage, constructed the
same way the existing one is, differing only in task key:

```go
type Service struct {
    analyzeLLM llm.Provider   // task key "generation-analyze"
    selectLLM  llm.Provider   // task key "generation-select"
    summaryLLM llm.Provider   // task key "generation-summary"
    coverLLM   llm.Provider   // task key "generation"
    // …
}
```

Each router keeps the existing gateway-or-local behaviour, so an unconfigured gateway still
serves every stage from Ollama.

### Data contract

`TailoredSections` currently carries the summary alongside the selection. The split separates
them so each stage has a schema containing only what that stage produces:

```go
type TailoredSelection struct {   // produced by generation-select
    Skills     []TailoredSkillGroup
    Experience []TailoredExperience
    Projects   []TailoredProject
    SkillGroupsToAdd    []TailoredSkillGroupAdd
    SkillGroupsToRemove []string
    SkillChanges        []SkillChange
}

type TailoredSummary struct {      // produced by generation-summary
    Summary string
}
```

`TailoredSections` remains the merged shape the render path consumes, assembled from both.
Schemas are derived from these types through the existing strict-schema path, so the
selection stage can no longer emit a summary and the summary stage can no longer touch the
selection — the split is enforced by the schema, not by prompt wording.

### Flow

```
vacancy ──> [analyze: cheap] ──> VacancyAnalysis
                                      │
master ───────────────────────────────┼──> [select: cheap] ──> TailoredSelection
                                      │                              │
                                      │                    grounding + completeness checks
                                      │                              │
                                      └──> [summary: premium] <──────┘   (trimmed prompt:
                                                   │                      analysis, years,
                                            summary grounding             selected highlights,
                                                   │                      top skill groups)
                                                   ▼
                                            merge ──> page fit (cheap, conditional) ──> render
```

The summary stage runs after selection because its prompt includes the selected highlights —
that is what lets it reference a real achievement without being handed the whole master.

### Verification per stage

Each stage is checked for the failure that stage can produce:

- **Selection**: existing skill-token grounding and highlight word-overlap checks (033
  FR-001/FR-002), plus a **completeness assertion** weighted by what the vacancy asked for —
  every master skill matching a vacancy-required skill retained exactly, nice-to-have matches
  retained at 80% or above, per-job achievements at or above the configured minimum. A second
  consecutive shortfall escalates the selection stage to the premium option rather than rendering. Round 3 measured three cheap models that silently dropped content
  (glm-4.7-flash returned 17 skill tokens of 187; deepseek-v4-flash returned 89;
  mistral-small-3.2 returned 6 highlights of 15). Silent truncation is the cheap tier's
  characteristic failure and nothing currently catches it.
- **Summary**: the existing years-assertion check (028), plus rejection of skill tokens and
  numeric metrics absent from the master. One re-prompt, then strip-and-log, per 033 FR-003.
- **Page fit**: re-runs the selection checks. It may not touch the summary at all — fitting
  adjusts achievements, skills and projects only, so the premium-written summary is immutable once
  produced. A page target that cannot be met by selection alone records the shortfall rather than
  rewording the summary to save space.

### Error handling

- Any stage exhausting its chain terminates at local Ollama; the run completes rather than
  failing (030-FR-009).
- If the summary stage is served by a fallback rather than its premium tier 1, the run is
  marked as substituted on the activity row — same contract as 034 FR-006, so a user is never
  silently given a cheap-model summary while believing otherwise.
- Explicit per-stage output caps: analyze 8192, select 16384, summary 2048. The summary cap is
  small because the stage produces ~200 tokens; a model that needs more than 2048 for it is
  reasoning and must be configured not to.
- Per-stage client timeouts stay in the application. LiteLLM's `request_timeout: 60` was
  observed **not** to be enforced (a single call hung 830s until the app's own 14-minute
  timeout fired), so the app's deadline is the only real bound.

### Testing

- Unit: per-stage prompt builders; the strict-schema transform (all properties required,
  `additionalProperties: false`, optional fields nullable); summary grounding checks;
  completeness assertion against a truncated-response fixture.
- Integration: a fake provider per stage asserting each stage requested its own task key and
  no stage received a provider or model name.
- Benchmark: extend the existing strictness benchmark to record per-stage latency, token
  usage and cost, so the cost table in this document can be regenerated rather than estimated.

## Prerequisites already implemented

Two fixes this design depends on are already in the working tree, found while measuring:

- `generationMaxTokens` 4096 → 16384, `analysisMaxTokens` 2048 → 8192
  (`rendercv_llm.go`). At 4096 a reasoning model consumed the whole budget on reasoning and
  returned empty content, so **every** resume run failed with
  `not valid JSON: unexpected end of JSON input`.
- `strictifySchema` (`platform/llm/domain/port.go`). The reflected schema omitted optional
  keys from `required`, which OpenAI and Azure reject with
  `invalid_json_schema`. gpt-5-mini and gpt-5.1 were 100% unusable before this.

## Open questions

- **Cost recording.** The gateway response carries `usage.cost` and the adapter discards it.
  Capturing it onto the activity row turns every cost figure in this document into a measured
  value, and is a prerequisite for 034 FR-002/FR-010 (showing indicative cost per option).
  Recommended, but out of scope here.
- **Local-only cheap tier.** `gpt-oss-120b` is open-weights, measured at $0.0004/call, and the
  Ollama terminal tier already runs `gpt-oss:120b-cloud`. If its rewriting behaviour (0.65
  overlap, 3 drifted highlights) is acceptable under the overlap check, the mechanical stages
  could run at zero marginal cost. Worth a follow-up measurement.
- **Interaction with 034.** The user-facing model picker chooses among curated options. With
  this split, an "option" becomes a *summary-stage* choice; the mechanical stages are not worth
  exposing. 034's option list should be scoped to `generation-summary` before it is built.
