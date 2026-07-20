# Phase 1 Data Model: Ghost-Job Detector

**One new table (`JobSignal`), one goose migration, one sqlc query file.** Everything else in this document is an existing type this feature *reads* and does not modify.

## New persisted entity: `JobSignal`

A scored judgement about a job, of a named `kind`. `kind = 'ghost'` is the only kind this feature writes; the column exists so future signal kinds (salary-realism, seniority-mismatch, …) reuse this table instead of each adding one.

### Table definition

Goose migration `apps/api/internal/db/migrations/00007_job_signal.sql`. Style follows `00001_init.sql`: quoted PascalCase table, quoted camelCase columns, `uuid` PK defaulting to `gen_random_uuid()`, `timestamp (3)` defaulting to `now()`.

```sql
-- +goose Up
CREATE TABLE "JobSignal" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"jobId" uuid NOT NULL,
	"kind" text NOT NULL,
	"score" integer NOT NULL,
	"signals" jsonb DEFAULT '{}'::jsonb NOT NULL,
	"model" text NOT NULL,
	"createdAt" timestamp (3) DEFAULT now() NOT NULL,
	CONSTRAINT "JobSignal_jobId_kind_unique" UNIQUE("jobId","kind")
);

ALTER TABLE "JobSignal"
	ADD CONSTRAINT "JobSignal_jobId_Job_id_fk"
	FOREIGN KEY ("jobId") REFERENCES "public"."Job"("id")
	ON DELETE cascade ON UPDATE no action;

CREATE INDEX "JobSignal_kind_score_idx" ON "JobSignal" USING btree ("kind","score");

-- +goose Down
DROP TABLE IF EXISTS "JobSignal";
```

### Columns

| Column | Type | Null | Default | Meaning |
|---|---|---|---|---|
| `id` | `uuid` | no | `gen_random_uuid()` | Primary key. |
| `jobId` | `uuid` | no | — | FK → `Job.id`. |
| `kind` | `text` | no | — | Signal family. `'ghost'` for this feature. Not an enum — a new kind must not require a migration. |
| `score` | `integer` | no | — | 0-100 (FR-001). `integer`, matching `MatchResult.score`; the model emits a float and the service rounds before persisting. |
| `signals` | `jsonb` | no | `'{}'` | The measured breakdown + confidence + explanation. Shape below. |
| `model` | `text` | no | — | Producing model identifier, e.g. `qwen2.5:7b`. Mirrors `MatchResult.model` (FR-007). |
| `createdAt` | `timestamp (3)` | no | `now()` | When this row's score was produced. Re-scoring replaces the row, so this doubles as "score age". |

### Constraints and indexes

| Object | Definition | Why |
|---|---|---|
| `JobSignal_pkey` | PK on `id` | Standard, matches every other table. |
| `JobSignal_jobId_kind_unique` | `UNIQUE("jobId","kind")` | **FR-009**: at most one result per job per kind. Re-score is an upsert on this constraint (`ON CONFLICT ("jobId","kind") DO UPDATE`), not an insert — no history accumulates. Note this is deliberately *not* `MatchResult`'s `UNIQUE("jobId")`: one job may hold several kinds at once. |
| `JobSignal_jobId_Job_id_fk` | FK → `Job(id)` `ON DELETE cascade`, `ON UPDATE no action` | **SC-010**: deleting a job removes its signal rows, zero orphans. Byte-identical cascade/no-action pairing to `MatchResult_jobId_Job_id_fk` (`00001_init.sql:110`). |
| `JobSignal_kind_score_idx` | btree `("kind","score")` | Feed/list queries filter by kind then band by score (`kind='ghost' AND score >= 50`). Leading `kind` keeps future kinds from scanning each other's rows. `MatchResult` has the analogous `MatchResult_score_idx`. |

No index on `jobId` alone — the unique constraint's index is `("jobId","kind")` and `jobId` is its leading column, so per-job lookups are already served.

### `signals` JSON shape

Written by the service, read by the API and the dashboard. Mirrors the LLM's structured output plus the deterministic measurements handed to it.

