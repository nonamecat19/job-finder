# Contract: Migration `00032_batch_ingest.sql`

**Goose version**: `00032`. Current maximum is `00031_djinni_scraping_enhance.sql`. Per the project constitution, migration versions must be unique and sequential — verify with `ls internal/db/migrations | tail -1` immediately before creating the file, since another in-flight branch may claim `00032` first.

## Up

```sql
-- +goose Up

-- Guards the bulk repost update against double-counting when an ingest task is
-- retried. RecordJobRepost incremented "seenCount" unconditionally, so a retry
-- after a mid-run failure re-counted every posting already stored by the failed
-- attempt — inflating the ghost-job repost signal that 00016 introduced
-- "seenCount" to feed. Nullable with no default: pre-existing rows have no
-- tracked run, and IS DISTINCT FROM handles NULL correctly, so no backfill.
ALTER TABLE "Job" ADD COLUMN "lastSeenRunId" uuid;

-- Serves FindJobsByCompanies, the batched merge-candidate lookup. The previous
-- per-posting FindJobByCompany filtered on LOWER("company") with no supporting
-- index, so every board-vendor posting scanned the whole table. A functional
-- index is required because the predicate is on LOWER("company"), not "company".
CREATE INDEX "Job_lower_company_idx" ON "Job" (LOWER("company"));
```

## Down

```sql
-- +goose Down
DROP INDEX IF EXISTS "Job_lower_company_idx";
ALTER TABLE "Job" DROP COLUMN IF EXISTS "lastSeenRunId";
```

## Properties

| Property | Value |
|---|---|
| Backfill required | **No.** NULL is a correct initial state; the next run counts each posting once. |
| Data loss on down | `lastSeenRunId` values only. Rolling back restores the double-counting bug but loses no posting data. |
| Blocking | `ADD COLUMN` with no default is metadata-only in PostgreSQL 11+ — instant. |
| Index build | Plain `CREATE INDEX` takes an `ACCESS EXCLUSIVE` lock. Acceptable: single-user self-hosted deployment, `Job` is small, migrations run at startup before the API serves traffic. `CONCURRENTLY` is deliberately **not** used — it cannot run inside goose's transaction and offers nothing here. |
| sqlc impact | **Yes.** `models.go` gains `LastSeenRunId`. Run `make sqlc-generate`; the `sqlc-drift` CI job fails otherwise. |
| tygo impact | **No.** No DTO changes, so `packages/shared` is untouched. |

## Verification

```bash
cd apps/api
go run ./cmd/server            # goose applies 00032 at startup
psql "$DATABASE_URL" -c '\d "Job"'          # lastSeenRunId present
psql "$DATABASE_URL" -c '\di "Job_lower_company_idx"'   # index present

# The index is actually used, not merely present:
psql "$DATABASE_URL" -c 'EXPLAIN SELECT id FROM "Job" WHERE LOWER("company") = LOWER($$Acme$$);'
# expect: Index Scan using "Job_lower_company_idx"  — NOT Seq Scan
```

The `EXPLAIN` check is the one that matters. An index that exists but is not chosen by the planner does not satisfy FR-007. On a table small enough for a sequential scan to win, force the check with `SET enable_seqscan = off` to confirm the index is *usable*, and re-verify on a seeded table (`make seed`) that it is *chosen*.
