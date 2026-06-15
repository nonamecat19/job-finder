-- +goose Up
-- URL-based subscriptions: a saved-filter URL on a job site (e.g. a djinni
-- subs page or a dou category listing) attached to a JobSource by its key.
-- Follows the PascalCase quoted-identifier convention of 00001_init.sql and
-- the FK-to-JobSource pattern used by "SourceRun".
CREATE TABLE "Subscription" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"sourceKey" text NOT NULL,
	"name" text,
	"url" text NOT NULL,
	"enabled" boolean DEFAULT true NOT NULL,
	"lastRunAt" timestamp (3),
	"createdAt" timestamp (3) DEFAULT now() NOT NULL,
	CONSTRAINT "Subscription_sourceKey_JobSource_key_fk" FOREIGN KEY ("sourceKey") REFERENCES "public"."JobSource"("key") ON DELETE cascade ON UPDATE no action
);
CREATE INDEX "Subscription_sourceKey_idx" ON "Subscription" USING btree ("sourceKey");

-- +goose Down
DROP TABLE IF EXISTS "Subscription";
