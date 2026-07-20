-- +goose Up
-- bucket format: '<title-slug>|<geo-slug>|<company-size-bucket>'
--   title-slug: lowercase hyphenated job title (e.g. 'senior-backend')
--   geo-slug: lowercase location slug (e.g. 'us', 'london-uk')
--   company-size-bucket: company size range (e.g. '1-10', '51-200', '1000+')
--   example: 'senior-backend|us|51-200'

ALTER TABLE "Job"
  ADD COLUMN "salaryMin" int,
  ADD COLUMN "salaryMax" int,
  ADD COLUMN "salaryCurrency" text,
  ADD COLUMN "salaryConfidence" float,
  ADD COLUMN "salarySource" text;

CREATE TABLE "SalaryCache" (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bucket text NOT NULL,
  "salaryMin" int,
  "salaryMax" int,
  currency text NOT NULL,
  source text NOT NULL,
  "sampleSize" int NOT NULL DEFAULT 1,
  "updatedAt" timestamp(3) NOT NULL DEFAULT now(),
  UNIQUE(bucket, currency, source)
);
CREATE INDEX "SalaryCache_bucket_idx" ON "SalaryCache"(bucket);

-- +goose Down
ALTER TABLE "Job"
  DROP COLUMN "salarySource",
  DROP COLUMN "salaryConfidence",
  DROP COLUMN "salaryCurrency",
  DROP COLUMN "salaryMax",
  DROP COLUMN "salaryMin";

DROP TABLE IF EXISTS "SalaryCache";
