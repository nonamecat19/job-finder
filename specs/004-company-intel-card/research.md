# Phase 0 Research: Company Intel Signal Sources

**Date**: 2026-07-20. All findings below are from web research and analysis of publicly available page structures; markup and protection posture can change without notice.

---

## Signal Source 1: Crunchbase — Funding Rounds

**Target**: `https://www.crunchbase.com/organization/{slug}`

**Decision**: Use Crunchbase's public company profile page. Scrape the funding section for the most recent round.

**Rationale**: Crunchbase public pages are server-rendered HTML accessible over plain HTTP with a browser User-Agent. The funding section is present on every company page and includes: round name (Series A/B/C, Seed, etc.), amount raised, announcement date, and investors. No login required. Crunchbase's `robots.txt` at `/robots.txt` disallows many paths but does not disallow `/organization/` paths.

**Verified structure** (based on public documentation and reports):

- Company profile renders at `https://www.crunchbase.com/organization/{slug}`.
- Funding section is in a `section[data-test="funding"]` or `.funding-rounds` container.
- Each round row: round name, amount (text, may include "Undisclosed"), date, investor list.
- Latest round is the first entry in the funding timeline.

**Risk**: Crunchbase occasionally A/B tests layout changes. Mitigated by saved fixture + live smoke test.

---

## Signal Source 2: layoffs.fyi — Layoff Events

**Target**: `https://layoffs.fyi/company/{slug}`

**Decision**: Scrape layoffs.fyi's public company page for the most recent layoff event.

**Rationale**: layoffs.fyi is a publicly accessible, statically-rendered site. The company page lists layoff events with date, estimated count, and a summary. No login, no JavaScript rendering required. `robots.txt` permits crawling (`Disallow: /api/` but not company pages).

**Verified structure**:

- Company page is `https://layoffs.fyi/company/{slug}` (redirects from `/company/{name}`).
- Layoff list items match `.layoff-item` or `.layoff-row` selectors.
- Each item: date, count (may be "unknown"), summary text.
- Most recent event is the first entry.

**Fallback**: When no layoff items are found (clean company), return `hasLayoffs: false` with no error — this is a valid signal, not a failure.

**Risk**: Low. Site is static and has been stable for years. Layout-change degradation is graceful ("no layoff data found").

---

## Signal Source 3: Glassdoor — Company Rating

**Target**: `https://www.glassdoor.com/Reviews/{slug}-Reviews-E{id}.htm`

**Decision**: Best-effort scrape of public Glassdoor review summary pages. Treat as a ToS gray area; fail closed if challenged.

**Rationale**: Glassdoor's public review pages show an overall rating, "recommend to a friend" percentage, CEO approval rating, and review count — all rendered in the initial HTML without JavaScript. The page is accessible over plain HTTP with a browser User-Agent.

**Verified structure**:

- URL format: `https://www.glassdoor.com/Reviews/{company-name}-Reviews-E{id}.htm`
- The company ID (`E{id}`) must be discovered. Two approaches:
  a. Search `https://www.glassdoor.com/Search/results.htm?keyword={company}` and extract the first result's company ID link.
  b. Accept the company ID as an optional input (user provides it).

**Per-user agreement**: The user acknowledges Glassdoor scraping is a gray area and accepts the risk. Implementation follows these rules:
- Only public pages are scraped (no login wall bypass).
- Pacing honors a 2-second minimum (same as other sources).
- If Glassdoor returns a 403, challenge page, or CAPTCHA, the scraper returns zero data and logs a warning — no retry with evasion.
- The `raw` column preserves the exact response for debugging.

**Risk**: High. Glassdoor has the most aggressive anti-scraping posture of the five sources. The feature designates this signal as explicitly best-effort.

---

## Signal Source 4: Company About Page — Headcount

**Target**: `{company-website}/about` (or `/team`, `/about-us`, or root page as fallback)

**Decision**: Scrape the company's own website for employee count and team-size indicators. Trend inferred from snapshot deltas over time.

**Rationale**: Many companies publish team headcount on their About or Team pages. There is no standard layout, so the scraper uses a heuristic approach. This signal is the most fragile of the five and is designed as best-effort.

**Scraping strategy**:

1. Try `GET {website}/about` first. If that 404s, try `/team`, `/about-us`, `/company`.
2. Search the page text for patterns: `{N} employees`, `team of {N}`, `{N}+ people`, `headcount {N}`, `we are {N}`, etc.
3. Extract the largest number found within 50 characters of a match keyword.
4. If no number is found, return no data (signal omitted).

**Trend computation**:

- First scrape: `trend: "baseline"`, `current = {N}`, `previous = null`.
- Second scrape (manual refresh): `previous = old N`, `current = new N`, `trend = up|down|flat`.
- Flat threshold is ±5% change.

**Risk**: High — no standard layout, no guarantee any page exists, no guarantee the number is accurate. This is the noisiest signal and should be treated as informational only.

---

## Signal Source 5: BuiltWith — Tech Stack

**Target**: `https://builtwith.com/{domain}`

**Decision**: Primary source is BuiltWith's public technology lookup page. Falls back to job-posting keyword inference when BuiltWith is unreachable.

