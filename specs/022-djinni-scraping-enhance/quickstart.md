# Quickstart: Djinni Scraping Enhancement

**Feature**: Djinni Scraping Enhancement | **Date**: 2026-07-28

## Prerequisites

1. Running Postgres DB (via `make up` from repo root)
2. Backend and frontend dependencies installed
3. Shared package built: `pnpm --filter @job-finder/shared build`

## Validation Scenarios

### Scenario 1: Company Name Extracted Correctly

**Setup**: Ensure the standard Djinni preset is configured to scrape `/jobs/774850-full-stack-developer/` or similar.

**Steps**:
1. Run the migration: `goose -dir apps/api/internal/db/migrations up`
2. Trigger the Djinni adapter to scrape the job: run the ingestion scheduler task or trigger enrichment directly
3. Query the database: `SELECT company FROM "Job" WHERE "externalId" = '774850-full-stack-developer'`

**Expected**: `company` = `"Novacore"`, not `"Unknown"`

### Scenario 2: Experience Level Extracted from Description

**Steps**:
1. Query: `SELECT experience_level, experience_min_years FROM "Job" WHERE "externalId" = '774850-full-stack-developer'`

**Expected**: `experience_level` = `"2+ years"`, `experience_min_years` = `2`

### Scenario 3: English Level Extracted from Description

**Steps**:
1. Query: `SELECT english_level FROM "Job" WHERE "externalId" = '774850-full-stack-developer'`

**Expected**: `english_level` = `"B1+"`

### Scenario 4: Salary Analytics Estimate Captured

**Steps**:
1. Query: `SELECT salary_estimate_raw, salary_estimate_min, salary_estimate_max FROM "Job" WHERE "externalId" = '774850-full-stack-developer'`

**Expected**: `salary_estimate_raw` contains `"$1500-3000"`, `salary_estimate_min` = `1500`, `salary_estimate_max` = `3000`

### Scenario 5: Job Detail API Returns All New Fields

**Steps**:
1. Get the job ID: `SELECT id FROM "Job" WHERE "externalId" = '774850-full-stack-developer'`
2. Hit the API: `curl http://localhost:8080/api/jobs/{id} | jq`

**Expected**: Response includes `experienceLevel`, `experienceMinYears`, `englishLevel`, `salaryEstimateRaw`, `salaryEstimateMin`, `salaryEstimateMax`, `salaryEstimateCurrency`, `salaryIsEstimated`

### Scenario 6: Dashboard Renders New Metadata Components

**Steps**:
1. Start the frontend: `pnpm --filter @job-finder/dashboard dev`
2. Navigate to `http://localhost:5173/jobs/{id}` (the Djinni job from above)
3. Verify the page renders without errors

**Expected**:
- `JobMeta` area shows company name "Novacore" (not "Unknown")
- Experience badge shows "2+ years"
- English badge shows "B1+"
- Salary estimate card shows "$1,500 – $3,000" with "Estimated" label
- All components gracefully omit if data is missing

### Scenario 7: Non-Djinni Jobs Are Unaffected

**Steps**:
1. Open a job from a different source (e.g., Indeed, Glassdoor) in the dashboard

**Expected**: New fields are absent (null) and no empty badges/components are rendered

## Test Commands

```bash
# Backend unit tests (includes regex parsing tests)
make test

# Backend integration tests (full enrichment pipeline)
make test-integration

# Dashboard tests
pnpm --filter @job-finder/dashboard test

# Full lint + test suite
make test-lint
```

## Key Files Changed

See [plan.md](./plan.md) Source Code section for the full file list.
