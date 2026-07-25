# Quickstart: Validating the Himalayas Job Source

Prerequisites: local stack running (`make up` or `make dev`), API reachable at its usual local URL,
dashboard Sources page reachable, Docker Compose Postgres/Redis up per the project's standard dev
setup. No credentials or env vars are required — Himalayas is fully public (see
[research.md#r1-access-model--public-vs-login-gated](./research.md#r1-access-model--public-vs-login-gated)).

## 1. Confirm Himalayas is registered

```bash
curl -s localhost:<api-port>/api/sources | jq '.[] | select(.key=="himalayas")'
```

Expected: an entry with `"key": "himalayas"`, `"kind": "api"`, `"enabled": true`, `"healthy": true`
(default state before any run) — see contracts/himalayas-adapter.md's REST table and
data-model.md's JobSource section.

## 2. Health check

```bash
curl -s -X POST localhost:<api-port>/api/sources/himalayas/test | jq
```

Expected: `{"healthy": true, "reason": "..."}` (or `false` + a human-readable reason if Himalayas's
feed is currently unreachable or its shape has changed) — validates US2 acceptance scenario 3 and
FR-006.

## 3. Save a subscription (category search-page URL)

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions \
  -H 'content-type: application/json' \
  -d '{"sourceKey":"himalayas","url":"https://himalayas.app/jobs?categories=Backend-Engineering","name":"Himalayas Backend"}' | jq
```

Expected: `200` with the created subscription. Validates US2 acceptance scenario 2 / FR-014.

Then confirm rejection of a non-Himalayas URL and a Himalayas URL missing `categories`:

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions \
  -H 'content-type: application/json' \
  -d '{"sourceKey":"himalayas","url":"https://example.com/not-himalayas"}' | jq

curl -s -X POST localhost:<api-port>/api/subscriptions \
  -H 'content-type: application/json' \
  -d '{"sourceKey":"himalayas","url":"https://himalayas.app/jobs"}' | jq
```

Expected: both non-`200` with an error message — validates FR-015 / SC-008.

## 4. Run the subscription and confirm listings land in the feed

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions/<subscription-id>/run | jq
```

```bash
curl -s 'localhost:<api-port>/api/jobs?source=himalayas' | jq '. | length'
```

Expected: count > 0 (bounded by the page-sweep cap — see research.md R7), or 0 with a successful
run status if the category genuinely has no matches within the swept pages (SC-002 / edge case
"zero results"). Each returned job has non-empty `title`, `company`, an openable `url`, and
`remote: true` — validates SC-004 and the spec's Assumption that every Himalayas listing is remote.

## 5. Confirm dedupe on re-run

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions/<subscription-id>/run | jq '.new'
```

Expected: `0` new jobs on an immediate re-run against the same category — validates US1 acceptance
scenario 2 / SC-003.

## 6. Confirm full description is present immediately (no enrichment step)

```bash
curl -s 'localhost:<api-port>/api/jobs?source=himalayas' | jq '.[0].description' | wc -c
```

Expected: a non-trivial length (the full posting body, not a one-line teaser) — validates US3's
acceptance scenario directly at ingestion time, since Himalayas has no separate enrichment step
(research.md R6); there is no `/api/sources/himalayas/enrich` call to make.

## 7. Confirm response-shape-change failure is distinguishable

This is best validated by unit test (step 8) rather than live traffic, since it requires an
injected malformed response. The adapter's `APIURL` override field exists exactly for this —
see contracts/himalayas-adapter.md.

## 8. Dashboard check

Open the Sources page in the dashboard: Himalayas should appear in the source list (US2 scenario 1)
and as a selectable option in the "New Subscription" form's source picker (the
`SUBSCRIPTION_SOURCES` list edit in `SourcesPage.tsx`).

## 9. Run the adapter's unit tests

```bash
cd apps/api && go test ./internal/jobsources/adapters/... -run Himalayas -v
```

Expected: all pass against the recorded `testdata/himalayas_*.json` fixtures served by an
`httptest.Server`, including a case exercising the "response shape changed" failure path and a case
exercising local category/timezone filtering — no live network calls in the test suite (constitution
IV: per-language test discipline).
