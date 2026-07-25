# Quickstart: Validate the Wellfound Job Source

## Prerequisites

- Local stack running: `make up` (Postgres, Redis, `apps/api`, `apps/dashboard`)
- Implementation complete: `WellfoundAdapter` registered in `cmd/server/compose.go`,
  Wellfound added to `SourcesPage.tsx`'s subscription-form dropdown

## 1. Confirm the source is registered

```sh
curl -s localhost:8080/api/sources | jq '.[] | select(.key == "wellfound")'
```

Expected: one object with `"key": "wellfound"`, `"kind": "scrape"` — validates FR-001,
FR-016.

## 2. Save a subscription

Via the dashboard: Sources screen → select **Wellfound** → paste a Wellfound search-results
URL → Save.

Or via API:

```sh
curl -s -X POST localhost:8080/api/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{"sourceKey":"wellfound","name":"Wellfound Go remote","url":"https://wellfound.com/role/r/golang-engineer"}'
```

Expected: `201` with the created subscription — validates US2 acceptance scenario 2, FR-014.
Re-run with a non-Wellfound URL (e.g. an Indeed URL) and confirm a `4xx` with a
human-readable rejection reason — validates FR-015, SC-008.

## 3. Enable the source and run it on demand

Dashboard: toggle Wellfound enabled, click "Run now". Or:

```sh
curl -s -X POST localhost:8080/api/sources/wellfound/enable
curl -s -X POST localhost:8080/api/sources/wellfound/run
```

Expected: run completes with an outcome of `succeeded` or `partial`, a non-negative
`listingsFound` count and a `listingsAdded` count (FR-007) — or, if Wellfound blocked the
request, a `failed` outcome carrying a human-readable reason distinguishable from "zero
results" (FR-011, SC-005). A blocked outcome here is an expected possible result given
Wellfound's anti-bot posture (research.md R3), not a bug — confirm the reason string clearly
says "blocked" rather than misreporting as "no listings found".

## 4. Confirm listings reached the feed

```sh
curl -s 'localhost:8080/api/jobs?source=wellfound' | jq '.items | length, .items[0]'
```

Expected: at least one job with non-empty `title`, `company`, and an openable `url` (SC-004),
attributed `sourceKey: "wellfound"` — validates US1 acceptance scenario 1, FR-002, FR-003.

## 5. Re-run and confirm deduplication

Run step 3 again immediately. Expected: `listingsFound` similar to before, but
`listingsAdded` (new-to-feed count) is `0` — no duplicate feed entries — validates US1
acceptance scenario 2, FR-004, SC-003.

## 6. Confirm source filtering in the feed

In the dashboard job feed, filter/sort by source and confirm Wellfound appears as a
selectable option (US1 acceptance scenario 3).

## 7. Confirm enrichment

Open one ingested Wellfound job in the dashboard job detail view (or
`GET /api/jobs/{id}`). Expected: after enrichment runs, `description` is the full posting
text plus qualifications (longer than the ingestion-time summary), and `postedAt` is
resolved — validates US3, SC-006, FR-009.

Then simulate a removed/session-gated listing (or find one naturally) and confirm its
existing summary data is preserved rather than cleared, with the listing marked unavailable
— validates FR-009's second acceptance scenario.

## 8. Confirm health check reporting

```sh
curl -s -X POST localhost:8080/api/sources/wellfound/test
```

Expected: `{"healthy": true|false, "reason": "..."}` — `reason` is human-readable and, on
failure, distinguishes reachability/parsing/blocking causes — validates FR-006, US2
acceptance scenario 3.

## 9. Confirm disabling stops requests

Disable Wellfound, trigger a scheduled ingestion cycle (or wait for the next one), and
confirm via logs/run history that no Wellfound request was attempted — validates FR-005, US2
acceptance scenario 4.

## 10. Confirm pacing and page cap

During a run against a subscription with many results, confirm (via logs or timing) that
requests are spaced at least 500ms apart and pagination stops at the fixed page cap rather
than continuing unbounded — validates FR-010.

## 11. Run the adapter's unit tests

```sh
cd apps/api && go test ./internal/jobsources/adapters/... -run Wellfound -v
```

Expected: all pass against the recorded `testdata/wellfound_*.html` fixtures, including a
blocked-response fixture — no live network calls in unit tests (constitution IV: per-language
test discipline).
