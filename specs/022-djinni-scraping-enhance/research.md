# Research: Djinni Scraping Enhancement

**Feature**: Djinni Scraping Enhancement | **Date**: 2026-07-28

## Decision 1: Company Name Extraction Fix

**Decision**: Add a new CSS selector `a[href*="/company-"]` to the detail-page scraper (`FetchDetail`), with fallback to `<title>` tag parsing when selectors return empty.

**Rationale**: Analysis of the Djinni detail page (`/jobs/774850-full-stack-developer/`) revealed the company name "Novacore" appears as `<a href="/jobs/company-novacore/" class="text-secondary fw-medium">Novacore</a>`. The existing list-page selector `a[href^="/company/"]` does not match this because detail-page company links use `/jobs/company-{slug}/` (not `/company/{slug}`). The `<title>` tag format (`"JobTitle в CompanyName – Djinni"`) provides a reliable fallback.

**Alternatives considered**:
- Parse company from `<script type="application/ld+json">` structured data — but this is not always present on Djinni pages
- Use `.breadcrumb li:last-child` — breadcrumb shows categories, not company name

---

## Decision 2: Experience Level Extraction

**Decision**: Extract experience level via regex patterns on the full description text (plain text, not HTML), with a structured fallback from the salary analytics URL `?exp=N` parameter.

**Regex patterns** (applied to plain-text description):

| Language | Pattern | Example match |
|----------|---------|---------------|
| English | `(\d+)\+?\s*(?:years?|yrs?\.?)\s*(?:of\s+)?(?:commercial\s+|professional\s+)?(?:full-stack\s+|software\s+)?(?:development\s+)?experience` | "**2+** years of commercial full-stack experience" |
| Ukrainian | `(?:від|не менше|мінімум)?\s*(\d+)\+?\s*(?:рок\S*|р\.|р)(?:\s+досвід)` | "від **2** років досвіду" |
| Ukrainian alt | `досвід\s+(?:роботи\s+)?(?:від\s+)?(\d+)` | "досвід від **3** років" |

**Fallback**: Parse the salary analytics URL `a[href*="/salaries/"]` for the `exp=N` query parameter. This is Djinni's own categorization of the job's experience level.

**Rationale**: The description text is the most direct source. The analytics URL (`/salaries/?category=fullstack&work_format=full_remote&exp=2&english_level=no_english`) contains Djinni's own metadata for the listing. The fallback covers cases where the description uses non-standard phrasing.

**Output**: Two fields — `experienceLevel` (raw matched text, e.g., "2+ years") and `experienceMinYears` (parsed integer, e.g., 2). Store the smaller value when a range is given.

**Alternatives considered**:
- Only parse from analytics URL — loses precision for jobs where the description says "5+ years" but the analytics bucket is `exp=5`
- Use LLM to classify experience level — violates constitution principle V for a simple text extraction task
- Parse structured data from `<script>` tags — Djinni does not consistently include this

---

## Decision 3: English Level Extraction

**Decision**: Extract English proficiency level from the description text using regex patterns covering both English and Ukrainian formulations.

**Regex patterns** (applied to plain-text description):

| Pattern | Matches |
|---------|---------|
| `(?i)english\s*(?:level|proficiency)?\s*[—:\-–]\s*(\S+(?:\s+\S+)?)` | "English level — **B1+**" |
| `(?i)english\s*[—:\-–]\s*(Upper-?Intermediate\|Advanced\|Fluent\|Beginner\|Elementary\|Pre-Intermediate\|Intermediate)` | "English: **Upper-Intermediate**" |
| `(?i)(?:Upper-?Intermediate\|Advanced\|Fluent\|Beginner\|Elementary\|Pre-Intermediate\|Intermediate)\s*(?:level\s*)?(?:english\|англ)` | "**Upper-Intermediate** English" |
| `(?i)(?:Англійська\|англійська)(?:\s*мова)?\s*[—:\-–]\s*(\S+(?:\s+\S+)?)` | "Англійська — **B1+**" |
| `(?i)рівень\s+англійської\s*[—:\-–]\s*(\S+(?:\s+\S+)?)` | "рівень англійської: **Intermediate**" |

