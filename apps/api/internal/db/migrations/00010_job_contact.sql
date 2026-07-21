-- +goose Up
CREATE TABLE "JobContact" (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "jobId" uuid NOT NULL REFERENCES "Job"(id) ON DELETE cascade,
  name text NOT NULL,
  title text,
  "linkedInUrl" text,
  email text,
  phone text,
  source text NOT NULL,
  confidence float NOT NULL,
  "fetchedAt" timestamp(3) NOT NULL DEFAULT now(),
  UNIQUE("jobId", source, name)
);
CREATE INDEX "JobContact_jobId_idx" ON "JobContact"("jobId");

-- +goose Down
DROP INDEX "JobContact_jobId_idx";
DROP TABLE "JobContact";
