# Phase 1 Data Model: Self-Hosted LLM Observability

**Feature**: `036-langfuse-llm-observability` | **Date**: 2026-08-07

This feature adds **no table to the platform's own schema** and **no goose migration**. The collector
owns its store in a separate logical database. What follows is (1) the shape the collector records,
(2) the one application type that changes, and (3) the mapping between them.

---

## 1. Collector-side entities

These live in the collector's database, not ours. They are specified here because FR-002, FR-009 and
FR-012 are assertions about their contents.

### Call Record (Langfuse "generation")

**Corrected 2026-08-07.** The first version of this table was wrong in three places and inverted the
reporting design. What the integration actually writes:

| Field | Source | Notes |
|---|---|---|
| `name` | `metadata.generation_name` if sent, else `litellm-{call_type}` | **Not** the task key by default. US1 records will read `litellm-acompletion` unless the key is sent |
| `model` | the **served deployment** | e.g. `openrouter/anthropic/claude-sonnet-5`. **Not** the requested task key |
| `metadata.model_group` | the requested task key | This — not `model` — is where the task key survives (FR-003) |
| ~~`metadata.served_model`~~ | — | **Does not exist.** The callback receives `kwargs`, not the HTTP response, and drops the `headers` key before logging |
| `input` | request messages | contains the user's master profile — see §4 |
| `output` | completion content | |
| `usage.input` / `usage.output` | prompt / completion tokens | |
| `usage.total_cost` | the proxy's response cost | See the caveat below — free and unpriced are indistinguishable |
| `startTime` / `endTime` | proxy timing | FR-002 duration |
| `level` / `statusMessage` | outcome | `ERROR` with the upstream reason when every tier fails (FR-002) |
| `traceId` | `metadata.existing_trace_id` | **Not** `metadata.trace_id`, which rewrites the trace's name, input, output and tags on every call. Absent for US1; present from US2 (FR-009) |

**Derived, not stored**: *substituted* — true exactly when `metadata.model_group` and `model` name
different deployments. FR-003 and FR-012's fallback rate are computed from that comparison. No
separate flag is recorded, and no `served_model` field is involved.

**Consequence for FR-012 (the reason this correction matters).** Because `model` is the deployment,
the collector's built-in per-model view groups by *serving model*. `generation-summary` and
`generation-select-premium` both resolve to `claude-sonnet-5` and would collapse into one bucket —
erasing exactly the per-stage distinction feature 035 exists to create. Per-task grouping must
therefore be created deliberately by sending `metadata.generation_name` = the task key and a matching
`metadata.tags` entry. That is an application change, which is why FR-012a moves per-task reporting
out of US1's zero-change scope.

**Caveat on cost (FR-014).** The integration writes the proxy's computed response cost. A model
absent from the proxy's cost map yields no cost rather than zero, so a genuinely free call and an
unpriced one are indistinguishable in the record. FR-014's "zero, not missing" therefore cannot be
asserted from the record alone; either the local deployment is given an explicit zero cost in the
proxy configuration, or FR-014 is weakened to "a record exists" and the zero-cost claim is dropped.

### Run Trace (Langfuse "trace")

| Field | Source | Notes |
|---|---|---|
| `id` | the platform's activity-run id | FR-010: the cross-reference is the id itself |
| `name` | the platform operation | `tailor`, `match`, `enrich`, … |
| child generations | every call made under that id | ordered by `startTime` (FR-009) |
| total cost / duration | collector aggregate over children | FR-009 |

Retries and escalations are additional child generations under the same trace id, not new traces
(FR-009). Concurrency safety (FR-011) follows from the id being minted per run before the first
call, never derived from time.

### Retention Policy

Not an entity so much as one configured number: a window, default **30 days** (research R7), after
which records and their content are pruned. Stated in `specs/domains/platform-operations.md` so it
is a documented property of the deployment rather than a collector default nobody chose.

---

## 2. Application-side change (US2 only)

One optional field. US1 requires none.

### `domain.CompleteOptions` — `apps/api/internal/platform/llm/domain/port.go`

Add alongside the existing `System`, `Temperature`, `MaxTokens`, `Model`, `ResponseMode`,
`JSONSchema`:

- **`TraceID string`** — an opaque correlation identifier grouping this call with the other calls of
  the same logical run. Empty means "not correlated", which is the pre-036 behaviour and remains
  valid for every call site that does not set it.

