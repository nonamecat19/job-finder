-- +goose Up
ALTER TABLE "Job"
  ADD COLUMN IF NOT EXISTS "experience_level" TEXT,
  ADD COLUMN IF NOT EXISTS "experience_min_years" INTEGER,
  ADD COLUMN IF NOT EXISTS "english_level" TEXT,
  ADD COLUMN IF NOT EXISTS "salary_estimate_raw" TEXT,
  ADD COLUMN IF NOT EXISTS "salary_estimate_min" INTEGER,
  ADD COLUMN IF NOT EXISTS "salary_estimate_max" INTEGER,
  ADD COLUMN IF NOT EXISTS "salary_estimate_currency" TEXT;

-- +goose Down
ALTER TABLE "Job"
  DROP COLUMN IF EXISTS "experience_level",
  DROP COLUMN IF EXISTS "experience_min_years",
  DROP COLUMN IF EXISTS "english_level",
  DROP COLUMN IF EXISTS "salary_estimate_raw",
  DROP COLUMN IF EXISTS "salary_estimate_min",
  DROP COLUMN IF EXISTS "salary_estimate_max",
  DROP COLUMN IF EXISTS "salary_estimate_currency";
