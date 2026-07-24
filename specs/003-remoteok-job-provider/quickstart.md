# Quickstart: Validating the RemoteOK Job Source

Prerequisites: local stack running (`make up` or `make dev`), API reachable at its usual
local URL, dashboard Sources page reachable, Docker Compose Postgres/Redis up per the
project's standard dev setup.

## 1. Confirm RemoteOK is registered

```bash
curl -s localhost:<api-port>/api/sources | jq '.[] | select(.key=="remoteok")'
```

Expected: an entry with `"key": "remoteok"`, `"kind": "api"`, `"enabled": true`,
`"healthy": true` (default state before any run) — see
contracts/remoteok-adapter.md's REST table and data-model.md's JobSource section.

## 2. Health check

```bash
curl -s -X POST localhost:<api-port>/api/sources/remoteok/test | jq
```

Expected: `{"healthy": true, "reason": "..."}` (or `false` + a human-readable reason if
RemoteOK is currently blocking/unreachable) — validates US2 acceptance scenario 3 and
FR-006.

## 3. Save a subscription (tag URL)

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions \
  -H 'content-type: application/json' \
  -d '{"sourceKey":"remoteok","url":"https://remoteok.com/remote-golang-jobs","name":"RemoteOK Golang"}' | jq
```

Expected: `200` with the created subscription. Validates US2 acceptance scenario 2 /
FR-014.

Then confirm rejection of a non-RemoteOK URL:

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions \
  -H 'content-type: application/json' \
  -d '{"sourceKey":"remoteok","url":"https://example.com/not-remoteok"}' | jq
```

Expected: non-`200` with an error message — validates FR-015 / SC-008.

## 4. Run the subscription and confirm listings land in the feed

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions/<subscription-id>/run | jq
```

Then:

```bash
curl -s 'localhost:<api-port>/api/jobs?source=remoteok' | jq '. | length'
```

Expected: count > 0 (or 0 with a successful run status if the tag genuinely has no
matches — SC-002 / edge case "zero results"). Each returned job has non-empty `title`,
`company`, `remote: true`, and an openable `url` — validates SC-004.

## 5. Confirm dedupe on re-run

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions/<subscription-id>/run | jq '.new'
```

Expected: `0` new jobs on an immediate re-run against the same URL — validates US1
acceptance scenario 2 / SC-003.

## 6. Confirm enrichment / unavailable-listing handling

Wait for the auto-enqueued enrich task (or trigger a backfill sweep):

```bash
curl -s -X POST localhost:<api-port>/api/sources/remoteok/enrich | jq
```

Then re-fetch one RemoteOK job and confirm `description`/`tags`/`postedAt` are populated —
validates US3 / SC-006. Because RemoteOK's API already returns full descriptions at
ingestion, expect most listings to already satisfy SC-006 before enrichment runs; the
enrichment pass mainly confirms the listing is still live (see
research.md#R5).

## 7. Dashboard check

Open the Sources page in the dashboard: RemoteOK should appear in the source list (US2
scenario 1) and as a selectable option in the "New Subscription" form's source picker
(the `SUBSCRIPTION_SOURCES` list edit in `SourcesPage.tsx`).

## 8. Run the adapter's unit tests

```bash
cd apps/api && go test ./internal/jobsources/adapters/... -run RemoteOK -v
```

Expected: all pass against the recorded `testdata/remoteok_*.json` fixtures, no live
network calls (constitution IV: per-language test discipline).
