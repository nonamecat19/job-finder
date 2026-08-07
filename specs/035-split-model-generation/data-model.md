# Phase 1 Data Model: Split-Model Resume Generation

## 1. Stage payload types

`TailoredSections` currently carries the summary alongside the selection, which is what lets one
model produce both. Splitting the type is what makes the stage boundary unrepresentable-if-violated
rather than merely forbidden.

### TailoredSelection (produced by `generation-select`)

`apps/api/internal/generation/domain/rendercv.go`

| Field | Type | Notes |
|---|---|---|
| `Skills` | `[]TailoredSkillGroup` | unchanged; one entry per master group, same indexes |
| `Experience` | `[]TailoredExperience` | unchanged; keyed by exact master company name |
| `Projects` | `[]TailoredProject` | unchanged |
| `SkillGroupsToAdd` | `[]TailoredSkillGroupAdd` | unchanged, optional |
| `SkillGroupsToRemove` | `[]string` | unchanged, optional |
| `SkillChanges` | `[]SkillChange` | unchanged, optional |

**No `Summary` field.** The page-fit stages (`expandContent`, `condenseContent`) use this type, so
FR-010's immutability is enforced by the schema (research R6).

### TailoredSummary (produced by `generation-summary`)

| Field | Type | Validation |
|---|---|---|
| `Summary` | `string` | non-empty; sentence count within `summaryRange(cfg)`; must open with the derived years figure (existing 028 rule) |

### TailoredSections (merged, unchanged externally)

Retained as the shape `MergeTailored` consumes and the render path expects, assembled from a
`TailoredSelection` plus a `TailoredSummary`. Existing callers and stored `content` payloads keep
working; no data migration of historical documents.

## 2. Stage outcome

`StageOutcome` — in-memory per stage, persisted as activity metadata and partly onto the document.

| Field | Type | Notes |
|---|---|---|
| `Stage` | `string` | `analyze` \| `select` \| `summary` \| `page-fit`. `page-fit` is a re-invocation of the selection stage (FR-001), labelled separately only so its cost and latency are attributable |
| `RequestedKey` | `string` | task key sent to the gateway |
| `ServedModel` | `string` | from `x-litellm-model-name` / response `model` (existing capture) |
| `Substituted` | `bool` | served model is not the key's tier-1 deployment |
| `Escalated` | `bool` | selection stage re-run on the premium router (FR-007) |
| `DurationMs` | `int64` | |
| `CostUSD` | `float64` | from gateway `usage.cost`; 0 when unavailable (research R8) |
| `PromptTokens` / `CompletionTokens` | `int` | |

## 3. Completeness report

`CompletenessReport` — output of the new verifier, `rendercv_completeness.go`. Pure function of
(master, merged, analysis, cfg).

| Field | Type | Notes |
|---|---|---|
| `RequiredMissing` | `[]string` | master skill tokens matching `analysis.RequiredSkills` absent from merged. **Non-empty = shortfall** (FR-006, exact) |
| `NiceToHaveRetained` | `float64` | 0–1; retained share of master tokens matching `NiceToHaveSkills`. **<0.8 = shortfall** |
| `NiceToHaveMissing` | `[]string` | for logging |
| `BulletShortfalls` | `map[string]int` | company → highlights returned, where below `cfg.ExperienceBulletsMin` |
| `StructuralFallback` | `bool` | analysis had no required skills, so the structural check ran instead (research R7) |
| `Shortfall` | `bool` | any of the above triggered |

Validation rules restated from FR-006:

1. `len(RequiredMissing) == 0`
2. `NiceToHaveRetained >= 0.80` (when the master has any nice-to-have matches)
3. every company's highlight count `>= cfg.ExperienceBulletsMin`
4. when `analysis.RequiredSkills` is empty: skill group count equals master's, rule 3 still applies,
   `StructuralFallback = true`

## 4. Persistence

### GeneratedDocument (existing table, new columns)

Current columns: `id, jobId, type, version, content, pdfPath, model, createdAt, company, title,
vacancy`. `model` stays as-is (the model that produced the document overall) for backward
compatibility.

Migration `apps/api/internal/db/migrations/00038_document_stage_provenance.sql` — next free goose
version after 00037:

| Column | Type | Null | Purpose |
|---|---|---|---|
| `summaryModel` | `text` | yes | model that actually wrote the summary |
| `summarySubstituted` | `boolean` | no, default `false` | drives the dashboard marker (FR-012) |
| `selectionModel` | `text` | yes | model that produced the selection. Persisted for operator audit; **not** exposed on the DTO — the dashboard has no use for it and §6 keeps the client surface minimal |
| `selectionEscalated` | `boolean` | no, default `false` | selection re-run on premium (FR-007) |
| `stageCostUsd` | `numeric(10,6)` | yes | summed measured cost for the run (FR-017) |

Nullable/defaulted throughout, so existing rows remain valid and no backfill is required.

### ActivityRun (existing table, no schema change)

Per-stage records go in the existing metadata payload via `activity.Recorder.Step`, one step per
stage carrying `StageOutcome` fields, plus steps for each completeness shortfall, escalation, and
grounding intervention (FR-007, FR-009, FR-016, FR-017).

## 5. State transitions

Selection stage, per run:

```
attempt 1 (economy) ──complete──> proceed
        └──shortfall──> attempt 2 (economy) ──complete──> proceed
                              └──shortfall──> attempt 3 (premium, escalated) ──complete──> proceed
                                                    └──shortfall──> fail run, all shortfalls logged
```

Summary stage, per run:

```
attempt 1 (premium) ──grounded──> merged, immutable for the rest of the run
        └──violation──> attempt 2 (re-prompt) ──grounded──> merged
                              └──violation──> strip offending claim, log, deliver (FR-009)
```

Chain-level substitution is orthogonal: any stage may be served by a fallback tier at any attempt
(FR-011); `Substituted` records it, and for the summary it additionally surfaces to the user.

## 6. Shared TypeScript types

`packages/shared` regenerates from Go via tygo. `GeneratedDocumentDto` gains `summaryModel`,
`summarySubstituted`, `selectionEscalated`, `stageCostUsd`. The dashboard reads
`summarySubstituted` for the FR-012 marker; hand-written duplicates are not permitted
(Constitution III).
