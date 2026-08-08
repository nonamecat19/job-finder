-- +goose Up
-- Which summary-writing option the user picked (034). Singleton row ("id" fixed
-- to 'default') so a read never has to handle "no row yet", the same guard
-- ResumeShapeSetting used (00034) and AutoGenerateSetting before it (00019).
--
-- "optionId" is validated against the catalogue in
-- internal/generation/domain/summary_option.go, not by a CHECK constraint here.
-- A constraint would have to be migrated every time an option is added or
-- renamed, and it would turn a stale value — an option removed between releases
-- — into a write error on a table the user cannot repair. The service resolves
-- an unrecognised id to the default instead, because a routing preference must
-- never be able to fail a resume run.
--
-- The seeded default is 'standard', which routes to the same task key the
-- pipeline used before this feature existed, so an upgraded install generates
-- identical documents until somebody deliberately changes it.
CREATE TABLE "SummaryModelSetting" (
  "id"        text PRIMARY KEY DEFAULT 'default' CHECK ("id" = 'default'),
  "optionId"  text NOT NULL DEFAULT 'standard',
  "updatedAt" timestamp(3) NOT NULL DEFAULT now()
);

INSERT INTO "SummaryModelSetting" ("id") VALUES ('default');

-- +goose Down
DROP TABLE IF EXISTS "SummaryModelSetting";
