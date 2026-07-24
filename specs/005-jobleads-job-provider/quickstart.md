# Quickstart: Validating the JobLeads Job Source

Prerequisites: local stack running (`make up` or `make dev`), API reachable at its usual
local URL, dashboard Sources page reachable, Docker Compose Postgres/Redis up per the
project's standard dev setup, and `JOBLEADS_EMAIL` / `JOBLEADS_PASSWORD` set to a real
JobLeads account's credentials (env vars — see [research.md#R2](./research.md#r2-credential-storage)).

## 1. Confirm JobLeads is registered

```bash
curl -s localhost:<api-port>/api/sources | jq '.[] | select(.key=="jobleads")'
```

Expected: an entry with `"key": "jobleads"`, `"kind": "scrape"`, `"enabled": true`,
`"healthy": true` (default state before any run) — see
contracts/jobleads-adapter.md's REST table and data-model.md's JobSource section.

## 2. Confirm credential-missing behavior (no env vars set)

With `JOBLEADS_EMAIL`/`JOBLEADS_PASSWORD` unset:

```bash
curl -s -X POST localhost:<api-port>/api/sources/jobleads/test | jq
```

Expected: `{"healthy": false, "reason": "..."}` mentioning missing credentials — validates
the "credentials not configured" precondition in contracts/jobleads-adapter.md.

## 3. Health check (with credentials configured)

```bash
curl -s -X POST localhost:<api-port>/api/sources/jobleads/test | jq
```

Expected: `{"healthy": true, "reason": "..."}` (or `false` + a human-readable reason if
JobLeads is currently blocking/unreachable/unauthorized) — validates US2 acceptance scenario
3 and FR-006.

## 4. Save a subscription (saved-search URL)

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions \
  -H 'content-type: application/json' \
  -d '{"sourceKey":"jobleads","url":"https://www.jobleads.com/job-search?q=golang","name":"JobLeads Golang"}' | jq
```

Expected: `200` with the created subscription. Validates US2 acceptance scenario 2 / FR-014.

Then confirm rejection of a non-JobLeads URL:

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions \
  -H 'content-type: application/json' \
  -d '{"sourceKey":"jobleads","url":"https://example.com/not-jobleads"}' | jq
```

Expected: non-`200` with an error message — validates FR-015 / SC-008.

## 5. Run the subscription and confirm listings land in the feed

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions/<subscription-id>/run | jq
```

This exercises the login flow on first run (session cookie not yet cached). Then:

```bash
curl -s 'localhost:<api-port>/api/jobs?source=jobleads' | jq '. | length'
```

Expected: count > 0 (or 0 with a successful run status if the search genuinely has no
matches — SC-002 / edge case "zero results"). Each returned job has non-empty `title`,
`company`, and an openable `url` — validates SC-004.

## 6. Confirm dedupe on re-run

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions/<subscription-id>/run | jq '.new'
```

Expected: `0` new jobs on an immediate re-run against the same URL — validates US1 acceptance
scenario 2 / SC-003.

## 7. Confirm session-expiry handling

Manually clear the stored session cookie (or wait for it to expire), then re-run:

```bash
curl -s -X POST localhost:<api-port>/api/subscriptions/<subscription-id>/run | jq
```

Expected: the adapter re-logs-in transparently and the run succeeds — OR, if credentials are
now invalid, the run fails with a reason distinguishable as "authentication required" (not
"no matching listings" or "could not be interpreted") — validates FR-011 and the "stored
account credentials expire or are revoked" edge case.

## 8. Confirm enrichment / unavailable-listing handling

Wait for the auto-enqueued enrich task (or trigger a backfill sweep):

```bash
curl -s -X POST localhost:<api-port>/api/sources/jobleads/enrich | jq
```

Then re-fetch one JobLeads job and confirm `description`/`postedAt` are populated —
validates US3 / SC-006.

## 9. Dashboard check

Open the Sources page in the dashboard: JobLeads should appear in the source list (US2
scenario 1) and as a selectable option in the "New Subscription" form's source picker (the
`SUBSCRIPTION_SOURCES` list edit in `SourcesPage.tsx`).

## 10. Run the adapter's unit tests

```bash
cd apps/api && go test ./internal/jobsources/adapters/... -run JobLeads -v
```

Expected: all pass against the recorded `testdata/jobleads_*.html` fixtures, including a
login-flow test against a fake HTTP server — no live network calls or real JobLeads
credentials in unit tests (constitution IV: per-language test discipline).
