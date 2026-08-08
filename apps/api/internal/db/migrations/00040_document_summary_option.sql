-- +goose Up
-- Which summary-model option the user picked for a run (034).
--
-- "summaryModel" (00038) already records which model *served* the stage, and
-- that is not the same question. Two options can land on the same upstream
-- after a fallback, so the served model cannot tell the user which option they
-- chose — and the choice is what they made and what the result surface has to
-- show back to them.
--
-- Nullable with no default, so every document written before this column
-- existed stays valid without a backfill and reads as "no recorded choice"
-- rather than falsely claiming the default was picked.
ALTER TABLE "GeneratedDocument"
  ADD COLUMN "summaryOptionId" text;

-- +goose Down
ALTER TABLE "GeneratedDocument"
  DROP COLUMN "summaryOptionId";
