# Phase 1 Data Model: Salary Inference

**One migration (`00009_salary_inference.sql`): five columns added to `"Job"`, one new table `"SalaryCache"`.** Everything else reuses types that already exist.

The band lives on `"Job"` rather than in a related table because a band describes exactly one posting — a 1:1 relation modelled as a join table would buy nothing and cost a join on the feed's hottest query. The consequence, accepted deliberately: no estimate history. Re-inferring overwrites.

## Schema change — `"Job"` (existing table, extended)

Note the repo's quoted-PascalCase table names and quoted-camelCase columns (`apps/api/internal/db/migrations/00001_init.sql`); the new columns follow that convention, not snake_case.

| Column | Type | Null? | Meaning |
|---|---|---|---|
| `"salaryMin"` | `integer` | nullable | Band minimum, in `"salaryCurrency"`, annualized. `NULL` when no source produced a band (FR-009). |
| `"salaryMax"` | `integer` | nullable | Band maximum, same currency and period as the minimum. Always `>= "salaryMin"` when both are set. |
| `"salaryCurrency"` | `text` | nullable | ISO 4217 code (`USD`, `UAH`, `EUR`, `PLN`). Not a symbol — `$` is ambiguous across USD/CAD/AUD and the parser sees all three. |
| `"salaryConfidence"` | `double precision` | nullable | 0–1. The blended confidence (FR-005). `< 0.3` drives the low-confidence marker (FR-006). |
| `"salarySource"` | `text` | nullable | One of `llm`, `levels-fyi`, `ingested-cache`, `blended`, `posting`. |

> **`posting` is a fifth value beyond the four the task description names.** FR-008 requires a parsed, posting-stated compensation to take precedence over any estimate, and FR-024 requires it to remain distinguishable from one. Folding it into `ingested-cache` would conflate "this posting says it pays X" with "postings like this one pay X" — a distinction Story 3's breakdown exists to surface and SC-003 measures separately from SC-004. Flagged for the implementing task to confirm.

All five are nullable together: a job either has a complete band or none at all. A partial band is a bug, not a state — SC-002 asserts this and the migration cannot express it as a `CHECK` without also permitting the all-null case, so it is enforced at the write boundary and asserted in tests.

### Index

The floor filter runs on every default feed load, so the filter predicate needs support:

```
CREATE INDEX "Job_salary_floor_idx" ON "Job" ("salaryMax") WHERE "salaryMax" IS NOT NULL;
```

Partial, because jobs with no band are never filtered by the floor (FR-019) and so are never probed by this predicate.

### Down migration

Drops the index, then the five columns. Dropping columns discards every stored band; re-inference repopulates them from scratch, at the cost of re-running the model over the corpus. Called out because it is not a free rollback.

## New table — `"SalaryCache"`

The external reference dataset reduced to buckets. Not per-job and not per-posting: one row per (title, geo, company-size) bucket, which is what FR-013 requires a lookup to resolve to.

| Column | Type | Null? | Meaning |
|---|---|---|---|
| `"id"` | `uuid` | PK, default `gen_random_uuid()` | Matches every other table's PK convention. |
| `"titleBucket"` | `text` | not null | Normalized title — lowercased, seniority-stripped, canonicalized (`Sr. Backend Engineer` → `backend engineer`). The normalization function is the load-bearing part; see research.md. |
| `"geoBucket"` | `text` | not null | Country or region code. Coarse by design — city-level buckets are too sparse to populate. `"*"` is the catch-all for postings with no geo (spec edge case). |
| `"companySizeBucket"` | `text` | not null | One of `startup`, `mid`, `large`, `unknown`. `unknown` is the common case, not the exception. |
| `"salaryMin"` | `integer` | not null | Bucket's observed minimum. |
| `"salaryMax"` | `integer` | not null | Bucket's observed maximum. |
| `"currency"` | `text` | not null | ISO 4217. |
| `"sampleSize"` | `integer` | not null | How many source rows the bucket aggregates. **Drives the source's confidence** — a bucket built from 3 rows must not speak as loudly as one built from 300. |
| `"source"` | `text` | not null | Which dataset the bucket came from, so a future second dataset does not require a schema change. |
| `"refreshedAt"` | `timestamp (3)` | not null, default `now()` | Staleness. Compensation data ages; a bucket refreshed 18 months ago should discount its confidence. |

**Unique constraint** on `("titleBucket", "geoBucket", "companySizeBucket", "source")` — the natural key, and what makes the startup load an idempotent upsert rather than an append that grows without bound on every restart.

**Lookup index**: the unique constraint's index serves the exact-bucket lookup. Fallback lookups (widen company size to `unknown`, then geo to `*`) are prefix-compatible with it, so no second index is needed.

### Why a table and not a file

The dataset could be held in memory. It is persisted because: it must survive restart without re-retrieval (FR-012); an unreachable dataset must not prevent startup (FR-010 of the runtime, SC-010), which requires yesterday's copy to still be on disk; and bucket lookups join naturally against `"Job"` for backfill.

