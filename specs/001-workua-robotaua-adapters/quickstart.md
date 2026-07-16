# Quickstart: Validating the work.ua Adapter

How to prove the feature works end-to-end once implemented. Implementation code belongs in `tasks.md`, not here.

## Prerequisites

```bash
pnpm install
pnpm --filter @job-finder/shared build   # must precede other workspace packages
make up                                   # Postgres + Redis + Ollama via Docker Compose
```

## Capture test fixtures

Unit tests parse saved HTML, so no network is needed in CI. Capture once (respecting the 2s crawl-delay between requests):

```bash
cd apps/api/internal/jobsources/adapters
mkdir -p testdata
UA='Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'
curl -s -H "User-Agent: $UA" 'https://www.work.ua/jobs-php/' -o testdata/workua_list.html
sleep 2
curl -s -H "User-Agent: $UA" 'https://www.work.ua/jobs/8047944/' -o testdata/workua_detail.html
```

Re-capture when selectors start failing; note the capture date in `research.md`. If the detail URL 404s, any live `/jobs/{id}/` from the list page works.

## Level 1 — Unit tests (no network)

```bash
cd apps/api
go test ./internal/jobsources/adapters/ -run WorkUa -v
```

Expected:

- `Key()` returns `"workua"`, `Kind()` returns `dto.SourceKindScrape`.
- Parsing `workua_list.html` yields ≥10 jobs, each with non-empty `Title`, non-empty `Company`, absolute `URL`, and a non-nil `ExternalID`.
- Parsing `workua_detail.html` yields a `Description` substantially longer than a card teaser.
- Parsing an empty/garbage HTML string yields zero jobs **and no error** (FR-007).
- Cyrillic titles survive round-trip byte-identical (FR-005).

## Level 2 — Live smoke (network, opt-in)

Behind the existing `live` build tag, matching `live_smoke_test.go` convention:

```bash
cd apps/api
go test -tags live ./internal/jobsources/adapters/ -run TestLive_WorkUa -v
```

Expected: a real search returns >0 jobs and logs the count. This is the canary for markup drift — it is the only test that catches work.ua changing its HTML.

## Level 3 — End-to-end through the running stack

```bash
make seed    # materializes the workua job_source row
make dev
```

Then in the dashboard (`http://localhost:5173`):

1. **Sources page** → work.ua is listed. Health dot is green (FR-009). No config inputs render — work.ua needs no credentials.
2. **Enable** work.ua; create/run a saved search with keywords (e.g. `php`).
3. **Jobs list** → work.ua jobs appear with title, company, and a link that opens the real posting (SC-003).
4. **Re-run the same search** → zero duplicates added (SC-004, FR-004).
5. **Open a work.ua job** → within ~10 min, description has grown from teaser to full text (SC-005, FR-010).
6. **Remote filter** → with remote-only on, results come from `/jobs-remote/` (FR-003).

## Level 4 — Failure-mode checks

| Scenario | How to force it | Expected |
|---|---|---|
| Markup change (FR-007) | Point a unit test at HTML with no `.job-link` cards | Zero jobs, no error, `slog.Warn` says markup may have changed |
| Board unreachable (FR-008) | Block `work.ua` in `/etc/hosts`, run a multi-source search | work.ua run marked failed; other sources' results still ingested |
| Dead posting (Story 3, scenario 2) | Call `FetchDetail` on a 404 job URL | Warning logged, teaser description preserved, other enrichments unaffected |
| Malformed subscription URL (FR-013) | Subscription with `://not a url` | Error names the offending URL |

## Regression gate

```bash
cd apps/api && go test ./...
```

The change is confined to `apps/api`, so per Principle IV `go test` is the binding gate; `make test-lint` is optional here since no cross-app boundary is touched.

Constitution Principle I check: grep the adapter for any non-GET request. There should be none — work.ua is read-only discovery.