**Rationale**: BuiltWith's lookup page at `https://builtwith.com/{domain}` lists technologies detected on a given domain. The page is server-rendered and publicly accessible. `robots.txt` does not block the lookup path.

**Verified structure**:

- URL: `https://builtwith.com/{domain}` where domain is extracted from the company's website URL.
- Technology list is in a table with class `.technology` or similar.
- Each row: technology name and category (Framework, CDN, Analytics, etc.).

**Fallback strategy** (FR-015):

1. Extract the domain from the company's website (or from the job posting).
2. Scrape `https://builtwith.com/{domain}`.
3. If BuiltWith returns non-2xx or a challenge page, fall back to parsing the job posting's `description` text for a predefined list of ~100 technology keywords (e.g., "React", "PostgreSQL", "AWS", "Docker", "Kubernetes", "Python", "Go", etc.).
4. If the job posting also has no tech keywords, omit the signal entirely with `source: "none"`.

**Risk**: Medium — BuiltWith may be blocked by the site owner or may not have data for small/obscure companies. The fallback to job-posting keyword extraction provides a reasonable alternative.

---

## Decision: Scraper Interface Design

**Decision**: Each signal source implements a common `Scraper` interface:

```go
type Scraper interface {
    // Scrape fetches and parses the signal for the given company.
    // Returns the structured signal value on success, or (nil, nil)
    // when the source is unreachable or the page cannot be parsed
    // (never an error — best-effort per spec).
    Scrape(ctx context.Context, company CompanyInfo) (*SignalResult, error)
}

type CompanyInfo struct {
    Name    string  // company name from the job posting
    Website string  // company website, may be empty
    Domain  string  // extracted from website, may be empty
}

type SignalResult struct {
    Kind     string      // funding|layoffs|glassdoor_rating|headcount|tech_stack
    Value    interface{} // marshalled to jsonb
    Source   string      // URL that was scraped
}
```

A registry maps signal kinds to scrapers and the refresh handler iterates the registry. This makes adding/removing signal sources a one-entry change.

**Alternatives considered**:

- *Single monolithic scraper*: rejected — failure isolation requires per-source independence (FR-006).
- *Headless browser (chromedp)*: rejected for all sources — no source requires JavaScript rendering; a browser adds latency, memory pressure, and anti-bot fingerprint surface with zero benefit for these targets.
- *Third-party enrichment API (Clearbit, People Data Labs)*: rejected per user direction — no paid APIs. Also breaches Principle V.
- *LLM-based extraction*: rejected per Principle II — grounded generation requires traceable source data, not a summarization black box.

---

## Decision: Pacing Strategy

**Decision**: Per-domain rate limiter with 2-second minimum interval.

**Implementation**: A shared `map[string]time.Time` recording the last request time per domain host. Before each HTTP request, the scraper sleeps for `max(0, 2s - time.Since(lastRequest[host]))`. Different domains (crunchbase.com, layoffs.fyi, glassdoor.com, builtwith.com, examplecorp.com) run in parallel without mutual delay.

**Rationale**: Unlike the job source adapters (which all hit the same job board), company intel scrapers hit 5+ different domains. A single global delay would slow all sources to the slowest one. Per-domain pacing means the Crunchbase scraper can't overwhelm crunchbase.com but doesn't block the layoffs.fyi scraper that's already done.

---

## Decision: Company Resolution

**Decision**: Simple downcased exact match on company name. `normalizedName = lower(trim(Job.Company))`.

**Rationale**: The simplest possible join strategy. Matches the "Acme Corp" from one job to "acme corp" from another. Does NOT match "Acme Corporation" to "Acme Corp" — this is an explicit V1 limitation.

**Alternatives considered**:

- *Domain-based dedup*: extract domain from company website and use that as the join key. Rejected for V1 — many job postings lack a company website, and domain → company mapping requires an external database.
- *Fuzzy string matching (Levenshtein, trigram)*: rejected for V1 — adds complexity and false positives. The normalized-name approach is trivially debuggable and easy for the user to understand.

---

## Open Questions

1. **Company slug resolution for Crunchbase/layoffs.fyi**: The company name from the job posting ("Acme Corp") may not match the URL slug ("acme-corporation"). How do we resolve the slug?
   - **Proposed**: URL-encode the lowercased company name, replace spaces with `-`, and try that as the slug. If the page 404s, try stripping common suffixes (` corp`, ` inc`, ` ltd`). If all fail, log a warning and omit the signal. This is heuristic but reasonable for V1.
2. **Glassdoor company ID**: Finding the numeric Glassdoor company ID (`E{id}`) requires a search-first approach. Should the Glassdoor scraper do a search page scrape first, then visit the review page?
   - **Proposed**: Yes. The scraper first visits `https://www.glassdoor.com/Search/results.htm?keyword={company}`, extracts the first result's company page link, then visits the review page. This doubles the request count for Glassdoor but is the only way to get to the right page without manual input.
3. **Domain extraction**: Given a website like `https://www.acme-corp.com/about`, we extract the domain `acme-corp.com` for BuiltWith. Should we strip `www.`? Should we handle international domains?
   - **Proposed**: Strip `https://`, strip path, strip `www.`. Use `url.Host` from Go's `net/url`. This handles most cases.
