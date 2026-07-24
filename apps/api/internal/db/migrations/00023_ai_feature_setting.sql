-- +goose Up
-- Generalizes AutoGenerateSetting (resume-only) into a per-feature table:
-- every AI feature that can run automatically off a job's match score
-- (resume, cover letter, salary inference) gets its own enabled/threshold
-- row. Below threshold (or disabled), the feature only runs on-demand.
-- Match scoring itself is excluded — it always runs unconditionally.
CREATE TABLE "AiFeatureSetting" (
  "featureKey" text PRIMARY KEY,
  "enabled"    boolean NOT NULL DEFAULT false,
  "threshold"  integer NOT NULL DEFAULT 90,
  "updatedAt"  timestamp(3) NOT NULL DEFAULT now()
);

INSERT INTO "AiFeatureSetting" ("featureKey", "enabled", "threshold")
SELECT 'resume', "enabled", "threshold" FROM "AutoGenerateSetting" WHERE "id" = 'default';

INSERT INTO "AiFeatureSetting" ("featureKey") VALUES
  ('cover_letter'),
  ('salary_infer');

DROP TABLE "AutoGenerateSetting";

-- +goose Down
CREATE TABLE "AutoGenerateSetting" (
  "id"        text PRIMARY KEY DEFAULT 'default' CHECK ("id" = 'default'),
  "enabled"   boolean NOT NULL DEFAULT false,
  "threshold" integer NOT NULL DEFAULT 90,
  "updatedAt" timestamp(3) NOT NULL DEFAULT now()
);

INSERT INTO "AutoGenerateSetting" ("id", "enabled", "threshold")
SELECT 'default', "enabled", "threshold" FROM "AiFeatureSetting" WHERE "featureKey" = 'resume';

DROP TABLE "AiFeatureSetting";
