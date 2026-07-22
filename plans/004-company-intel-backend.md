# Plan: 004 Company Intel — Backend Implementation

**Status**: Frontend done, backend missing. The dashboard calls `GET /companies/{jobId}/intel` and `POST /companies/{jobId}/intel/refresh` but no Go handler exists.

**Spec**: `specs/004-company-intel-card/spec.md`
**Plan**: `specs/004-company-intel-card/plan.md`

## What Exists

| Layer | Status |
|-------|--------|
| SQL migration `00007_company_intel.sql` | Done — `Company` + `CompanySignal` tables |
| Dashboard `CompanyIntelCard.tsx` | Done — full component with tests |
| Dashboard hooks (`useCompanyIntel`, `useRefreshCompanyIntel`) | Done |
| Dashboard API client (`api.companies.intel()`, `api.companies.refresh()`) | Done |
| Shared types (`CompanyIntelDto`) | Done |
| Go `internal/companyintel/` package | **Missing** |
| Go HTTP handler routes | **Missing** |

## Tasks

### 1. Create `apps/api/internal/companyintel/` package

- [ ] **1.1** `scraper.go` — Scraper interface + registry (maps `kind → scraper`)
- [ ] **1.2** `scrape_crunchbase.go` — Crunchbase public company profile scraper
- [ ] **1.3** `scrape_layoffs.go` — layoffs.fyi scraper
- [ ] **1.4** `scrape_glassdoor.go` — Glassdoor public review page scraper (best-effort, fail-closed)
- [ ] **1.5** `scrape_headcount.go` — Company About page headcount scraper
- [ ] **1.6** `scrape_techstack.go` — BuiltWith + fallback to job posting skills
- [ ] **1.7** `service.go` — Orchestrates all scrapers, upserts Company + CompanySignal rows
- [ ] **1.8** `scraper_test.go` — Unit tests over saved HTML fixtures
- [ ] **1.9** `testdata/` — Saved HTML fixtures per source

### 2. Create sqlc queries

- [ ] **2.1** `apps/api/internal/db/queries/company.sql` — UpsertCompany, GetCompanyByName
- [ ] **2.2** `apps/api/internal/db/queries/company_signal.sql` — UpsertCompanySignal, ListCompanySignals
- [ ] **2.3** Run `sqlc generate`

### 3. Wire HTTP endpoints

- [ ] **3.1** `GET /api/jobs/{id}/company-intel` — Returns all CompanySignal rows for the job's company
- [ ] **3.2** `POST /api/jobs/{id}/company-intel/refresh` — Re-scrapes all sources, upserts, returns fresh signals
- [ ] **3.3** Register routes in `apps/api/internal/httpapi/router.go`
- [ ] **3.4** Wire service in `apps/api/cmd/server/main.go`

### 4. Verify

- [ ] **4.1** `go test ./internal/companyintel/...` passes
- [ ] **4.2** Dashboard CompanyIntelCard renders data from the API
- [ ] **4.3** Refresh button works end-to-end

## Dependencies

- SQL migration `00007_company_intel.sql` already applied
- Existing `internal/scraping.Service` for HTTP fetch
- Existing `goquery` for HTML parsing
