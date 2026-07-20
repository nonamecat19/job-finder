# Feature Specification: Company Intel Card

**Feature Branch**: `004-company-intel-card`

**Created**: 2026-07-20

**Status**: Draft

**Input**: User description: "surface company-level intelligence on the job detail page so I can kill bad-fit companies before writing tailored documents."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See company intel on the job detail page (Priority: P1)

A user opens a job detail page and sees a company intel card showing all available signals — funding round, layoff news, Glassdoor rating, headcount trend, and tech stack — or a clear 'no data yet' state if the company has never been probed. The user can immediately identify red flags (e.g., recent layoffs, low Glassdoor score, dying headcount trend) before deciding whether to write a tailored resume or cover letter.

**Why this priority**: This is the feature's sole modal value — it surfaces the information users need to make the kill/don't-kill decision in the same screen as the job itself, without tabbing out to Crunchbase, Glassdoor, or layoffs.fyi. On its own it is a complete, shippable increment.

**Independent Test**: Open any job detail page whose company has been probed; confirm the card renders with at least one non-empty signal. Then open a job whose company has never been seen; confirm the card shows a clean "no data yet" state.

**Acceptance Scenarios**:

1. **Given** a job whose company has been probed for at least one signal, **When** the user opens the job detail page, **Then** the company intel card displays each available signal with its value, label, and a human-readable timestamp of when it was fetched.
2. **Given** a job whose company has never been probed, **When** the user opens the job detail page, **Then** the card renders with a "no data yet" message and a refresh button.
3. **Given** multiple signals for a company, **When** the card renders, **Then** each signal occupies a distinct, labeled row within the card.
4. **Given** a company signal older than 30 days, **When** the card renders, **Then** the signal MAY display a subtle "possibly stale" indicator next to its value. This is optional; the worker may omit it.

---

### User Story 2 - Manually refresh company intel from the card (Priority: P2)

While viewing the job detail page, the user clicks a refresh button on the company intel card. The card shows a loading state while signals are re-fetched, then updates in-place with fresh values. All signals are re-scraped from their sources and the card reflects the latest data without the user navigating away.

**Why this priority**: Without a manual trigger, the card would only ever show whatever data was captured at first probe. Since there is no scheduled refresh (per persistence decision), the refresh button is the only way to update stale signals.

**Independent Test**: Open a job detail page whose card has data, click the refresh button, confirm the loading state appears, and confirm the card updates with new timestamps and (if any source changed) new values.

**Acceptance Scenarios**:

1. **Given** a rendered company intel card with existing signals, **When** the user clicks the refresh button, **Then** the card enters a loading state, calls `POST /jobs/{id}/company-intel/refresh`, and re-renders with fresh data on success.
2. **Given** a refresh in progress, **When** the user clicks refresh again, **Then** the second click is ignored (debounced).
3. **Given** a refresh that fails for one or more signal sources, **When** the response returns, **Then** the card shows previously cached values for failed sources and fresh values for successful ones, with a per-source error indicator.
4. **Given** a refresh that fails entirely (all sources unreachable), **When** the response returns, **Then** the card keeps its previous values and displays a top-level error banner.

---

### User Story 3 - Normalized company signals across multiple jobs (Priority: P3)

The same company appears in two different job postings. The user opens the first job's detail page and then the second. Both pages show the same company signals — identical values, identical timestamps. Signals were fetched once and shared, not duplicated per job.

**Why this priority**: Without normalization, the same Crunchbase page would be scraped N times for N jobs at the same company, wasting fetch budget and making "possibly stale" thresholds inconsistent across jobs.

**Independent Test**: Open Job A at Company X and confirm its signals. Open Job B (different role, same company) at Company X and confirm the signals are identical to Job A's — down to the `fetchedAt` timestamp.

**Acceptance Scenarios**:

1. **Given** a company with signals already stored from a previous job, **When** a new job at the same company is viewed, **Then** the card renders the same signal values and timestamps without any additional fetch.
2. **Given** a refresh triggered from one job's card, **When** another job at the same company is viewed immediately afterward, **Then** that job's card reflects the refreshed data.
3. **Given** a company name that appears in two different casings (e.g., "Acme Corp" and "acme corp"), **When** both jobs are viewed, **Then** they resolve to the same normalized Company row and share signals.

