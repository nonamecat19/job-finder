# Contract: sqlc Queries

New queries in `internal/db/queries/job.sql` and `internal/db/queries/activityrun.sql`. Regenerate with `make sqlc-generate`; never hand-edit `internal/db/sqlcgen/`.

Existing `GetJobByDedupeKey`, `RecordJobRepost`, `InsertJob`, `FindJobByCompany` and `MergeJobBoard` are **retained**, not deleted — they are used by other call sites and by existing tests. Only the ingest persist path stops calling them.

---

## 1. `GetJobsByDedupeKeys`

```sql
-- name: GetJobsByDedupeKeys :many
-- Batch replacement for the per-posting GetJobByDedupeKey in the ingest
-- persist loop. Returns only the subset of $1 that already exists, so the
-- caller classifies known vs new from one round trip.
SELECT "id", "dedupeKey" FROM "Job" WHERE "dedupeKey" = ANY($1::text[]);
```

**Guarantees**: returns at most one row per input key (backed by `Job_dedupeKey_unique`). Absent keys are simply not returned; the caller must not assume input/output alignment by position.

---

## 2. `FindJobsByCompanies`

```sql
-- name: FindJobsByCompanies :many
-- Batch replacement for FindJobByCompany. One candidate per company: the most
-- recently ingested job from a DIFFERENT source, matching the per-posting
-- query's ORDER BY "ingestedAt" DESC LIMIT 1 semantics.
-- Served by "Job_lower_company_idx" (migration 00032) — the per-posting form
-- had no supporting index and scanned the whole table for every board posting.
SELECT DISTINCT ON (LOWER("company"))
  LOWER("company") AS company_key, "id", "sourceKey", "title"
FROM "Job"
WHERE LOWER("company") = ANY($1::text[])
  AND "sourceKey" != $2
ORDER BY LOWER("company"), "ingestedAt" DESC;
```

**Guarantees**: at most one row per input company. `company_key` is the lowercased company, so the caller correlates by that rather than by position.

**Explicitly out of SQL**: `titlesOverlap` stays in Go (`dedupe.go:44`). It has existing unit coverage and reimplementing its word-overlap heuristic in SQL would duplicate tested logic in an untested place. The query returns candidates; Go decides whether each is a match.

---

## 3. `BulkInsertJobs`

```sql
-- name: BulkInsertJobs :many
-- One statement per chunk, replacing N InsertJob calls. ON CONFLICT DO NOTHING
-- handles both in-batch duplicates that survived Go-side dedup and genuine
-- races with a concurrent run: the loser gets no RETURNING row, so it queues
-- no downstream work and neither run fails.
-- COPY was rejected here precisely because it cannot express ON CONFLICT.
INSERT INTO "Job" (
  "dedupeKey", "sourceKey", "externalId", "title", "company", "location",
  "remote", "salaryRaw", "url", "description", "raw", "postedAt",
  "subscriptionId", "seenOnSources"
)
SELECT
  d, s, e, t, c, l, r, sr, u, ds, rw, pa, $13, ARRAY[s]
FROM unnest(
  $1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[],
  $7::bool[], $8::text[], $9::text[], $10::text[], $11::jsonb[], $12::timestamptz[]
) AS x(d, s, e, t, c, l, r, sr, u, ds, rw, pa)
ON CONFLICT ("dedupeKey") DO NOTHING
RETURNING "id", "dedupeKey";
```

**Guarantees**: `RETURNING` contains exactly the rows this statement inserted. A key absent from the result was either already present or won by a concurrent inserter — in both cases the caller must not queue downstream work for it.

**Caller obligation**: correlate results back to postings by `dedupeKey`. **Row order is not guaranteed** and must not be relied on.

**Null handling**: `externalId`, `location`, `salaryRaw`, `description` and `postedAt` are nullable. Arrays must carry SQL NULL at those positions, not empty strings — `sqlc.yaml` sets `emit_pointers_for_null_types: true`, so the Go side builds `[]*string` / `[]*time.Time`.

---

## 4. `BulkRecordJobReposts`

```sql
-- name: BulkRecordJobReposts :execrows
-- Set-based replacement for N RecordJobRepost calls, plus the retry guard.
-- "lastSeenRunId" IS DISTINCT FROM $3 (not != $3) because the column is NULL
-- for every row predating migration 00032, and NULL != $3 is NULL, which would
-- exclude every pre-existing row from ever being counted.
UPDATE "Job" SET
  "seenCount" = "seenCount" + 1,
  "ingestedAt" = now(),
  "subscriptionId" = COALESCE("subscriptionId", $2),
  "lastSeenRunId" = $3
WHERE "dedupeKey" = ANY($1::text[])
  AND "lastSeenRunId" IS DISTINCT FROM $3;
```

**Guarantees**: idempotent per run. Executing it twice with the same `$3` increments each posting exactly once — this is FR-005.

**Returns** the affected row count, which is the run's true repost total (rows skipped by the guard are excluded).

---

## 5. `BulkMergeJobBoards`

```sql
-- name: BulkMergeJobBoards :execrows
-- Batch form of MergeJobBoard: folds board-vendor postings into an existing
-- job from another source. Arrays are position-aligned; unnest pairs them.
UPDATE "Job" SET
  "url" = x.url,
  "sourceKey" = x.source_key,
  "seenOnSources" = array_append("seenOnSources", x.source_key)
FROM unnest($1::uuid[], $2::text[], $3::text[]) AS x(id, url, source_key)
WHERE "Job"."id" = x.id;
```

**Caller obligation**: the three arrays are position-aligned and must be built in lockstep.

---

## 6. `BulkInsertActivities`

```sql
-- name: BulkInsertActivities :many
-- Batch form of the per-task activity.New insert. Without this the N+1 simply
-- moves from the persist loop to the enqueue loop and SC-002 is not met.
-- Returns "jobId" alongside "id" so the caller correlates by job rather than
-- by row order, which is not guaranteed.
INSERT INTO "ActivityRun" ("kind", "label", "jobId", "status")
SELECT k, l, j, 'queued'
FROM unnest($1::text[], $2::text[], $3::uuid[]) AS x(k, l, j)
RETURNING "id", "jobId", "kind";
```

**Caller obligation**: correlate by `(jobId, kind)`. A single job gets two activity rows (`match` and `ghost`), so `jobId` alone is not a key.

**Schema check required before writing this**: the exact column set of `ActivityRun` must be read from `internal/db/migrations/` and `activity.New` in `internal/activity/` — the columns above are the ones `activity.New` populates, but the table may carry additional NOT NULL columns with defaults. Verify before implementing.

---

## Interaction budget

| Statement | Conditional? |
|---|---|
| `GetJobsByDedupeKeys` | always |
| `FindJobsByCompanies` | only when the chunk has board-vendor postings |
| `BulkInsertJobs` | only when the chunk has new postings |
| `BulkRecordJobReposts` | only when the chunk has known postings |
| `BulkMergeJobBoards` | only when merges were resolved |
| `BulkInsertActivities` | only when postings were inserted |

**Maximum 6 per chunk against SC-002's budget of 10.** Constant with respect to posting count, which is the requirement.
