-- +goose Up
CREATE TABLE "ActivityRun" (
  "id"         uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "op"         text NOT NULL,
  "state"      text NOT NULL DEFAULT 'queued',
  "label"      text NOT NULL,
  "step"       text,
  "jobId"      uuid,
  "sourceKey"  text,
  "queueTaskId" text,
  "refId"      text,
  "error"      text,
  "meta"       jsonb NOT NULL DEFAULT '{}'::jsonb,
  "createdAt"  timestamp (3) DEFAULT now() NOT NULL,
  "startedAt"  timestamp (3),
  "finishedAt" timestamp (3)
);
ALTER TABLE "ActivityRun" ADD CONSTRAINT "ActivityRun_jobId_Job_id_fk"
  FOREIGN KEY ("jobId") REFERENCES "public"."Job"("id") ON DELETE cascade;
CREATE INDEX "ActivityRun_createdAt_idx" ON "ActivityRun" ("createdAt");
CREATE INDEX "ActivityRun_active_idx" ON "ActivityRun" ("state") WHERE "state" IN ('queued','running');
-- +goose Down
DROP TABLE IF EXISTS "ActivityRun";
