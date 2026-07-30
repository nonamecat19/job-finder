# API Contract: Djinni Scraping Enhancement

**Feature**: Djinni Scraping Enhancement | **Date**: 2026-07-28

## Affected Endpoints

### GET /api/jobs/{id}

The existing job detail endpoint gains new fields in the response body.

**Response** (new fields only — see [data-model.md](../data-model.md) for full `JobDto`):

```json
{
  "id": "6c800802-9253-4cb8-a512-c778310c2ad2",
  "title": "Full-Stack Developer (React + Node.js)",
  "company": "Novacore",
  "salaryRaw": null,
  "salaryMin": null,
  "salaryMax": null,
  "salaryCurrency": null,
  "salaryConfidence": null,
  "salarySource": null,
  "salaryBelowFloor": false,
  "experienceLevel": "2+ years",
  "experienceMinYears": 2,
  "englishLevel": "B1+",
  "salaryEstimateRaw": "$1500-3000",
  "salaryEstimateMin": 1500,
  "salaryEstimateMax": 3000,
  "salaryEstimateCurrency": "USD",
  "salaryIsEstimated": true
}
```

**Field details**:

| Field | Type | Nullable | Source |
|-------|------|----------|--------|
| `experienceLevel` | string | yes | Regex on Djinni description text |
| `experienceMinYears` | number | yes | Parsed minimum from `experienceLevel` |
| `englishLevel` | string | yes | Regex on Djinni description text |
| `salaryEstimateRaw` | string | yes | `div.salaries-info-link strong#salary-suggestion` on Djinni detail |
| `salaryEstimateMin` | number | yes | `ParseSalaryRaw()` on `salaryEstimateRaw` |
| `salaryEstimateMax` | number | yes | `ParseSalaryRaw()` on `salaryEstimateRaw` |
| `salaryEstimateCurrency` | string | yes | `ParseSalaryRaw()` on `salaryEstimateRaw` |
| `salaryIsEstimated` | boolean | no | Computed: `salaryEstimateMin != null` |

**Notes**:
- `salaryIsEstimated` is `true` when the salary estimate fields are populated, regardless of whether employer-disclosed salary (`salaryMin`/`salaryMax`) is also present. Both can coexist.
- All new fields default to `null`/omitted for jobs from non-Djinni sources.
- The `salaryEstimate*` fields are NOT fed into the existing salary inference pipeline (spec 006). They are a separate signal.
- The `company` field is now populated from the detail page during enrichment, fixing "Unknown" regressions.

### GET /api/jobs (list)

No changes to the list endpoint. New fields are only populated during detail enrichment and returned on the detail endpoint.

### NormalizedJob (source adapter contract)

The `NormalizedJob` struct (used internally between adapters and the ingestion pipeline) gains optional fields. This is an internal contract, not directly exposed via HTTP.

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

## Backward Compatibility

- All new fields are `omitempty` in JSON — existing API consumers see the same response shape with additional optional keys
- `salaryIsEstimated` is always present (`bool`, defaults to `false`)
- Existing `salaryRaw`/`salaryMin`/`salaryMax` fields are unchanged
- The `company` field change is a bugfix — previously "Unknown" → now correct company name
