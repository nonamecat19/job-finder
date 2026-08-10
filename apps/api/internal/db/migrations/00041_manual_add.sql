-- +goose Up
-- Manual vacancy add (041). Two changes, both additive:
--   1. Subscription gains a kind, so a non-crawling "Manual" row can exist alongside
--      the saved-search rows without ever being scheduled.
--   2. SourceRun gains a subscription link and a trigger, so a manual add is recorded
--      like any ingest run yet excluded from source-health accounting.
ALTER TABLE "Subscription"
    ADD COLUMN "kind" text NOT NULL DEFAULT 'crawl';

ALTER TABLE "Subscription"
    ADD CONSTRAINT "Subscription_kind_check" CHECK ("kind" IN ('crawl', 'manual'));

-- At most one manual subscription per source.
CREATE UNIQUE INDEX "Subscription_manual_per_source_idx"
    ON "Subscription" ("sourceKey") WHERE "kind" = 'manual';

ALTER TABLE "SourceRun"
    ADD COLUMN "subscriptionId" uuid,
    ADD COLUMN "trigger" text NOT NULL DEFAULT 'scheduled';

ALTER TABLE "SourceRun"
    ADD CONSTRAINT "SourceRun_subscriptionId_Subscription_id_fk"
    FOREIGN KEY ("subscriptionId") REFERENCES "public"."Subscription"("id")
    ON DELETE SET NULL ON UPDATE no action;

ALTER TABLE "SourceRun"
    ADD CONSTRAINT "SourceRun_trigger_check" CHECK ("trigger" IN ('scheduled', 'manual'));

CREATE INDEX "SourceRun_subscriptionId_idx" ON "SourceRun" ("subscriptionId");

-- The 'manual' JobSource backs fill-in vacancies on hosts with no reader (FR-012a).
-- Inserted here so the first fill-in save has a source to reference; the registered
-- ManualAdapter makes it visible in the sources list.
INSERT INTO "JobSource" ("key", "kind", "enabled", "config", "healthy")
VALUES ('manual', 'manual', true, '{}'::jsonb, true)
ON CONFLICT ("key") DO NOTHING;

-- +goose Down
DROP INDEX IF EXISTS "SourceRun_subscriptionId_idx";
ALTER TABLE "SourceRun" DROP CONSTRAINT IF EXISTS "SourceRun_trigger_check";
ALTER TABLE "SourceRun" DROP CONSTRAINT IF EXISTS "SourceRun_subscriptionId_Subscription_id_fk";
ALTER TABLE "SourceRun" DROP COLUMN IF EXISTS "trigger";
ALTER TABLE "SourceRun" DROP COLUMN IF EXISTS "subscriptionId";
DROP INDEX IF EXISTS "Subscription_manual_per_source_idx";
ALTER TABLE "Subscription" DROP CONSTRAINT IF EXISTS "Subscription_kind_check";
ALTER TABLE "Subscription" DROP COLUMN IF EXISTS "kind";
-- The 'manual' JobSource row and any vacancies under it are intentionally left in
-- place: dropping the source would cascade-delete real vacancies the operator entered
-- by hand. Down is for rolling back the schema, not for discarding the operator's data.
