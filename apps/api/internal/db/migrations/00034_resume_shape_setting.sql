-- +goose Up
-- Configurable resume generation shape: how long the summary is, how many
-- bullets each experience entry keeps, whether skills/projects render at all,
-- how many projects and project bullets survive, and how many pages the render
-- loop aims for. Singleton row ("id" fixed to 'default') so a read never has
-- to handle "no row yet", same guard AutoGenerateSetting used (00019).
--
-- 0 means "unlimited / no limit" for "skillsMaxGroups", "projectsMin",
-- "projectsMax" and "projectBulletsMax". The seeded defaults reproduce the
-- shape the pipeline hardcoded before this table existed, so a fresh install
-- and an upgraded install generate identical documents.
CREATE TABLE "ResumeShapeSetting" (
  "id"                   text PRIMARY KEY DEFAULT 'default' CHECK ("id" = 'default'),
  "summaryLines"         integer NOT NULL DEFAULT 4  CHECK ("summaryLines" BETWEEN 1 AND 12),
  "skillsEnabled"        boolean NOT NULL DEFAULT true,
  "skillsMaxGroups"      integer NOT NULL DEFAULT 0  CHECK ("skillsMaxGroups" BETWEEN 0 AND 20),
  "experienceBulletsMin" integer NOT NULL DEFAULT 8  CHECK ("experienceBulletsMin" BETWEEN 1 AND 20),
  "experienceBulletsMax" integer NOT NULL DEFAULT 10 CHECK ("experienceBulletsMax" BETWEEN 1 AND 20),
  "targetPages"          integer NOT NULL DEFAULT 2  CHECK ("targetPages" BETWEEN 1 AND 3),
  "projectsEnabled"      boolean NOT NULL DEFAULT true,
  "projectsMin"          integer NOT NULL DEFAULT 0  CHECK ("projectsMin" BETWEEN 0 AND 20),
  "projectsMax"          integer NOT NULL DEFAULT 0  CHECK ("projectsMax" BETWEEN 0 AND 20),
  "projectBulletsMax"    integer NOT NULL DEFAULT 0  CHECK ("projectBulletsMax" BETWEEN 0 AND 10),
  "updatedAt"            timestamp(3) NOT NULL DEFAULT now()
);

INSERT INTO "ResumeShapeSetting" ("id") VALUES ('default');

-- +goose Down
DROP TABLE IF EXISTS "ResumeShapeSetting";
