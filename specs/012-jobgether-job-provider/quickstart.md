# Quickstart: Validating the Jobgether Job Source

Prerequisites: local stack running (`make up` or `make dev`), API reachable at its usual local
URL, dashboard Sources page reachable, Docker Compose Postgres/Redis up per the project's
standard dev setup. No credentials or env vars required — Jobgether is publicly accessible
(see [research.md#R1](./research.md#r1-access-model--public-vs-login-gated)).

## 1. Confirm Jobgether is registered

```bash
curl -s localhost:<api-port>/api/sources | jq '.[] | select(.key=="jobgether")'
```

Expected: an entry with `"key": "jobgether"`, `"kind": "scrape"`, `"enabled": true`,
`"healthy": true` (default state before any run) — see contracts/jobgether-adapter.md's REST
table and data-model.md's Job Source section.

## 2. Health check

```bash
curl -s -X POST localhost:<api-port>/api/sources/jobgether/test | jq
```

Expected: `{"healthy": true, "reason": "..."}` (or `false` + a human-readable reason if
Jobgether is currently blocking/unreachable) — validates US2 acceptance scenario 3 and FR-006.

## 3. Save a subscription (search-results URL)

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions \
  -H 'content-type: application/json' \
  -d '{"sourceKey":"jobgether","url":"https://jobgether.com/remote-jobs?q=golang","name":"Jobgether Golang"}' | jq
```

Expected: `200` with the created subscription. Validates US2 acceptance scenario 2 / FR-014.

Then confirm rejection of a non-Jobgether URL:

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions \
  -H 'content-type: application/json' \
  -d '{"sourceKey":"jobgether","url":"https://example.com/not-jobgether"}' | jq
```

Expected: non-`200` with an error message — validates FR-015 / SC-008.

## 4. Run the subscription and confirm listings land in the feed

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions/<subscription-id>/run | jq
```

Then:

```bash
curl -s 'localhost:<api-port>/api/jobs?source=jobgether' | jq '. | length'
```

Expected: count > 0 (or 0 with a successful run status if the search genuinely has no
matches — SC-002 / "zero results" edge case). Each returned job has non-empty `title`,
`company`, and an openable `url` — validates SC-004. At least 20 distinct listings are
expected for a typical subscription (SC-002).

## 5. Confirm dedupe on re-run

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions/<subscription-id>/run | jq '.new'
```

Expected: `0` new jobs on an immediate re-run against the same URL — validates US1 acceptance
scenario 2 / SC-003.

## 6. Confirm match-score metadata does not affect scoring

```bash
curl -s 'localhost:<api-port>/api/jobs?source=jobgether' | jq '.[0].raw'
```

Expected: `raw` may contain a `jobgetherMatchScore` field when Jobgether published one for that
listing, but the job's own `matchScore`/scoring fields (produced by this product's matching
pipeline) are computed independently — validates FR-012 and the "Jobgether's own AI
match-percentage score" edge case.

## 7. Confirm blocked/throttled handling is distinguishable

If Jobgether blocks or rate-limits the run (observed via logs or a deliberately-triggered
high-frequency test), the source run's failure reason should read as a blocked/rate-limited
condition, not "no matching listings" — validates SC-005 and the "source blocks or throttles
the request" edge case. Already-ingested listings from earlier in the run remain in the feed.

## 8. Confirm enrichment / unavailable-listing handling

Wait for the auto-enqueued enrich task (or trigger a backfill sweep):

```bash
curl -s -X POST localhost:<api-port>/api/sources/jobgether/enrich | jq
```

Then re-fetch one Jobgether job and confirm `description`/`postedAt` are populated — validates
US3 / SC-006 (at least 90% of enriched listings have a description longer than the ingestion
summary).

## 9. Dashboard check

Open the Sources page in the dashboard: Jobgether should appear in the source list (US2
scenario 1) and as a selectable option in the "New Subscription" form's source picker (the
`SUBSCRIPTION_SOURCES` list edit in `SourcesPage.tsx`).

## 10. Run the adapter's unit tests

```bash
cd apps/api && go test ./internal/jobsources/adapters/... -run Jobgether -v
```

Expected: all pass against the recorded `testdata/jobgether_*.html` fixtures, including a
blocked/interstitial-page fixture and an empty-results fixture — no live network calls in unit
tests (constitution IV: per-language test discipline).
