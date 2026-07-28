-- +goose Up
ALTER TABLE "ActivityRun" ADD COLUMN "heartbeatAt" timestamp (3);
ALTER TABLE "ActivityRun" ADD COLUMN "timeoutMs" integer;
CREATE INDEX "ActivityRun_heartbeat_idx" ON "ActivityRun" ("heartbeatAt") WHERE "state" = 'running';
ALTER TABLE "Job" ADD COLUMN "embeddingHash" text;

-- +goose Down
DROP INDEX IF EXISTS "ActivityRun_heartbeat_idx";
ALTER TABLE "ActivityRun" DROP COLUMN "heartbeatAt";
ALTER TABLE "ActivityRun" DROP COLUMN "timeoutMs";
ALTER TABLE "Job" DROP COLUMN "embeddingHash";
