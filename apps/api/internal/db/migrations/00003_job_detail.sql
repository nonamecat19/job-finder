-- +goose Up
-- "detailScrapedAt" IS NULL marks a Job row as list-only (shallow) data that
-- still needs its detail page fetched. Set once the detail enrichment task
-- fills in description/salary/location/postedAt.
ALTER TABLE "Job" ADD COLUMN "detailScrapedAt" timestamp (3);
CREATE INDEX "Job_detail_pending_idx"
  ON "Job" ("sourceKey") WHERE "detailScrapedAt" IS NULL;

-- +goose Down
DROP INDEX IF EXISTS "Job_detail_pending_idx";
ALTER TABLE "Job" DROP COLUMN IF EXISTS "detailScrapedAt";
