-- +goose Up
CREATE TABLE "JobSignal" (
  "id"        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "jobId"     uuid NOT NULL REFERENCES "Job"("id") ON DELETE CASCADE,
  "kind"      text NOT NULL,
  "score"     int NOT NULL,
  "signals"   jsonb NOT NULL,
  "model"     text,
  "createdAt" timestamp(3) NOT NULL DEFAULT now(),
  UNIQUE("jobId", "kind")
);

CREATE INDEX "JobSignal_jobId_idx" ON "JobSignal"("jobId");
CREATE INDEX "JobSignal_kind_idx" ON "JobSignal"("kind");

-- +goose Down
DROP INDEX IF EXISTS "JobSignal_kind_idx";
DROP INDEX IF EXISTS "JobSignal_jobId_idx";
DROP TABLE IF EXISTS "JobSignal";
