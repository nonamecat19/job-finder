-- +goose Up
ALTER TABLE "Profile"
  ADD COLUMN "rendercvConfig" jsonb,
  ADD COLUMN "rendercvYaml" text,
  ALTER COLUMN "document" DROP NOT NULL;

-- +goose Down
ALTER TABLE "Profile"
  DROP COLUMN "rendercvConfig",
  DROP COLUMN "rendercvYaml",
  ALTER COLUMN "document" SET NOT NULL;