```json
{
  "repostCount": 3,
  "daysOpen": 71,
  "crossBoardCount": 2,
  "alwaysHiringCount": 9,
  "confidence": 0.72,
  "explanation": "Reposted 3 times across runs and open 71 days; 9 other postings from this company in the last 90 days never progressed past discovery.",
  "notes": {
    "daysOpen": "measured",
    "crossBoard": "measured",
    "alwaysHiring": "measured",
    "repost": "measured"
  }
}
```

| Key | Type | Unknown-value contract |
|---|---|---|
| `repostCount` | `int` | Always measurable (≥1: the job's own appearance). Never null. |
| `daysOpen` | `int \| null` | `null` when `Job.postedAt` is null (edge case: no posting date). Emits `notes.daysOpen = "unknown: no postedAt"` and lowers `confidence` (FR-011, SC-005). |
| `crossBoardCount` | `int \| null` | `null` when `Job.description` is empty or below the minimum length for a meaningful hash. |
| `alwaysHiringCount` | `int \| null` | `null` when `Job.company` is empty, whitespace, punctuation-only, or the ingestion placeholder `"Unknown"` (edge case: unparseable company). **Never** group placeholder companies together. |
| `confidence` | `float` 0-1 | Required. Reduced once per `null` signal. |
| `explanation` | `string` | Plain English, grounded only in the numbers above (FR-019). |
| `notes` | `object<string,string>` | Per-signal provenance: `"measured"` or `"unknown: <reason>"`. Makes SC-003's "explicit unknown" auditable. |

**Every-signal-unknown case**: when `daysOpen`, `crossBoardCount`, and `alwaysHiringCount` are all `null` and `repostCount` is 1, the service declines to score — no row is written, no LLM call is made (spec edge case; SC-003 stays true because no partial row exists).

## New in-process types (not persisted)

### `GhostSignals` — the deterministic measurement bundle

Computed by SQL before any model call. Handed to the prompt verbatim so the model blends rather than invents (FR-019).

| Field | Type | Source of the measurement |
|---|---|---|
| `RepostCount` | `int` | Count of ingestion runs in which a job with this `dedupeKey` was seen. |
| `DaysOpen` | `*int` | `now() - Job.postedAt`, whole days. `nil` when `postedAt` is null. |
| `CrossBoardCount` | `*int` | Count of *distinct* `Job.sourceKey` values, other than this job's own, whose description hashes to a near-identical value within 60 days. |
| `AlwaysHiringCount` | `*int` | Count of `Job` rows with the same `lower(company)`, ingested in the last 90 days, still in `status = 'found'` with no progressed `Application`. |
| `Notes` | `map[string]string` | Provenance per signal; becomes `signals.notes`. |

### `GhostJobResult` — the LLM's structured output

Mirrors `matching.FitResult` (`apps/api/internal/matching/types.go`) exactly in shape and discipline: a plain struct with `json` + `jsonschema` tags, plus a `Validate()` method reproducing the range check that `llm.CompleteStructured`'s retry loop enforces.

| Field | Type | Notes |
|---|---|---|
| `Score` | `float64` | `jsonschema:"minimum=0,maximum=100"`. Rounded to `int` before persisting. |
| `Confidence` | `float64` | `jsonschema:"minimum=0,maximum=1"`. |
| `Explanation` | `string` | Plain-English reasoning. |
| `TopSignals` | `[]string` | Which signals drove the score, model's own ranking. |

`Validate()` returns an error when `Score` is outside 0-100 or `Confidence` outside 0-1. **FR-010**: a result failing validation after the retry budget persists nothing; any prior row survives untouched.

## Reused existing types (read-only — none modified)

### `Job` (`00001_init.sql:31`)

| Column | Used for | Contract this feature depends on |
|---|---|---|
| `dedupeKey` | Repost signal | `sha256(lower(company) \| lower(title) \| canonicalUrl)`, computed by `ingestion.DedupeKey` (`apps/api/internal/ingestion/dedupe.go`). **FR-003**: the detector MUST call that same function / join on that same column — never recompute the identity independently, or the detector and ingestion will disagree about what a repost is. Carries `Job_dedupeKey_unique`. |
| `postedAt` | Days-open signal | `timestamp (3)`, **nullable**. Null is the documented no-posting-date edge case, not an error. |
| `company` | Always-hiring signal | `text NOT NULL`, but its *value* may be the ingestion placeholder `"Unknown"` (adapters fall back to it per spec 001 FR-006). Grouping is on `lower(company)`; placeholder and blank values are excluded, not grouped. |
| `description` | Cross-board signal | `text NOT NULL`, may be a short teaser before enrichment. Below the minimum length the signal is skipped. |
| `sourceKey` | Cross-board signal | Distinguishes "same JD on a different board" from "same JD seen twice on one board". |
| `status` | Always-hiring signal | `text NOT NULL DEFAULT 'found'`. `'found'` means never progressed. |
| `id` | FK target | — |

**No column is added to `Job`.** The ghost score lives entirely in `JobSignal`.

### `Application` (`00001_init.sql:7`)

Read for FR-006's progression test. `Application.status` is `text NOT NULL DEFAULT 'shortlisted'`, constrained in Go to `dto.ApplicationStatus` (`apps/api/internal/dto/dto.go:17`):

| Status | Counts as progression? |
|---|---|
| `found` | **No** — initial discovery. |
| `shortlisted` | Yes |
| `docs_generated` | Yes |
| `applied` | Yes |
| `interview` | Yes |
| `offer` | Yes |
| `rejected` | Yes — the posting *did* progress and got an answer; it is evidence of a real process, not of ghosting. |

`Application` carries `UNIQUE("jobId")`, so the progression test is a plain left join, not an aggregate. Status transitions are appended to `Application.events` by `applications.Service.Update` (`apps/api/internal/applications/service.go:71-74`), which also mirrors the new status onto `Job.status` (line 100) — hence `Job.status = 'found'` is a valid fast path, with the `Application` join as the authoritative check.

### `MatchResult` (`00001_init.sql:61`)

**Read for nothing, written never.** Listed only to state the boundary: **FR-008** means the ghost score is not a column on `MatchResult` and re-running fit scoring must not touch `JobSignal`. The two tables share only `jobId`. Their unique constraints differ on purpose — `MatchResult` is `UNIQUE("jobId")` (one fit score, full stop), `JobSignal` is `UNIQUE("jobId","kind")` (one row per kind).

### `dto` / `packages/shared`

Per Constitution Principle III, the ghost DTO is authored in Go and regenerated to TypeScript via tygo — never hand-written on the TS side. Adding `JobSignal` to the job/detail DTOs is a generated-type change, so `packages/shared` must be rebuilt before the dashboard compiles.

## Query surface (sqlc — `apps/api/internal/db/queries/jobsignal.sql`)

| Query | Purpose |
|---|---|
| `UpsertJobSignal` | `INSERT … ON CONFLICT ("jobId","kind") DO UPDATE SET score, signals, model, "createdAt" = now()` — FR-009's replace-don't-accumulate. |
| `GetJobSignal` | One row by `jobId` + `kind`, for the detail panel. |
| `ListJobSignalsByJobIds` | Batch fetch for the feed, so the list endpoint issues one query, not N. |
| `CountRepostsByDedupeKey` | Repost signal. |
| `CountCrossBoardDuplicates` | Cross-board signal, 60-day window, distinct `sourceKey`. |
| `CountAlwaysHiringByCompany` | Always-hiring signal, 90-day window, `lower(company)`, excluding progressed jobs. |

## Validation rules (traced to spec requirements)

- **FR-001 / FR-010**: `score` is `NOT NULL integer` in 0-100. Out-of-range results are discarded before the write, so the DB never holds an invalid score. A `CHECK` constraint is *not* added — validation is enforced in `GhostJobResult.Validate()`, matching how `MatchResult.score` is handled today.
- **FR-007**: `score`, `model`, `createdAt` are all `NOT NULL`; `signals` defaults to `{}` but the service always writes a full object. SC-003's "no partial rows" is upheld by the upsert writing every field in one statement.
- **FR-008**: no shared row, no shared constraint, no shared write path with `MatchResult`.
- **FR-009**: `UNIQUE("jobId","kind")` + upsert.
- **FR-011**: `signals.confidence` is required; each `null` signal lowers it and records a reason in `signals.notes`.
- **FR-015**: no column anywhere records a suppression, hide, or auto-action. The schema has no way to express one, which is the point.
- **SC-004**: `alwaysHiringCount` counts the job's *own* company cohort; a value of 1 (the job itself) MUST be treated as no evidence by the prompt and asserted in tests.
- **SC-010**: `ON DELETE cascade` on the FK.