---

### Edge Cases

- Company name cannot be parsed from the job posting → the card is hidden entirely (no orphan data).
- Company website returns a non-2xx or times out → the card shows no data and logs the failure, with a "could not reach source" message per signal.
- Crunchbase blocks the request → funding signal is omitted from the card; other signals unaffected.
- Glassdoor blocks the request → Glassdoor rating signal is omitted; other signals unaffected.
- BuiltWith blocks the request → tech stack falls back to the job posting's required-skills section; if that is also empty, the tech stack signal is omitted.
- layoffs.fyi page layout changes → layoff signal scrapes zero results; card shows "no layoff data found" rather than failing.
- Conflicting funding data between rounds → the scraper captures the most recent funding round from the Crunchbase page (the page shows all rounds; we display the latest). The `raw` column preserves the full scrape for later debugging.
- The same company name in different jobs actually refers to different legal entities (e.g., "Apple" as Apple Inc vs. Apple Freight) → not addressed in V1. Downcased name matching is intentionally simple and may conflate entities. A future disambiguation mechanism can be added.
- A company has no website in the job posting → the scraper still probes Crunchbase/layoffs.fyi by company name; headcount and BuiltWith probes are skipped.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST render a company intel card on the job detail page when the job has a parseable company name.
- **FR-002**: System MUST display each available signal (funding, layoffs, glassdoor_rating, headcount, tech_stack) as a labeled row with its value and fetch timestamp.
- **FR-003**: System MUST display a "no data yet" state when the company has never been probed, with a refresh button.
- **FR-004**: System MUST provide a `POST /jobs/{id}/company-intel/refresh` endpoint that re-scrapes all signal sources for the job's company and upserts results.
- **FR-005**: System MUST show a loading state on the card during refresh and ignore duplicate clicks (debounce).
- **FR-006**: System MUST preserve previously cached signal values when a signal source is unreachable during refresh, and display a per-source error indicator.
- **FR-007**: System MUST treat an entirely failed refresh (all sources down) as a non-error for the user: previous values remain, a top-level error banner appears.
- **FR-008**: System MUST normalize company signals by company identity (lowercased name), so the same company across multiple jobs shares a single set of signal rows.
- **FR-009**: System MUST upsert CompanySignal rows on refresh using `UNIQUE(companyId, kind)`, never duplicating or leaving orphan rows.
- **FR-010**: System MUST store the raw scraped response body in the `raw` jsonb column for each signal, enabling post-hoc debugging of scrape failures.
- **FR-011**: System MUST scrape all external sources (Crunchbase, layoffs.fyi, Glassdoor, BuiltWith, company About page) via public page scraping only — no paid API keys, no authentication bypass, no challenge-solving.
- **FR-012**: System MUST pace repeated requests to each external source at a configurable interval, defaulting to no faster than one request per 2 seconds per source domain.
- **FR-013**: System MUST NOT submit forms, post comments, or take any action on any external site — the feature is strictly read-only intelligence gathering.
- **FR-014**: System MUST hide the company intel card when the job has no parseable company name (empty or whitespace-only after trimming).
- **FR-015**: System MUST fall back to the job posting's required-skills section for tech stack signals when BuiltWith is unreachable, and omit the signal when both sources fail.
- **FR-016**: System MUST log per-source scrape failures at `slog.Warn` with the source domain and HTTP status, enabling diagnosis without requiring reproduction by hand.

### Key Entities

