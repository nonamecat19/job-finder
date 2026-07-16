-- +goose Up
ALTER TABLE "GeneratedDocument"
  ALTER COLUMN "jobId" DROP NOT NULL,
  ADD COLUMN "company" text,
  ADD COLUMN "title" text,
  ADD COLUMN "vacancy" text;

-- +goose Down
ALTER TABLE "GeneratedDocument"
  DROP COLUMN "company",
  DROP COLUMN "title",
  DROP COLUMN "vacancy",
  ALTER COLUMN "jobId" SET NOT NULL;
