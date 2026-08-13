-- +goose Up
ALTER TABLE "Job"     ALTER COLUMN "embedding" TYPE vector(1024) USING NULL;
ALTER TABLE "Profile" ALTER COLUMN "embedding" TYPE vector(1024) USING NULL;
ALTER TABLE "Job"     ADD COLUMN "embedModel" text;
UPDATE "Job"     SET "embeddingHash" = NULL;
UPDATE "Profile" SET "embedModel" = NULL;

-- +goose Down
ALTER TABLE "Job"     ALTER COLUMN "embedding" TYPE vector(768) USING NULL;
ALTER TABLE "Profile" ALTER COLUMN "embedding" TYPE vector(768) USING NULL;
ALTER TABLE "Job"     DROP COLUMN "embedModel";
UPDATE "Job"     SET "embeddingHash" = NULL;
UPDATE "Profile" SET "embedModel" = NULL;