Zero value is empty, so every existing caller compiles and behaves unchanged (the same
backward-compatibility discipline `ResponseMode` used in 033).

Accessor, matching the existing `ModelOr` / `Temp` / `SystemPrompt` style:

- **`Trace() string`** — returns the trace id, or empty when `opts` is nil.

### Provider adapters

- **Gateway adapter** (`infrastructure/gateway/gateway.go`): when `Trace()` is non-empty, include a
  `metadata` object on the outbound chat request carrying the trace id. When empty, the field is
  omitted entirely — the request body is byte-identical to today's.
- **Ollama adapter**: ignores the field. Local calls do not traverse the proxy and are out of
  coverage by FR-013.
- **Router** (`application/router.go`): passes `CompleteOptions` through as it already does; no
  change beyond what `withModel` already handles.

### Call sites that set it

**Corrected**: these are not in `service.go`. Six of the seven are free functions in
`rendercv_llm.go` taking `(ctx, lc, model, ...)` with no `*activity.Recorder` in scope, so each needs
a signature change — this is a larger edit than "pass the id into each stage call".

| Call site | File:line | Value passed |
|---|---|---|
| `analyzeVacancy` | `generation/application/rendercv_llm.go:68` | the tailoring run's activity-run id |
| `selectContent` | `rendercv_llm.go:299` | same |
| `writeSummary` | `rendercv_llm.go:352` | same |
| structure re-tailor | `rendercv_llm.go:372` | same |
| `expandContent` | `rendercv_llm.go:386` | same |
| `condenseContent` | `rendercv_llm.go:476` | same |
| cover letter | `generation/application/service.go` | same |
| match | `matching/application/service.go` | the match run's activity-run id |

Retries and escalations reuse the same id (FR-009), which falls out of passing it per run rather than
per call.

Other AI call sites (ghostjob, salary, outreach, recruiter, enrichment) are left uncorrelated in this
feature. They are single-call operations, so a per-run trace adds nothing over the per-call record
US1 already gives them. Adding the field later is a one-line change per call site.

---

## 3. Field mapping: proxy response → call record

```text
request.model  (the task key)     ──► Call Record.metadata.model_group
resolved deployment               ──► Call Record.model
request.metadata.generation_name  ──► Call Record.name            (US2 — else litellm-acompletion)
request.metadata.tags             ──► Call Record.tags            (US2)
request.metadata.existing_trace_id──► Call Record.traceId         (US2 — append, not overwrite)
proxy-computed response cost      ──► Call Record.usage.total_cost
prompt / completion tokens        ──► Call Record.usage.input / .output
proxy-measured wall clock         ──► startTime / endTime
upstream error (all tiers)        ──► level=ERROR, statusMessage
```

**No response header appears on the left.** The callback never sees the HTTP response; the earlier
mapping from `x-litellm-model-name` was wrong on both ends.

**Correction to the Go-side claim.** The first version said `gateway.go` reads `usage.cost`, the token
counts and `x-litellm-model-name` for feature 035's per-stage cost recording. It does not.
`usageFrom` (`gateway.go:100,103,107`) reads **`x-litellm-model-group`**, **`x-litellm-response-cost`**
and **`x-litellm-attempted-fallbacks`**; `x-litellm-model-name` is consumed only by `servedModel`
(`gateway.go:115`) for the log line and `ReportServedModel`. None of this changes what the collector
records — the proxy emits to it directly — but the two paths read different headers and the spec
should not imply otherwise.

---

## 4. Sensitive content

`input` and `output` carry the user's master profile: legal name, employers, dates, contact details,
and any generated document text. Three controls, all specified rather than assumed:

1. **Locality** (FR-005) — the collector runs in the deployment; no third-party egress. Enforced by
   configuration review, not by trust: the collector's outbound configuration is inspected in
   quickstart step 6.
2. **Retention** (FR-008) — 30 days, then pruned.
3. **Credential isolation** (FR-007) — collector keys exist in the collector and proxy container
   environments only. The application never receives them and cannot read them, exactly as provider
   keys work today.

Redaction was considered and rejected (research R7): a trace with the profile stripped out cannot
answer the grounding question that is the main reason to keep traces.

---

## 5. What is deliberately not modelled

- No table, column or migration in the platform's Postgres schema.
- No DTO, no tygo regeneration, no `packages/shared` change — nothing reaches the dashboard.
- No new activity-record fields. The activity-run id already exists and is reused as-is; the trace
  points at the platform, not the other way round.
