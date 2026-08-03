> **ARCHIVED — SHIPPED.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/job-sources.md`](../../domains/job-sources.md) — read that first.

---
# Feature Specification: Djinni Scraping Enhancement

**Feature Branch**: `022-djinni-scraping-enhance`

**Created**: 2026-07-28

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "need to improve djinni scrapping, here is current output http://localhost:5173/jobs/6c800802-9253-4cb8-a512-c778310c2ad2
vacancy: https://djinni.co/jobs/774850-full-stack-developer/, the company is Novacore but in our system is Unknown. also should be scrapped required number of years (Only from 2 years of experience), vacancy salary should also scrapped if you see something like (You expect a higher salary than the job is going to pay
The company decided not to disclose the salary range for the job
Your expectations: $4000 ). english level too. all that stuff should be visible on dashboard as cool components"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Accurate Company Name Extraction (Priority: P1)

As a job seeker browsing the dashboard, I want to see the correct company name for every Djinni listing so I can identify which company posted the vacancy and track opportunities by employer.

**Why this priority**: The company name is a fundamental piece of job information. Currently showing "Unknown" means users cannot identify the employer, making it impossible to filter or track by company. This is a regression — the scraper should already extract this but is failing.

**Independent Test**: Open any Djinni job in the dashboard (including https://djinni.co/jobs/774850-full-stack-developer/) and verify the company field shows "Novacore" instead of "Unknown". Can be tested by re-scraping the specific listing and checking the dashboard rendering.

**Acceptance Scenarios**:

1. **Given** a Djinni job listing with a company name on the page (e.g., "Novacore" in the header/title), **When** the scraper ingests the job, **Then** the `company` field is populated with the correct company name, not "Unknown".
2. **Given** a Djinni job listing where the company name is present in the page title/header but the list-page company selector returns empty, **When** the detail page is fetched, **Then** the company name is extracted from the detail page and persisted.
3. **Given** a Djinni listing where no company name can be found anywhere, **When** the scraper ingests the job, **Then** the `company` field falls back to "Unknown" and the raw HTML is preserved for debugging.

---

### User Story 2 - Required Experience Level Extraction (Priority: P2)

As a job seeker, I want to see the required years of experience for each vacancy (e.g., "2+ years") so I can quickly determine if I qualify without reading the full description.

**Why this priority**: Experience requirement is a key filter criterion for job seekers. Djinni already includes this information in the job description and in structured metadata on the page; extracting it saves users from scanning every description manually.

**Independent Test**: Open a Djinni job detail page containing "2+ years of commercial full-stack experience" in the requirements and verify the dashboard shows "2+ years" (or a parsed equivalent) in a prominent metadata section. Can be tested by re-ingesting a known listing and checking the job detail view.

**Acceptance Scenarios**:

1. **Given** a Djinni job description containing a pattern like "N+ years of experience" or "N years of ... experience", **When** the scraper processes the job, **Then** the experience requirement is extracted as structured data (e.g., minimum years value and raw text).
2. **Given** a Djinni job description that does NOT contain any experience requirement text, **When** the scraper processes the job, **Then** the experience field is left empty/null and the dashboard displays nothing for that section.
3. **Given** Djinni analytics metadata on the page (e.g., `?exp=2` in the salary analytics URL), **When** text-based extraction yields no result, **Then** the scraper falls back to extracting experience from structured page metadata.

---

### User Story 3 - Salary Information Extraction (Priority: P2)

As a job seeker, I want to see salary information (both directly disclosed and estimated ranges from Djinni's salary analytics) so I can evaluate compensation before spending time on a detailed review.

**Why this priority**: Compensation is often a top-3 filter criterion. Djinni surfaces salary data in two ways: (a) directly on listings that disclose it, and (b) via an analytics sidebar showing "average salary for similar positions." Both should be captured.

**Independent Test**: Open a Djinni job detail page with the salary analytics widget (e.g., "$1500-3000" for similar positions) and verify the dashboard displays both the direct salary (if present) and the analytics-based estimate. Can be tested against the example listing at https://djinni.co/jobs/774850-full-stack-developer/.

**Acceptance Scenarios**:

1. **Given** a Djinni job with a directly disclosed salary range in the listing, **When** the scraper processes the job, **Then** the salary is captured in `salaryRaw` and parsed into numeric min/max values (as the existing salary inference pipeline already does).
2. **Given** a Djinni job without a directly disclosed salary but with the salary analytics widget showing "average salary for similar positions," **When** the scraper processes the job, **Then** the analytics salary estimate is captured separately (not mixed with the direct salary field) and displayed as an estimated range with a clear "estimated" label.
3. **Given** a Djinni job with neither direct salary nor analytics data, **When** the scraper processes the job, **Then** both salary fields are empty and no salary UI is shown.

---

### User Story 4 - English Level Extraction (Priority: P3)

As a job seeker, I want to see the required English proficiency level (e.g., "B1+", "Upper-Intermediate") so I can filter roles by language requirements without reading full descriptions.

**Why this priority**: English level is often listed in Djinni requirements and is a key screening criterion for international teams. It is lower priority than company/experience/salary because it impacts fewer users directly, but adds significant filtering value.

**Independent Test**: Open a Djinni job detail page containing "English level — B1+" in the requirements and verify the dashboard shows the English level as a distinct metadata field. Can be tested by re-ingesting a known listing and checking the rendering.

**Acceptance Scenarios**:

1. **Given** a Djinni job description containing patterns like "English [level] — [B1/B2/C1/etc]" or "English: [level]", **When** the scraper processes the job, **Then** the English proficiency level is extracted and stored.
2. **Given** a Djinni job description in Ukrainian with "Англійська" patterns, **When** the scraper processes the job, **Then** the English level is likewise extracted from Ukrainian-language text.
3. **Given** a Djinni job description with no English level mentioned, **When** the scraper processes the job, **Then** the English level field remains empty/null.

---

### User Story 5 - Enhanced Job Detail Dashboard Components (Priority: P3)

As a job seeker viewing a job in the dashboard, I want the newly scraped fields (experience level, English level, salary analytics) displayed as clear, visually appealing components so I can absorb key information at a glance.

**Why this priority**: Data extraction without good presentation has limited user value. The dashboard already has a tile-based layout for metadata; new fields should follow the same visual language. This is same-tier as P3 because it's presentational — the data can be scraped and stored without it, but the user-facing value depends on it.

**Independent Test**: Open any Djinni job in the dashboard and verify that the job detail page renders experience level, English level, and salary information (when available) as distinct, well-styled components within the existing tile grid layout.

**Acceptance Scenarios**:

1. **Given** a job has an extracted experience level, **When** the dashboard renders it, **Then** the experience level is shown as a metadata chip/badge or tile in the job detail header area.
2. **Given** a job has an extracted English level, **When** the dashboard renders it, **Then** the English level is shown as a metadata chip/badge or tile in the job detail header area.
3. **Given** a job has a salary analytics estimate (but no direct salary), **When** the dashboard renders it, **Then** the estimated salary is shown in a salary tile/component clearly labeled as "Estimated" to distinguish from employer-disclosed salary.
4. **Given** a job has none of the enhanced fields, **When** the dashboard renders it, **Then** the UI gracefully omits those components without broken layout or empty placeholders.

---

### Edge Cases

- **Company name in different elements**: Djinni sometimes puts the company name in the page `<title>`, sometimes in a `<a>` tag, sometimes in a sidebar widget. What if the company appears in multiple places with different text? The scraper should prefer the most authoritative source (detail page header over list page link text).
- **Experience level in Ukrainian vs English**: Some Djinni listings are written in Ukrainian (e.g., "2+ роки досвіду"). Patterns must cover both languages.
- **Ambiguous experience text**: Phrases like "experience with React" should NOT be parsed as a years-of-experience requirement. Only patterns like "N+ years", "N years of ... experience", "досвід від N років" should match.
- **Multiple salary sources**: A listing may have both a disclosed salary and an analytics estimate. Both should be preserved — displayed salary takes priority, with the estimate shown alongside or as a fallback.
- **Scraping failure**: If Djinni changes their HTML structure, the extraction should fail gracefully — log the raw HTML for the affected fields, return partial results rather than erroring the entire job ingestion.
- **Salary analytics widget not present**: Not all Djinni pages include the salary analytics sidebar. The scraper must handle its absence without errors.
- **English level alongside other languages**: If a listing mentions both English and other language requirements (e.g., German B2), the scraper should extract all language requirements, not just English.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST extract and persist the company name from Djinni job detail pages, with the detail-page value taking precedence over the list-page value.
- **FR-002**: System MUST extract the required years of experience from Djinni job descriptions using regex patterns covering both English ("N+ years", "N years of...experience") and Ukrainian ("N+ років", "досвід від N років") formulations.
- **FR-003**: System MUST extract the English proficiency level from Djinni job descriptions using patterns covering both English ("English [level] — B1/B2/C1/etc.", "English: Advanced") and Ukrainian ("Англійська — B1", "Рівень англійської: Intermediate") formulations.
- **FR-004**: System MUST extract the salary analytics estimate (average salary for similar positions) from Djinni job detail pages when the salary analytics widget is present.
- **FR-005**: System MUST store experience level and English level as new fields on the Job entity, accessible through the API and dashboard.
- **FR-006**: System MUST store the salary analytics estimate separately from the employer-disclosed salary, with a flag distinguishing the source (disclosed vs. analytics-estimated).
- **FR-007**: The job detail dashboard MUST render experience level, English level, and salary information as visually distinct components within the existing tile-based layout when data is available.
- **FR-008**: The job detail dashboard MUST gracefully handle the absence of any enhanced field by omitting the corresponding component without broken layout.
- **FR-009**: Scraping extraction failures for individual fields MUST NOT cause the entire job ingestion to fail; partial results with missing fields are acceptable.

### Key Entities

- **Job**: Gains new optional fields: `experienceLevel` (minimum years as integer with raw text), `englishLevel` (string, e.g., "B1+", "Upper-Intermediate"), `salaryEstimateMin`/`salaryEstimateMax` (optional integers, for analytics-derived estimates), `salaryEstimateRaw` (optional string, raw analytics salary text), `salaryIsEstimated` (boolean, true when salary comes from analytics rather than employer disclosure).
- **NormalizedJob** (source adapter contract): Gains optional fields for experience level, English level, and salary estimate to be passed from the Djinni adapter through the ingestion pipeline.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Company name is correctly populated for at least 95% of Djinni listings that display a company name on the detail page, up from the current state where some listings show "Unknown."
- **SC-002**: Experience level is extracted and displayed for at least 80% of Djinni listings that mention experience requirements in the description.
- **SC-003**: English level is extracted and displayed for at least 80% of Djinni listings that mention English proficiency in the description.
- **SC-004**: Salary analytics estimates are captured for Djinni listings that include the salary analytics widget.
- **SC-005**: The dashboard loads the job detail page in under 2 seconds even with the new components rendered.
- **SC-006**: Users can scan key job metadata (company, experience, English, salary) in under 5 seconds from the job detail page without reading the full description.

## Assumptions

- Djinni's HTML structure is assumed to be server-rendered and consistent across similar job pages; the selectors/regex patterns target the current structure observed as of July 2026.
- The existing salary inference pipeline (spec 006) continues to operate on employer-disclosed salary data; the analytics estimate is additive, not a replacement.
- Dashboard rendering follows the existing tile-based card layout pattern already used on the job detail page.
- Job descriptions on Djinni are primarily in English or Ukrainian; patterns cover the most common formulations in both languages.
- The salary analytics widget on Djinni is rendered as visible text (e.g., "$1500-3000"), not behind JavaScript interactivity.
