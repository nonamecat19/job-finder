-- +goose Up
CREATE TABLE "KeywordDiff" (
  "id"              uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "jobId"           uuid NOT NULL,
  "matched"         jsonb NOT NULL DEFAULT '[]'::jsonb,
  "missingRequired" jsonb NOT NULL DEFAULT '[]'::jsonb,
  "missingPreferred" jsonb NOT NULL DEFAULT '[]'::jsonb,
  "coveragePct"     double precision,
  "model"           text NOT NULL,
  "createdAt"       timestamp (3) DEFAULT now() NOT NULL,
  CONSTRAINT "KeywordDiff_jobId_unique" UNIQUE("jobId")
);

CREATE TABLE "NormalizedTerm" (
  "id"         uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "source"     text NOT NULL CHECK ("source" IN ('jd', 'resume')),
  "sourceId"   uuid NOT NULL,
  "original"   text NOT NULL,
  "canonical"  text NOT NULL,
  "stemmed"    text NOT NULL,
  "category"   text,
  "createdAt"  timestamp (3) DEFAULT now() NOT NULL
);

CREATE TABLE "SynonymOverride" (
  "id"        uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "alias"     text NOT NULL UNIQUE,
  "canonical" text NOT NULL,
  "createdAt" timestamp (3) DEFAULT now() NOT NULL
);

ALTER TABLE "KeywordDiff"
  ADD CONSTRAINT "KeywordDiff_jobId_Job_id_fk"
  FOREIGN KEY ("jobId") REFERENCES "public"."Job"("id") ON DELETE CASCADE;

CREATE INDEX "KeywordDiff_jobId_idx" ON "KeywordDiff" ("jobId");
CREATE INDEX "NormalizedTerm_source_idx" ON "NormalizedTerm" ("source", "sourceId");
CREATE INDEX "NormalizedTerm_canonical_idx" ON "NormalizedTerm" ("canonical");

-- +goose Down
DROP INDEX "NormalizedTerm_canonical_idx";
DROP INDEX "NormalizedTerm_source_idx";
DROP INDEX "KeywordDiff_jobId_idx";

ALTER TABLE "KeywordDiff" DROP CONSTRAINT "KeywordDiff_jobId_Job_id_fk";

DROP TABLE "SynonymOverride";
DROP TABLE "NormalizedTerm";
DROP TABLE "KeywordDiff";