- **Company**: A normalized entity keyed by lowercased company name. Rows: `id`, `name` (original casing), `normalizedName` (lowercased), `website`, `firstSeenAt`, `lastRefreshedAt`.
- **CompanySignal**: A per-company, per-kind signal row. Rows: `id`, `companyId` (FK cascade), `kind` (one of `funding|layoffs|glassdoor_rating|headcount|tech_stack`), `value` (structured JSON), `source` (URL scraped), `fetchedAt`, `raw` (full scraped response body JSON). Unique constraint on `(companyId, kind)`.
- **Job Company Field**: The existing `Job.company` text field is the join key used to look up or create the normalized Company row. No new column on the Job table.
- **Company Intel Card**: A dashboard component rendering the signal rows. Lives on the job detail page.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can open any job detail page for a probed company and see all 5 signal types (or a graceful subset) within the card within 2 seconds of page load. No additional click or navigation required.
- **SC-002**: Clicking refresh on a card with previous data returns updated values (or cached values for failed sources) and a new `fetchedAt` timestamp within 60 seconds for typical cases (all sources reachable, each paced at 2s = ~10s for 5 sources).
- **SC-003**: Opening a second job for the same company shows identical signal values and timestamps as the first — no second scrape occurs.
- **SC-004**: The card gracefully handles all listed edge cases without erroring the page: missing company name hides the card; blocked sources show partial data; layout changes produce a "no data" state rather than a crash.
- **SC-005**: External source scrape failures are diagnosable from logged warnings alone — the log line contains the domain, HTTP status or error, and company name — without reproducing the failure by hand.
- **SC-006**: A refresh retry (clicking refresh again after a partial failure) successfully re-scrapes only the previously-failed sources, leaving already-successful signals in place, within the same 60s budget.
- **SC-007**: Adding the company intel card to the job detail page requires zero changes to the existing job model, job list, search pipeline, or scoring engine — the card is a read-only overlay.
- **SC-008**: A Crunchbase, Glassdoor, or BuiltWith layout change degrades only that signal (returns "no data" for that source), does not block other signals, and does not crash the card.

## Assumptions

- **Public pages stay readable**: Crunchbase company pages, layoffs.fyi pages, Glassdoor company review pages, and BuiltWith lookup pages remain publicly reachable over HTTP without authentication. If any of these sites gates content behind a login wall or bot challenge, that signal degrades to "no data" without crashing the card (see FR-011 — no paid APIs, no challenge-solving).
- **Glassdoor is best-effort**: The user acknowledges Glassdoor scraping is a ToS gray area. The scraper targets public review summary pages; it does not bypass a login wall. If Glassdoor blocks or challenges the scraper, the signal fails closed gracefully (FR-006). This is acceptable to the user and the feature explicitly treats Glassdoor as best-effort.
- **Company name is the join key**: `Job.company` is the sole bridge between a job and its Company row. No external company database, no domain matching, no fuzzy dedup. Downcased exact match means "Acme Corp" and "acme corp" match but "Acme Corporation" and "Acme Corp" do not. This is intentionally simple for V1.
- **No scheduled refresh**: Per user decision, the only refresh path is manual via the button on the card. No background cron, no automatic re-scrape on page visit. Staleness is the user's responsibility; the optional "possibly stale" hint at 30 days is a UI affordance, not a system guarantee.
- **Scraping is best-effort, not guaranteed**: External sites may change layout, block requests, or go down at any time. All scrapers follow the "zero results + warning, never an error" pattern established by spec 001 (FR-007). A scrape producing no parseable data is normal, not a defect.
- **Headcount trend requires patience**: Headcount is inferred from delta between snapshots of the company's own "About" or "Team" page. The first scrape records a baseline; a trend arrow requires at least two scrapes separated in time. Freshly-probed companies show headcount as "baseline captured" rather than a trend.
- **Refresh is all-or-nothing per signal**: Each signal source is scraped independently during a refresh. A failed source does not block others (FR-006). However, within a single source (e.g., Crunchbase), a failure means that signal is not updated — no partial "we got the name but not the funding amount" merging.

## Dependencies

- The existing job detail page (`apps/dashboard/src/features/job-detail/JobDetailPage.tsx`) — the card renders inside it.
- The existing API server (`apps/api`) — the refresh endpoint lives here.
- The existing HTTP scraping service (shared `internal/scraping.Service` used by the job source adapters) — reused for company signal probes.
- PostgreSQL for the new `Company` and `CompanySignal` tables — requires migration `00007_company_intel.sql`.
- External sites: Crunchbase, layoffs.fyi, Glassdoor, BuiltWith, and the target company's own website — all outside the project's control, all best-effort per the assumptions above.
- The `goquery` HTML parsing library (already vendored for job source adapters) — reused for company-page scraping.
