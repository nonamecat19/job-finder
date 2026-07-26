-- +goose Up
-- Employer roster for the ATS board source family (013): a registered
-- employer's board on one vendor, plus proposed-but-undecided candidates
-- mined from apply URLs already in the feed. Kept separate from JobSource
-- (which stays one row per vendor, matching the existing one-source-per-key
-- model) since the roster is shared across a vendor's many employers.
CREATE TABLE "EmployerBoard" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"vendor" text NOT NULL,
	"employerIdentifier" text NOT NULL,
	"displayName" text NOT NULL,
	"addedVia" text NOT NULL DEFAULT 'pasted',
	"enabled" boolean NOT NULL DEFAULT true,
	"lastSuccessAt" timestamp (3),
	"lastPostingCount" integer NOT NULL DEFAULT 0,
	"consecutiveEmptyRuns" integer NOT NULL DEFAULT 0,
	"createdAt" timestamp (3) NOT NULL DEFAULT now(),
	CONSTRAINT "EmployerBoard_vendor_employerIdentifier_unique" UNIQUE ("vendor", "employerIdentifier")
);

CREATE TABLE "BoardCandidate" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"vendor" text NOT NULL,
	"employerIdentifier" text NOT NULL,
	"inferredFromJobId" uuid,
	"state" text NOT NULL DEFAULT 'proposed',
	"createdAt" timestamp (3) NOT NULL DEFAULT now(),
	"decidedAt" timestamp (3),
	CONSTRAINT "BoardCandidate_vendor_employerIdentifier_unique" UNIQUE ("vendor", "employerIdentifier")
);
ALTER TABLE "BoardCandidate" ADD CONSTRAINT "BoardCandidate_inferredFromJobId_Job_id_fk" FOREIGN KEY ("inferredFromJobId") REFERENCES "public"."Job"("id") ON DELETE SET NULL ON UPDATE no action;

-- Merge support (US3): the source-history a Job has been seen on, appended
-- to (not replaced) when a later ingest merges into an existing row rather
-- than inserting a new one.
ALTER TABLE "Job" ADD COLUMN "seenOnSources" text[] NOT NULL DEFAULT '{}';

-- Per-employer run detail (FR-020, FR-023): board vendor sources fan out
-- over many employers in one run, so the existing found/new columns need a
-- per-employer breakdown alongside the aggregate.
ALTER TABLE "SourceRun" ADD COLUMN "employerDetail" jsonb;

-- +goose Down
ALTER TABLE "SourceRun" DROP COLUMN IF EXISTS "employerDetail";
ALTER TABLE "Job" DROP COLUMN IF EXISTS "seenOnSources";
DROP TABLE IF EXISTS "BoardCandidate";
DROP TABLE IF EXISTS "EmployerBoard";
