# Quickstart: Validate the Glassdoor Job Source

## Prerequisites

- Local stack running: `make up` (Postgres, Redis, `apps/api`, `apps/dashboard`)
- Implementation complete: `GlassdoorAdapter` registered in `cmd/server/compose.go`,
  Glassdoor added to `SourcesPage.tsx`'s subscription-form dropdown

## 1. Confirm the source is registered

```sh
curl -s localhost:8080/api/sources | jq '.[] | select(.key == "glassdoor")'
```

Expected: one object with `"key": "glassdoor"`, `"kind": "scrape"`.

## 2. Save a subscription

Via the dashboard: Sources screen → select **Glassdoor** → paste a Glassdoor search-results
URL (e.g. `https://www.glassdoor.com/Job/remote-golang-jobs-SRCH_...htm`) → Save.

Or via API:

```sh
curl -s -X POST localhost:8080/api/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{"sourceKey":"glassdoor","name":"Glassdoor Go remote","url":"https://www.glassdoor.com/Job/remote-golang-jobs-SRCH_..."}'
```

Expected: `201` with the created subscription. Re-run with a non-Glassdoor URL (e.g. an
Indeed URL) and confirm a `4xx` with a human-readable rejection reason (FR-015).

## 3. Enable the source and run it on demand

Dashboard: toggle Glassdoor enabled, click "Run now". Or:

```sh
curl -s -X POST localhost:8080/api/sources/glassdoor/enable
curl -s -X POST localhost:8080/api/sources/glassdoor/run
```

Expected: run completes with an outcome of `succeeded` or `partial`, a non-negative
`listingsFound` count, and — if Glassdoor blocked the request — a `failed` outcome carrying
a human-readable reason distinguishable from "zero results" (FR-011, spec SC-005). This is
the step most likely to surface Glassdoor's anti-bot posture (research.md R3); a blocked
outcome here is an expected possible result, not a bug, but confirm the reason string
clearly says "blocked" rather than misreporting as "no listings found".

## 4. Confirm listings reached the feed

```sh
curl -s 'localhost:8080/api/jobs?source=glassdoor' | jq '.items | length, .items[0]'
```

Expected: at least one job with non-empty `title`, `company`, and an openable `url`
(SC-004), attributed `sourceKey: "glassdoor"`.

## 5. Re-run and confirm deduplication

Run step 3 again immediately. Expected: `listingsFound` similar to before, but
`listingsAdded` (new-to-feed count) is `0` — no duplicate feed entries (SC-003).

## 6. Confirm enrichment

Open one ingested Glassdoor job in the dashboard job detail view (or
`GET /api/jobs/{id}`). Expected: after enrichment runs, `description` is the full posting
text (longer than the ingestion-time summary), and `postedAt` is resolved (SC-006).

## 7. Confirm health check reporting

```sh
curl -s -X POST localhost:8080/api/sources/glassdoor/test
```

Expected: `{"healthy": true|false, "reason": "..."}` — `reason` is human-readable and, on
failure, distinguishes reachability/parsing/blocking causes (FR-006).

## 8. Confirm disabling stops requests

Disable Glassdoor, trigger a scheduled ingestion cycle (or wait for the next one), and
confirm via logs/run history that no Glassdoor run was attempted (FR-005).
