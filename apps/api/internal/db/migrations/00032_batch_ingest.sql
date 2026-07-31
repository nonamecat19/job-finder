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

-- +goose Down
DROP INDEX IF EXISTS "Job_lower_company_idx";
ALTER TABLE "Job" DROP COLUMN IF EXISTS "lastSeenRunId";
