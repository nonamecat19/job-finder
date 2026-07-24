-- +goose Up
CREATE TABLE "StarStory" (
  "id"         uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "profileId"  uuid NOT NULL,
  "title"      text NOT NULL,
  "situation"  text NOT NULL,
  "task"       text NOT NULL,
  "action"     text NOT NULL,
  "result"     text NOT NULL,
  "skills"     jsonb NOT NULL DEFAULT '[]'::jsonb,
  "categories" jsonb NOT NULL DEFAULT '[]'::jsonb,
  "createdAt"  timestamp (3) DEFAULT now() NOT NULL
);

ALTER TABLE "StarStory"
  ADD CONSTRAINT "StarStory_profileId_Profile_id_fk"
  FOREIGN KEY ("profileId") REFERENCES "public"."Profile"("id") ON DELETE CASCADE;

CREATE INDEX "StarStory_profileId_idx" ON "StarStory" ("profileId");

-- +goose Down
DROP INDEX "StarStory_profileId_idx";
ALTER TABLE "StarStory" DROP CONSTRAINT "StarStory_profileId_Profile_id_fk";
DROP TABLE "StarStory";
