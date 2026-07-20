# Quickstart: Validating the Company Intel Card

How to prove the feature works end-to-end once implemented. Implementation code belongs in the tasks plan, not here.

## Prerequisites

```bash
pnpm install
pnpm --filter @job-finder/shared build   # must precede other workspace packages
make up                                   # Postgres + Redis + Ollama via Docker Compose
make migrate                              # applies 00007_company_intel.sql
```

## Capture test fixtures

Unit tests parse saved HTML per source, so no network is needed in CI. Capture once per source (respecting 2s crawl-delay between requests to the same domain):

```bash
cd apps/api/internal/companyintel
mkdir -p testdata
UA='Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'

curl -s -H "User-Agent: $UA" 'https://www.crunchbase.com/organization/acme-corp' -o testdata/crunchbase_org.html
sleep 2
curl -s -H "User-Agent: $UA" 'https://layoffs.fyi/company/acme-corp' -o testdata/layoffs_fyi.html
sleep 2
curl -s -H "User-Agent: $UA" 'https://www.glassdoor.com/Reviews/acme-corp-Reviews-E12345.htm' -o testdata/glassdoor_reviews.html
sleep 2
curl -s -H "User-Agent: $UA" 'https://builtwith.com/acme-corp' -o testdata/builtwith_lookup.html
sleep 2
curl -s -H "User-Agent: $UA" 'https://acme-corp.com/about' -o testdata/company_about.html
```

Re-capture when selectors start failing; note the capture date in `research.md`.

## Level 1 — Unit tests (no network)

```bash
cd apps/api
go test ./internal/companyintel/ -v
```

Expected:

- Each scraper correctly parses its fixture HTML into a structured `SignalResult`.
- Crunchbase fixture yields a funding round with round name, amount, and date.
- layoffs.fyi fixture yields layoff count and date (or "no layoff data" for a clean company).
- Glassdoor fixture yields a rating (overall, recommend %, review count).
- Headcount scraper yields a number from "About" page text.
- BuiltWith fixture yields a non-empty list of technologies.
- Each scraper returns zero data + no error for an empty/garbage HTML string.
- All scrapers combined complete in <100ms (no network).

## Level 2 — Live smoke (network, opt-in)

Behind the existing `live` build tag, matching the convention from spec 001:

```bash
cd apps/api
go test -tags live ./internal/companyintel/ -run TestLive_Crunchbase -v
go test -tags live ./internal/companyintel/ -run TestLive_Glassdoor -v
```

Expected: each live probe returns >0 results for a known public company (e.g. "acme-corp" or "google"). This is the canary for markup drift.

## Level 3 — End-to-end through the running stack

```bash
make seed
make dev
```

Then in the dashboard (`http://localhost:5173`):

1. **Open any job detail page** whose company name resolves (e.g., a job at "Google").
2. **Company Intel Card** appears below the job description with rows for each available signal (SC-001).
3. Each signal row shows a label, a value, and a "fetched at" timestamp.
4. **Click Refresh** on the card. Loading spinner appears. Within ~15-60s, the card updates with fresh timestamps (SC-002).
5. **Open a second job** at the same company — signals are identical to the first job's (SC-003).
6. **Open a job with an unresolvable company** — the card is not rendered (FR-014).

## Level 4 — Failure-mode checks

| Scenario | How to force it | Expected |
|---|---|---|
| Crunchbase unreachable (FR-006) | Block `crunchbase.com` in `/etc/hosts`, refresh | Funding signal omitted; other signals still render. Per-source error indicator shown. |
| Glassdoor layout change (FR-011) | Point a unit test at HTML with no `.rating` elements | Zero data, no error, `slog.Warn` says Glassdoor markup may have changed |
| All sources down (FR-007) | Block all 5 source domains in `/etc/hosts`, refresh | Card keeps previous values; top-level error banner appears |
| No company name (FR-014) | View a job with an empty `company` field | Card not rendered at all |
| BuiltWith unreachable + no tech skills in posting (FR-015) | Block `builtwith.com` and set a job with no tech keywords | Tech stack signal omitted; other signals unaffected |
| layoffs.fyi page changes | Unit test with fixture missing `.layoff-item` elements | `hasLayoffs: false`, no error, logged warning |
| Company About page shows no employee count | Unit test with fixture lacking number text | Headcount signal omitted; `slog.Warn` says "could not determine headcount" |
| Concurrent refresh from two browser tabs | Open card in two tabs, click refresh on both | One succeeds, the other gets `409 Conflict`; no duplicate signal rows (UNIQUE constraint) |

## Regression gate

```bash
make test-lint    # Covers apps/api, apps/dashboard, and packages/shared
```

The change spans backend + frontend, so `make test-lint` is the binding gate. This also catches any sqlc/tygo regeneration issues.

Constitution Principle I check: grep the entire feature for any HTTP method other than `GET`. The scrapers and the refresh endpoint should only issue `GET` requests to external sites.
