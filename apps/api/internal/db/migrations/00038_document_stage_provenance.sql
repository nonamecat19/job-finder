-- +goose Up
-- Records which model served each stage of a split-model generation run, so a
-- substituted summary can be flagged on the dashboard and an escalated
-- selection audited after the fact. Every column is nullable or defaulted, so
-- documents written before the split keep their rows valid without a backfill.
ALTER TABLE "GeneratedDocument"
  ADD COLUMN "summaryModel" text,
  ADD COLUMN "summarySubstituted" boolean NOT NULL DEFAULT false,
  ADD COLUMN "selectionModel" text,
  ADD COLUMN "selectionEscalated" boolean NOT NULL DEFAULT false,
  ADD COLUMN "stageCostUsd" numeric(10,6);

-- +goose Down
ALTER TABLE "GeneratedDocument"
  DROP COLUMN "summaryModel",
  DROP COLUMN "summarySubstituted",
  DROP COLUMN "selectionModel",
  DROP COLUMN "selectionEscalated",
  DROP COLUMN "stageCostUsd";
