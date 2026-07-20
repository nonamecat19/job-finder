# Phase 1 Data Model: Recruiter / Hiring-Manager Resolution

**One new persisted entity: `JobContact`. One new migration. No change to existing tables.** This feature adds a child table hanging off `Job`, plus an in-process DTO the resolution sources emit. This document is the field-level and constraint-level definition.

## New persisted entity: `JobContact`

New table, PascalCase quoted identifiers per the existing convention (see `apps/api/internal/db/migrations/00001_init.sql`). Created by a new goose migration.

### Migration

- **File**: `apps/api/internal/db/migrations/00010_job_contact.sql`
- **Version**: `00010` — follows `00009_salary_inference.sql` (plan 006-2). Goose versions MUST be unique and sequential (Constitution: Technology & Architecture Constraints). Note `00011_jd_keyword.sql` already exists on the branch line; if `00010` is taken by the time this lands, take the next free sequential number and update this doc — never reuse or duplicate a version.
- **Markers**: `-- +goose Up` / `-- +goose Down`.

### Schema

```sql
-- +goose Up
CREATE TABLE "JobContact" (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    "jobId"      uuid NOT NULL REFERENCES "Job"(id) ON DELETE CASCADE,
    name         text NOT NULL,
    title        text,
    "linkedInUrl" text,
    email        text,
    phone        text,
    source       text NOT NULL,
    confidence   float NOT NULL,
    "fetchedAt"  timestamp(3) NOT NULL DEFAULT now(),
    UNIQUE("jobId", source, name)
);
CREATE INDEX "JobContact_jobId_idx" ON "JobContact"("jobId");

-- +goose Down
DROP TABLE "JobContact";
```

### Columns

| Column | Type | Null? | Meaning |
|---|---|---|---|
| `id` | `uuid` | NOT NULL (PK) | Surrogate key, `gen_random_uuid()` default. |
| `jobId` | `uuid` | NOT NULL | FK → `Job(id)`, `ON DELETE CASCADE`. The requisition this contact belongs to. |
| `name` | `text` | NOT NULL | Person's name as observed in the source. Part of the uniqueness key — a contact without a name is not stored as a named row (FR-007). |
| `title` | `text` | NULL | Role/title (`recruiter`, `hiring manager`, `talent partner`, …) when the source states one. |
| `linkedInUrl` | `text` | NULL | Person's LinkedIn profile URL when found. Sensitive (FR-018). |
| `email` | `text` | NULL | Contact email when found and valid. Sensitive (FR-018). |
| `phone` | `text` | NULL | Contact phone when found and valid. Sensitive (FR-018). |
| `source` | `text` | NOT NULL | Producer of the row: `posting`, `company-page`, or `linkedin`. Part of the uniqueness key. |
| `confidence` | `float` | NOT NULL | Producer-assigned confidence in `[0,1]`; drives the headline pick (FR-009) and list ordering (FR-010). |
| `fetchedAt` | `timestamp(3)` | NOT NULL | When this row was resolved; `now()` default. Updated on re-run. |

### Constraints & indexes

- **Primary key**: `id`.
- **Foreign key**: `jobId` → `Job(id)` `ON DELETE CASCADE` — deleting a job removes its contacts (FR-014, SC-009). No orphans possible.
- **Unique**: `(jobId, source, name)` — a re-run of the same source that finds the same person updates that row rather than inserting a duplicate (FR-013, SC-006). Deliberately *includes* `source`, so the same person resolved from both `posting` and `linkedin` is two rows with distinct provenance (spec edge case "same person from two sources").
- **Index**: `JobContact_jobId_idx` on `jobId` — the detail-page read path fetches all contacts for one job (Stories 1 & 3); this index serves that lookup and the FK.

### `source` allowed values

`source` is free `text` (no DB enum), constrained by convention to exactly:

| Value | Producer | Gating |
|---|---|---|
| `posting` | Posting-text LLM parser | Always on. |
| `company-page` | Company team/about-page parser | Runs when `Company.website` is set (plan 004). |
| `linkedin` | LinkedIn company-page People parser | Opt-in: only when `LINKEDIN_SCRAPE_ENABLED=true`. |

## New in-process type (not persisted): `ResolvedContact`

Emitted by each resolution source, consumed by the resolution use-case that writes `JobContact` rows. Shape mirrors the table columns minus the DB-managed ones (`id`, `fetchedAt`). Naming/placement is an implementation concern for the resolution use-case task; this defines its fields.

| Field | Type | Maps to `JobContact` column | Notes |
|---|---|---|---|
| `Name` | `string` | `name` | Empty name ⇒ not a named contact; see FR-007 handling. |
| `Title` | `*string` | `title` | `nil` when the source names no title. |
| `LinkedInURL` | `*string` | `linkedInUrl` | `nil` when absent. |
| `Email` | `*string` | `email` | `nil` when absent or invalid. |
| `Phone` | `*string` | `phone` | `nil` when absent or invalid. |
| `Source` | `string` | `source` | One of `posting` / `company-page` / `linkedin`. |
| `Confidence` | `float64` | `confidence` | `[0,1]`, producer-assigned. |

> **Typed contracts (Constitution Principle III)**: `JobContact` reaches Go through sqlc regeneration, never a hand-written struct. If any contact field is exposed to the dashboard, it flows through the existing tygo path into `packages/shared` — no hand-duplicated TS interface. This spec authors no cross-language type by hand.

## Reused existing types (unchanged)

### `Job` (`apps/api/internal/db/migrations/00001_init.sql:31`)

| Field used | Type | Role here |
|---|---|---|
| `Job.id` | `uuid` | FK target of `JobContact.jobId`. |
| `Job.description` | `text` | Input to the posting-text source (`source='posting'`). |

No column added to `Job`.

### `Company` (from plan 004 — dependency, not created here)

| Field used | Role here |
|---|---|
| `Company.website` | Input URL for the company team-page source (`source='company-page'`). |

The company-page **fetch** is reused from plan 004's `internal/companyintel` package — this feature does not add its own company-page fetcher. Implementation MUST NOT begin until 004-4 has landed.

## Persisted rows touched

| Table | Interaction |
|---|---|
| `JobContact` | New. Insert/upsert on resolution and re-run; cascade-deleted with its `Job`. |
| `Job` | Read only — `id` and `description`. No write, no new column. |
| `Company` (plan 004) | Read only — `website`. No write. |

## Validation rules (from spec requirements)

- **FR-007**: a source that yields a channel (email/phone) but no human name MUST NOT produce a `name`-bearing row. Store as an explicitly unnamed low-confidence row (a sentinel name is *not* a real person's name) or drop — never invent a name to satisfy `name NOT NULL`.
- **FR-008 / Principle II**: every non-null field must trace to observed source text. A test asserts the extractor does not populate a field absent from its input fixture.
- **FR-013**: persistence MUST upsert on `(jobId, source, name)` — insert-or-update, never blind insert. Assert a second identical run leaves row count unchanged.
- **FR-014**: deleting a `Job` must remove its `JobContact` rows. Assert via a cascade test. **Also update `apps/api/internal/db/integration_test.go:truncateAll` to truncate `JobContact` BEFORE `Job`** (FK order), matching the pattern the other child tables follow.
- **FR-016**: zero resolved contacts is a success, not an error — the resolution use-case commits zero rows and returns no error.
- **FR-018**: `email`, `phone`, `linkedInUrl` are sensitive — never logged in full.
