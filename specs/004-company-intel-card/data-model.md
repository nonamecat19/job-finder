# Data Model: Company Intel Card

**Two new tables. One new migration (`00007_company_intel.sql`).**

## New Tables

### `company`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | `uuid` | `PK DEFAULT gen_random_uuid()` | |
| `name` | `text` | `NOT NULL UNIQUE` | Original casing from the job posting. E.g. `"Acme Corp"` |
| `normalizedName` | `text` | `NOT NULL UNIQUE` | `lower(name)`. Used as the join key from `Job.company`. |
| `website` | `text` | nullable | Parsed from the job posting or from the company page scrape. May be empty. |
| `firstSeenAt` | `timestamptz` | `NOT NULL DEFAULT now()` | When this company was first encountered from any job. |
| `lastRefreshedAt` | `timestamptz` | nullable | Most recent successful signal refresh across all signal kinds. `NULL` if never refreshed. |

**Indexes**:

- `PK` on `id` (implicit via `PRIMARY KEY`)
- `UNIQUE` on `name` (natural key — prevents two rows for "Acme Corp" vs "Acme Corp")
- `UNIQUE` on `normalizedName` (lookup key from `Job.company` — `WHERE normalizedName = lower($1)`)

### `company_signal`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | `uuid` | `PK DEFAULT gen_random_uuid()` | |
| `companyId` | `uuid` | `NOT NULL REFERENCES company(id) ON DELETE CASCADE` | FK to `company`. Cascade delete — removing a company removes all its signals. |
| `kind` | `text` | `NOT NULL` | One of `funding`, `layoffs`, `glassdoor_rating`, `headcount`, `tech_stack`. Check constraint enforces this set. |
| `value` | `jsonb` | `NOT NULL` | Structured signal payload. Shape depends on `kind` (see below). |
| `source` | `text` | `NOT NULL` | URL that was scraped. E.g. `"https://www.crunchbase.com/organization/acme-corp"` |
| `fetchedAt` | `timestamptz` | `NOT NULL` | When this signal's scrape completed. |
| `raw` | `jsonb` | `NOT NULL` | Full scraped response body (HTML or structured data) as a JSON string. Preserved for post-hoc debugging of scrape failures. |

**Indexes**:

- `PK` on `id`
- `UNIQUE(companyId, kind)` — at most one signal per kind per company. `ON CONFLICT (companyId, kind) DO UPDATE` for refresh upserts.
- `INDEX(companyId)` — fast lookup of all signals for a company (though with only 5 kinds per company, this is mainly for FK enforcement performance).
- `CHECK(kind IN ('funding', 'layoffs', 'glassdoor_rating', 'headcount', 'tech_stack'))`

**Signal `value` shapes per kind**:

```jsonc
// funding
{
  "round": "Series B",          // latest round name
  "amount": "30M",              // displayed amount text, may be "Undisclosed"
  "date": "2026-03-15",        // ISO date of the round
  "investors": ["Sequoia", "a16z"]  // may be empty
}

// layoffs
{
  "hasLayoffs": true,
  "count": 120,                // may be null if exact count unknown
  "date": "2026-01-20",        // most recent layoff date
  "summary": "120 employees laid off in restructuring"  // scraped text
}

// glassdoor_rating
{
  "overall": 4.2,              // 0.0 - 5.0
  "recommendToFriend": 78,     // percentage 0-100, may be null
  "ceoApproval": 85,           // percentage 0-100, may be null
  "reviewCount": 1423,         // integer
  "summaryUrl": "https://www.glassdoor.com/Reviews/..."
}

// headcount
{
  "current": 3500,             // most recently scraped headcount
  "previous": 3200,            // previous scrape's headcount, null on first scrape
  "trend": "up",               // "up" | "down" | "flat" | "baseline"
  "source": "About page",      // which page was scraped
  "sourceUrl": "https://company.com/about"
}

// tech_stack
{
  "technologies": [
    {"name": "React", "category": "Frontend Framework"},
    {"name": "PostgreSQL", "category": "Database"},
    {"name": "AWS", "category": "Cloud Provider"}
  ],
  "source": "builtwith",       // "builtwith" | "job_posting" | "none"
  "sourceUrl": "https://builtwith.com/acme-corp"
}
```

## Migration

**File**: `apps/api/internal/db/migrations/00007_company_intel.sql`

```sql
CREATE TABLE company (
    id              uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    name            text NOT NULL UNIQUE,
    normalized_name text NOT NULL UNIQUE,
    website         text,
    first_seen_at   timestamptz NOT NULL DEFAULT now(),
    last_refreshed_at timestamptz
);

CREATE TABLE company_signal (
    id          uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    company_id  uuid NOT NULL REFERENCES company(id) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind IN (
                    'funding', 'layoffs', 'glassdoor_rating',
                    'headcount', 'tech_stack'
                )),
    value       jsonb NOT NULL,
    source      text NOT NULL,
    fetched_at  timestamptz NOT NULL,
    raw         jsonb NOT NULL,
    UNIQUE(company_id, kind)
);

CREATE INDEX idx_company_signal_company_id ON company_signal(company_id);
```

## Reused existing types

### `Job.company` (existing column on the `job` table)

The `Job.company` text field is the join key. On first encounter for a given company name:

1. `SELECT id FROM company WHERE normalized_name = lower($1)`
2. If no row: `INSERT INTO company (name, normalized_name) VALUES ($1, lower($1)) RETURNING id`
3. Associate the returned `companyId` with all subsequent signal operations.

This lookup is cheap (indexed `UNIQUE` on `normalized_name`) and does not require a new column on the `job` table. The `FirstSeenAt` timestamp is set on first insert; `lastRefreshedAt` is updated whenever a refresh produces any new signal.

### `dto.NormalizedJob` — unchanged

No new field on the job DTO. The card fetches company signals by company name separately from the job query.

### `scraping.Service` — reused

The same HTTP fetch service used by job source adapters. Each signal scraper is a function accepting `*scraping.Service`, a company name, and an optional website URL.

## Validation rules (from spec requirements)

- **FR-001/FR-014**: If `Job.company` is empty or whitespace-only after trimming, the card is hidden. No Company row is created.
- **FR-008/FR-009**: `normalizedName = lower(companyName)` is the join key. Exact match only — no fuzzy or phonetic matching in V1. `ON CONFLICT (companyId, kind) DO UPDATE` prevents duplicate signal rows on refresh.
- **FR-011**: All scrapes target public pages. No paid API headers, no authentication tokens, no Cloudflare bypass. If a page returns a non-2xx status, the scraper returns zero data and logs a warning; the signal is omitted from the card (FR-006).
- **FR-015**: Tech stack falls back to parsing the job posting's required-skills section when BuiltWith is unreachable. The `value.source` field distinguishes `"builtwith"` from `"job_posting"` from `"none"`.