## Reused existing types — unchanged

| Type | Location | Role here |
|---|---|---|
| `llm.Provider` | `apps/api/internal/llm` | The self-hosted model runtime. Unchanged — no new provider (Principle V). |
| `llm.CompleteStructured[T]` | `apps/api/internal/llm/types.go:106` | The generic structured-completion path: schema-in-prompt, parse, validate, retry twice. The LLM source calls it as `CompleteStructured[SalaryBand]`. Nothing to add — this is exactly its intended use. |
| `llm.CompleteOptions` | `apps/api/internal/llm/types.go` | Passed through unchanged. |
| `dto.NormalizedJob.SalaryRaw` | `apps/api/internal/dto` | The input to the parser. Stays populated and stays displayed (FR-024). |
| `config.Config` | `apps/api/internal/config/config.go` | Gains `SalaryFloorUsd` alongside the existing `mapstructure`-tagged fields. Viper-loaded; no new mechanism. |
| `JobFilters` | `apps/dashboard/src/lib/api.ts:30` | Gains one optional field for the below-floor toggle. Existing fields untouched. |
| `ListJobsByScore` / `ListJobsByDate` / `CountJobs` | `apps/api/internal/db/queries/joblist.sql` | The floor predicate is added to all three. `CountJobs` must receive the identical predicate or pagination totals will disagree with the page contents. |

## New in-process types — not persisted

### `SalaryBand`

The unit every source emits, and the type parameter to `CompleteStructured`. Its JSON-schema shape is what the model is asked to produce, so field names are part of the prompt contract and are not free to rename.

| Field | Type | Notes |
|---|---|---|
| `Min` | `int` | Annualized. |
| `Max` | `int` | Annualized, `>= Min`. |
| `Currency` | `string` | ISO 4217. |
| `Period` | `string` | `year` or `month` as emitted; normalized to annual before storage. Present because a model asked for a Ukrainian salary will answer monthly unless told otherwise, and a silent monthly-vs-annual mix is a 12× error (spec edge case). |

### `SourceEstimate`

One source's independent answer. Never persisted — it is the input to blending and the content of Story 3's breakdown.

| Field | Type | Notes |
|---|---|---|
| `Source` | `string` | `llm` / `levels-fyi` / `ingested-cache` / `posting`. |
| `Band` | `SalaryBand` | That source's band. |
| `Confidence` | `float64` | 0–1, that source's own. Sources do not see each other's answers (FR-003). |

### `BlendedEstimate`

What gets written to the `"Job"` columns.

| Field | Type | Maps to |
|---|---|---|
| `Band` | `SalaryBand` | `"salaryMin"`, `"salaryMax"`, `"salaryCurrency"` |
| `Confidence` | `float64` | `"salaryConfidence"` |
| `Source` | `string` | `"salarySource"` |
| `Components` | `[]SourceEstimate` | **Not persisted.** Story 3's breakdown is computed on demand, since persisting it would mean either a JSON blob on `"Job"` or the related table the user ruled out. Consequence: the breakdown is only available while the estimate is being computed, or by recomputing. Flagged in plan.md as an open design point. |

## Validation rules (from spec requirements)

- **FR-004/FR-005**: weights are the confidences, normalized; the final confidence is their **sum, capped at 1**. Uncapped, three sources at 0.5 would yield 1.5 — an impossible confidence that would break any threshold comparison downstream.
- **FR-005 corollary**: a single source at confidence *c* blends to exactly *c*. Assert this — a blend of one must be a no-op, and an implementation that normalizes wrongly will silently inflate it to 1.0.
- **FR-002/FR-003**: each source runs independently. A source that errors contributes no estimate and does not abort the others (FR-023).
- **FR-008**: when a parsed posting-stated band exists, it wins outright. It is not blended in at high confidence — it replaces the estimate, and `"salarySource"` records `posting`.
- **FR-009**: zero usable estimates → all five columns stay `NULL`. Never a partial write, never a zero-valued band; a stored `0`–`0` band would be filtered out by every non-zero floor, silently hiding the job.
- **FR-018/FR-019**: floor `0` → the predicate is omitted entirely, not evaluated as `> 0`. `NULL` `"salaryMax"` → never filtered; the predicate must be written so `NULL` passes, which SQL three-valued logic does *not* give for free (`"salaryMax" >= $1` drops `NULL` rows).
- **FR-020**: convert before comparing. A band whose currency has no conversion rate is not filtered out — unfilterable must fail open, never closed.
- **FR-022**: a job with a non-null `"salarySource"` is skipped unless its inputs changed. This is what makes SC-009 (zero model calls on a second run) hold.
- **SC-002**: the five columns are all-null or all-non-null. Assert directly — a partial band is invisible until a filter or a UI component divides by it.
