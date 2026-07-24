# Quickstart: Validating the Indeed Job Source

Prerequisites: local stack running (`make up` or `make dev`), API reachable at its usual
local URL, dashboard Sources page reachable, Docker Compose Postgres/Redis up per the
project's standard dev setup.

## 1. Confirm Indeed is registered

```bash
curl -s localhost:<api-port>/api/sources | jq '.[] | select(.key=="indeed")'
```

Expected: an entry with `"key": "indeed"`, `"kind": "scrape"`, `"enabled": true`,
`"healthy": true` (default state before any run) — see contracts/indeed-adapter.md's REST
table and data-model.md's JobSource section.

## 2. Health check

```bash
curl -s -X POST localhost:<api-port>/api/sources/indeed/test | jq
```

Expected: `{"healthy": true, "reason": "..."}` (or `false` + a human-readable reason if
Indeed is currently blocking/unreachable) — validates US2 acceptance scenario 3 and FR-006.

## 3. Save a subscription (pasted search URL)

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions \
  -H 'content-type: application/json' \
  -d '{"sourceKey":"indeed","url":"https://www.indeed.com/jobs?q=golang&l=remote","name":"Indeed Go Remote"}' | jq
```

Expected: `200` with the created subscription. Validates US2 acceptance scenario 2 / FR-015.

Then confirm rejection of a non-Indeed URL:

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions \
  -H 'content-type: application/json' \
  -d '{"sourceKey":"indeed","url":"https://example.com/not-indeed"}' | jq
```

Expected: non-`200` with an error message — validates FR-016 / SC-008.

## 4. Run the subscription and confirm listings land in the feed

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions/<subscription-id>/run | jq
```

Then:

```bash
curl -s 'localhost:<api-port>/api/jobs?source=indeed' | jq '. | length'
```

Expected: count > 0 (or 0 with a successful run status if the search genuinely has no
matches — SC-002 / edge case "zero results"). Each returned job has non-empty `title`,
`company` (or explicitly empty, not dropped), and an openable `url` — validates SC-004.

## 5. Confirm dedupe on re-run

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions/<subscription-id>/run | jq '.new'
```

Expected: `0` new jobs on an immediate re-run against the same URL — validates US1
acceptance scenario 2 / SC-003.

## 6. Confirm enrichment fills in full detail

Wait for the auto-enqueued enrich task (or trigger a backfill sweep):

```bash
curl -s -X POST localhost:<api-port>/api/sources/indeed/enrich | jq
```

Then re-fetch one Indeed job and confirm `description` grew beyond the list-page snippet,
and `postedAt`/`remote` are populated where the source publishes them — validates US3 / SC-006.

## 7. Dashboard check

Open the Sources page in the dashboard: Indeed should appear in the source list (US2
scenario 1) and as a selectable option in the "New Subscription" form's source picker
(the `SUBSCRIPTION_SOURCES` list edit in `SourcesPage.tsx`).

## 8. Run the adapter's unit tests

```bash
cd apps/api && go test ./internal/jobsources/adapters/... -run Indeed -v
```

Expected: all pass against the recorded `testdata/indeed_*.html` fixtures, no live network
calls (constitution IV: per-language test discipline).