**Output**: Single string field `englishLevel` (e.g., "B1+", "Upper-Intermediate").

**Rationale**: English level is embedded in the job description body, not in structured HTML. The patterns cover the two most common formats on Djinni: CEFR scale (A1-C2) codes and descriptive labels (Upper-Intermediate, etc.). Both English and Ukrainian job postings are covered.

**Alternatives considered**:
- Parse from analytics URL `english_level=no_english` — this is binary presence/absence of English requirement, not the proficiency level
- Separate field for language + level — over-complicates the data model for a single use case

---

## Decision 4: Salary Analytics Extraction

**Decision**: Extract the salary analytics estimate from `div.salaries-info-link strong#salary-suggestion` on the detail page in `FetchDetail`. Store it as separate fields (`salaryEstimateRaw`, `salaryEstimateMin`, `salaryEstimateMax`, `salaryEstimateCurrency`) distinct from the employer-disclosed salary.

**Rationale**: The salary analytics widget is a Djinni-specific feature that shows "average salary for similar positions." It appears as:
```html
<div class="salaries-info-link card card-body border-0 bg-light-subtle">
  <div>
    📊
    <strong id="salary-suggestion">$1500-3000</strong>
    <span>Середня зарплатна вилка схожих вакансій у</span>
    <a href="/salaries/?...">аналітиці →</a>
  </div>
</div>
```

The existing `salaryRaw` captures only employer-disclosed salary from `.public-salary-item, .text-success`. The analytics estimate is a different signal and must not be mixed with disclosed salary to avoid misleading users.

The raw analytics text is passed through the existing `ParseSalaryRaw()` function to extract numeric min/max/currency values.

**Alternatives considered**:
- Merge with existing `salaryMin`/`salaryMax` — would confuse disclosed vs. estimated salary and mislead users about what the employer is actually offering
- Store in a separate table — over-engineering for a single widget on a single source

---

## Decision 5: Migration Strategy

**Decision**: Use migration number 00031. Add columns directly to the `Job` table (no new tables).

**New columns on `Job`**:
- `experience_level` TEXT (nullable) — raw text
- `experience_min_years` INTEGER (nullable) — parsed years
- `english_level` TEXT (nullable) — e.g., "B1+"
- `salary_estimate_raw` TEXT (nullable) — raw analytics text
- `salary_estimate_min` INTEGER (nullable)
- `salary_estimate_max` INTEGER (nullable)
- `salary_estimate_currency` TEXT (nullable)

**Rationale**: These fields are core job metadata, not a separate entity. They mirror the existing pattern of `salaryRaw`/`salaryMin`/`salaryMax` but are sourced from analytics rather than employer disclosure. The next available migration number is 00031.

**Alternatives considered**:
- Key-value metadata table — over-engineered; these are fixed, well-defined fields
- JSONB blob — defeats the purpose of typed contracts and makes querying/filtering harder

---

## Decision 6: Dashboard Component Design

**Decision**: Render new fields as metadata chips/badges in the `JobMeta` area of the job detail header, plus a dedicated salary estimate tile in the tile grid.

**Components**:
1. **ExperienceBadge**: Chip showing experience level (e.g., "2+ years") in the `JobMeta` area. Only rendered when `experienceLevel` is non-null.
2. **EnglishLevelBadge**: Chip showing English level (e.g., "B1+") in the `JobMeta` area. Only rendered when `englishLevel` is non-null.
3. **SalaryEstimateCard**: Tile in the DashboardGrid showing estimated salary range with an "Estimated" label. Only rendered when salary estimate data exists and no employer salary was disclosed (or alongside if both exist).

**Rationale**: Follows existing dashboard patterns — `JobMeta` already displays company, location, remote, and salary as inline metadata with dot separators. Chips are already used for sourceKey and status. Tiles follow the `DashboardGrid` layout used by CompanyIntel, Contact, etc.

**Alternatives considered**:
- Full-width description blocks — too intrusive, experience and English level are scannable metadata
- Separate filter sidebar — out of scope for this feature, belongs in a future filtering feature
