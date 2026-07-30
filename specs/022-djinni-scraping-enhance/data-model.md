# Data Model: Djinni Scraping Enhancement

**Feature**: Djinni Scraping Enhancement | **Date**: 2026-07-28

## Entity: Job

### New Columns (Migration 00031)

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `experience_level` | TEXT | yes | Raw matched experience text, e.g., "2+ years" |
| `experience_min_years` | INTEGER | yes | Parsed minimum years, e.g., 2 |
| `english_level` | TEXT | yes | English proficiency, e.g., "B1+", "Upper-Intermediate" |
| `salary_estimate_raw` | TEXT | yes | Raw salary analytics text, e.g., "$1500-3000" |
| `salary_estimate_min` | INTEGER | yes | Parsed minimum from analytics estimate |
| `salary_estimate_max` | INTEGER | yes | Parsed maximum from analytics estimate |
| `salary_estimate_currency` | TEXT | yes | Currency code, e.g., "USD" |

### Existing Columns (for reference)

The `Job` table already has these related columns:
- `salary_raw` (TEXT) — employer-disclosed salary
- `salary_min`, `salary_max`, `salary_currency` — parsed from employer-disclosed salary via spec 006 pipeline
- `salary_confidence`, `salary_source` — inference metadata

### Validation Rules

- `experience_min_years` must be >= 1 when set
- `salary_estimate_min` must be <= `salary_estimate_max` when both set
- All new fields are independently nullable — absence of one does not imply anything about others

---

## Data Flow: Source → DB → API → Dashboard

### Layer 1: Source Adapter (`apps/api/internal/jobsources/adapters/djinni.go`)

**`DjinniDetailPatch`** (returned by `FetchDetail`):

```go
type DjinniDetailPatch struct {
    Description          string
    SalaryRaw            *string
    Location             *string
    Remote               bool
    PostedAt             *string
    Raw                  map[string]string
    // NEW fields
    Company               string    // from detail page
    ExperienceLevel       *string   // raw text from regex
    ExperienceMinYears    *int      // parsed years
    EnglishLevel          *string   // from regex
    SalaryEstimateRaw     *string   // from salary-suggestion strong
    SalaryEstimateMin     *int      // parsed from estimate
    SalaryEstimateMax     *int      // parsed from estimate
    SalaryEstimateCurrency *string  // parsed from estimate
}
```

Extraction logic in `FetchDetail`:
1. Company: `a[href*="/company-"]` text, fallback to parsing `<title>` (regex: `(.+?) в (.+?) – Djinni`)
2. Experience: regex on plain-text description (see [research.md](./research.md) for patterns), fallback to `?exp=N` from analytics URL
3. English: regex on plain-text description
4. Salary estimate: `div.salaries-info-link strong#salary-suggestion` text, parsed via `ParseSalaryRaw()`

### Layer 2: NormalizedJob (`apps/api/internal/dto/jobs.go`)

New optional fields for the source-agnostic normalized contract:

```go
type NormalizedJob struct {
    // ... existing fields ...
    ExperienceLevel       *string `json:"experienceLevel,omitempty"`
    ExperienceMinYears    *int    `json:"experienceMinYears,omitempty"`
    EnglishLevel          *string `json:"englishLevel,omitempty"`
    SalaryEstimateRaw     *string `json:"salaryEstimateRaw,omitempty"`
    SalaryEstimateMin     *int    `json:"salaryEstimateMin,omitempty"`
    SalaryEstimateMax     *int    `json:"salaryEstimateMax,omitempty"`
    SalaryEstimateCurrency *string `json:"salaryEstimateCurrency,omitempty"`
}
```

### Layer 3: Enrichment Handler (`apps/api/internal/enrichment/handler.go`)

`enrichDjinni()` passes new fields from `DjinniDetailPatch` to `UpdateJobDetail` via the updated params struct.

### Layer 4: SQL (`apps/api/internal/db/queries/job.sql`)

`UpdateJobDetail` gains new columns, each using `COALESCE(sqlc.narg('field'), "field")` to never overwrite non-null existing data with nulls:

```sql
UPDATE "Job" SET
    -- ... existing columns ...
    "company" = COALESCE(NULLIF(sqlc.arg('company'), ''), "company"),
    "experience_level" = COALESCE(sqlc.narg('experience_level'), "experience_level"),
    "experience_min_years" = COALESCE(sqlc.narg('experience_min_years'), "experience_min_years"),
    "english_level" = COALESCE(sqlc.narg('english_level'), "english_level"),
    "salary_estimate_raw" = COALESCE(sqlc.narg('salary_estimate_raw'), "salary_estimate_raw"),
    "salary_estimate_min" = COALESCE(sqlc.narg('salary_estimate_min'), "salary_estimate_min"),
    "salary_estimate_max" = COALESCE(sqlc.narg('salary_estimate_max'), "salary_estimate_max"),
    "salary_estimate_currency" = COALESCE(sqlc.narg('salary_estimate_currency'), "salary_estimate_currency"),
WHERE "id" = sqlc.arg('id')
```

Note: `company` uses `COALESCE(NULLIF(''), ...)` to allow overwriting empty/blank values but not overwriting valid data with empty strings.

### Layer 5: Domain Model (`apps/api/internal/domain/job.go`)

```go
type Job struct {
    // ... existing fields ...
    ExperienceLevel       *string
    ExperienceMinYears    *int
    EnglishLevel          *string
    SalaryEstimateRaw     *string
    SalaryEstimateMin     *int
    SalaryEstimateMax     *int
    SalaryEstimateCurrency *string
}
```

### Layer 6: DTO (`apps/api/internal/dto/jobs.go`)

```go
type JobDto struct {
    // ... existing fields ...
    ExperienceLevel       *string `json:"experienceLevel"`
    ExperienceMinYears    *int    `json:"experienceMinYears"`
    EnglishLevel          *string `json:"englishLevel"`
    SalaryEstimateRaw     *string `json:"salaryEstimateRaw"`
    SalaryEstimateMin     *int    `json:"salaryEstimateMin"`
    SalaryEstimateMax     *int    `json:"salaryEstimateMax"`
    SalaryEstimateCurrency *string `json:"salaryEstimateCurrency"`
    SalaryIsEstimated     bool    `json:"salaryIsEstimated"` // computed: true when salaryEstimate* fields are set
}
```

`SalaryIsEstimated` is a computed field — `true` when `salaryEstimateMin` or `salaryEstimateMax` is non-nil, regardless of whether employer-disclosed salary exists. This lets the dashboard distinguish "this job has an estimate" without checking multiple null fields.

### Layer 7: Shared TS (`packages/shared/src/index.ts`)

```ts
export interface NormalizedJob {
    // ... existing fields ...
    experienceLevel?: string
    experienceMinYears?: number
    englishLevel?: string
    salaryEstimateRaw?: string
    salaryEstimateMin?: number
    salaryEstimateMax?: number
    salaryEstimateCurrency?: string
}

export interface JobDto {
    // ... existing fields ...
    experienceLevel?: string
    experienceMinYears?: number
    englishLevel?: string
    salaryEstimateRaw?: string
    salaryEstimateMin?: number
    salaryEstimateMax?: number
    salaryEstimateCurrency?: string
    salaryIsEstimated: boolean
}
```

### Layer 8: Tygo Generated (`packages/shared/src/generated.ts`)

Regenerated via `tygo generate` from `apps/api`. Types mirror the Go DTOs.

---

## Entity Relationships

No new relationships. All new fields are attributes on the existing `Job` entity. The `NormalizedJob` contract gains optional fields for cross-source compatibility.
