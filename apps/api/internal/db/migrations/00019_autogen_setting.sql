-- +goose Up
-- Auto-generate-on-high-score: when a job's match score crosses this
-- threshold, a resume is auto-enqueued for it (matching/handler.go, after
-- MatchJob returns a score). Singleton row ("id" fixed to 'default') so a
-- read never has to handle "no row yet".
CREATE TABLE "AutoGenerateSetting" (
  "id"        text PRIMARY KEY DEFAULT 'default' CHECK ("id" = 'default'),
  "enabled"   boolean NOT NULL DEFAULT false,
  "threshold" integer NOT NULL DEFAULT 90,
  "updatedAt" timestamp(3) NOT NULL DEFAULT now()
);

INSERT INTO "AutoGenerateSetting" ("id") VALUES ('default');

-- +goose Down
DROP TABLE IF EXISTS "AutoGenerateSetting";
