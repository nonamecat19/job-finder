-- +goose Up
CREATE TABLE "Company" (
  "id"              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "name"            text NOT NULL,
  "normalizedName"  text NOT NULL UNIQUE,
  "website"         text,
  "firstSeenAt"     timestamp(3) NOT NULL DEFAULT now(),
  "lastRefreshedAt" timestamp(3)
);

CREATE TABLE "CompanySignal" (
  "id"         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "companyId"  uuid NOT NULL REFERENCES "Company"("id") ON DELETE CASCADE,
  "kind"       text NOT NULL,
  "value"      jsonb NOT NULL,
  "source"     text,
  "fetchedAt"  timestamp(3) NOT NULL DEFAULT now(),
  "raw"        jsonb,
  UNIQUE("companyId", "kind")
);

CREATE INDEX "CompanySignal_companyId_idx" ON "CompanySignal"("companyId");
CREATE INDEX "Company_normalizedName_idx" ON "Company"("normalizedName");

-- FK-by-convention: Job.company → Company.normalizedName
-- (no SQL FK; join by lowercased name, mirroring Job.sourceKey → JobSource.key)

-- +goose Down
DROP INDEX IF EXISTS "Company_normalizedName_idx";
DROP INDEX IF EXISTS "CompanySignal_companyId_idx";
DROP TABLE IF EXISTS "CompanySignal";
DROP TABLE IF EXISTS "Company";
