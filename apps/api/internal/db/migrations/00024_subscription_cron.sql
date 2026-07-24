-- +goose Up
-- Subscriptions carried a "lastRunAt" from the start but nothing ever set it
-- on a schedule: the ingestion scheduler only walked SavedSearch, so a
-- subscription was scraped only when someone pressed Run. Give it the same
-- cron column SavedSearch has so the scheduler can treat both the same way.
--
-- Default matches SavedSearch's own default cron ("0 */6 * * *"), so existing
-- rows start on a six-hourly slot rather than all firing at once on deploy.
ALTER TABLE "Subscription" ADD COLUMN "cron" text NOT NULL DEFAULT '0 */6 * * *';

-- +goose Down
ALTER TABLE "Subscription" DROP COLUMN IF EXISTS "cron";
