# Quickstart: Employer ATS Board Sources

Validation guide for US1–US4 in [spec.md](./spec.md). Assumes the standard local stack is up
(`make up`) and migrations applied.

## Prerequisites

- `make up` running (Postgres, Redis, api, dashboard).
- At least a few existing `Job` rows in the DB whose `url` points at a Greenhouse or Lever board
  (real data from an existing source run, or seeded fixtures) — needed for US2/candidate
  discovery.

## 1. US1 — read one employer per vendor

For each of the 5 vendors, register one known-real employer board directly:

```bash
curl -X POST localhost:8080/api/roster \
  -d '{"url":"https://boards.greenhouse.io/<real-token>"}'
# repeat for lever.co/<token>, jobs.ashbyhq.com/<token>, apply.workable.com/<token>,
# jobs.smartrecruiters.com/<token>
```

Then run each vendor's source on demand (reuses existing endpoint):
```bash
curl -X POST localhost:8080/api/sources/greenhouse/run
```

**Expected**: `SourceRun` row for `greenhouse` with `ok=true`, `found>0`; corresponding `Job` rows
in the feed with `sourceKey=greenhouse`, `title`, `company`, `location`, `remote`, `url`,
`description` all populated (spec Acceptance Scenario 1). Re-running produces `new=0` on the
second call for the same employer (Scenario 2). No env var/API key was needed for any of the five
(Scenario 4).

## 2. US2 — candidate discovery

```bash
curl -X POST localhost:8080/api/roster/discover
curl localhost:8080/api/roster/candidates
```

**Expected**: candidates list names the correct vendor + employer for board-linked jobs already in
the DB (Acceptance Scenario 1). Accept one:
```bash
curl -X POST localhost:8080/api/roster/candidates/<id>/accept
curl -X POST localhost:8080/api/sources/<vendor>/run
```
**Expected**: that employer's postings appear after the run (Scenario 2). Reject another and
re-run discovery — it must not reappear (Scenario 3).

Paste an unsupported vendor URL:
```bash
curl -X POST localhost:8080/api/roster -d '{"url":"https://jobs.example-unsupported.com/x"}'
```
**Expected**: `422 unsupported_vendor` naming the 5 supported vendors (Edge Case).

## 3. US3 — merge with an aggregator copy

1. Run an existing aggregator source (e.g. `adzuna`) that happens to surface a posting also on a
   registered employer board.
2. Run the matching board vendor source.

**Expected**: feed shows one `Job` row, `url` now the board's apply URL, `seenOnSources` contains
both source keys (Acceptance Scenario 1–2). If the aggregator copy was already saved/scored/moved
on the board before the merge, that `Application`/`MatchResult`/board-position state is unchanged
after the merge (Scenario 3). Two distinct same-titled jobs at different companies remain two rows
(Scenario 4).

## 4. US4 — Sources screen parity

In the dashboard Sources page: each of the 5 vendors appears as a source with enable/disable/run/
health-check controls identical to existing sources (Acceptance Scenario 1). After a run with a
deliberately broken employer (bad token) mixed with good ones, the run record shows employers
read, postings found/new, and employers failed — the broken employer's failure does not zero out
the others' results (Scenarios 2–3). An employer with several consecutive zero-posting runs shows
a stale flag in the roster view (Scenario 4).

## Cleanup

```bash
curl -X DELETE localhost:8080/api/roster/<employer-id>
```
Confirms previously ingested `Job` rows from that employer remain (FR-012).
