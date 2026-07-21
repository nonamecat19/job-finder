-- +goose Up
-- Append-only outcome event log for applications (spec 010).
-- "Application"."status" keeps holding the *current* state for fast reads and
-- "Application"."events" keeps holding free-form UI annotations; this table is
-- the queryable, ordered, timestamped history the post-age/response-rate signal
-- aggregates over. The log is additive — it never replaces the status column.
CREATE TABLE "ApplicationOutcome" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"applicationId" uuid NOT NULL,
	"eventType" text NOT NULL CHECK ("eventType" IN ('applied', 'viewed', 'screen', 'offer', 'rejected')),
	"occurredAt" timestamp (3) NOT NULL,
	"recordedAt" timestamp (3) DEFAULT now() NOT NULL,
	"note" text,
	"createdAt" timestamp (3) DEFAULT now() NOT NULL
);

ALTER TABLE "ApplicationOutcome"
	ADD CONSTRAINT "ApplicationOutcome_applicationId_Application_id_fk"
	FOREIGN KEY ("applicationId") REFERENCES "public"."Application"("id") ON DELETE CASCADE;

-- Every read is "this application's events, oldest first"; the signal scans by
-- application. One composite index covers both.
CREATE INDEX "ApplicationOutcome_applicationId_idx"
	ON "ApplicationOutcome" ("applicationId", "occurredAt");

-- Terminal-once idempotency: 'applied' anchors post-age and the response-rate
-- denominator, 'offer'/'rejected' are terminal — at most one of each per
-- application, so a double-submitted transition cannot duplicate them.
-- 'viewed'/'screen' may legitimately recur (multiple screens) and are excluded
-- from the predicate. Single partial index over all three so the write path can
-- name one ON CONFLICT arbiter.
CREATE UNIQUE INDEX "ApplicationOutcome_terminalOnce_idx"
	ON "ApplicationOutcome" ("applicationId", "eventType")
	WHERE "eventType" IN ('applied', 'offer', 'rejected');

-- +goose Down
DROP INDEX "ApplicationOutcome_terminalOnce_idx";
DROP INDEX "ApplicationOutcome_applicationId_idx";

ALTER TABLE "ApplicationOutcome" DROP CONSTRAINT "ApplicationOutcome_applicationId_Application_id_fk";

DROP TABLE "ApplicationOutcome";
