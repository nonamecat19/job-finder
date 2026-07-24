-- +goose Up
-- Threads which Subscription a Job was found under, so the feed can filter
-- by subscription (not just sourceKey) and Sources can deep-link into it.
-- Nullable + ON DELETE SET NULL: jobs outlive a deleted subscription, mirroring
-- how Job.sourceKey already isn't cascaded away on JobSource changes.
ALTER TABLE "Job" ADD COLUMN "subscriptionId" uuid;
ALTER TABLE "Job" ADD CONSTRAINT "Job_subscriptionId_Subscription_id_fk" FOREIGN KEY ("subscriptionId") REFERENCES "public"."Subscription"("id") ON DELETE SET NULL ON UPDATE no action;
CREATE INDEX "Job_subscriptionId_idx" ON "Job" USING btree ("subscriptionId");

-- +goose Down
DROP INDEX IF EXISTS "Job_subscriptionId_idx";
ALTER TABLE "Job" DROP CONSTRAINT IF EXISTS "Job_subscriptionId_Subscription_id_fk";
ALTER TABLE "Job" DROP COLUMN IF EXISTS "subscriptionId";
