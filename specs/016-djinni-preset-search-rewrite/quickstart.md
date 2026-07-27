# Quickstart: Djinni Preset-Search Rewrite

**Date**: 2026-07-28
**Spec**: [spec.md](./spec.md)
**Plan**: [plan.md](./plan.md)
**Contracts**: [contracts/README.md](./contracts/README.md)
**Data model**: [data-model.md](./data-model.md)

End-to-end validation scenarios for the rewrite. These are the manual
and automated checks that prove the feature works; they reference the
contracts above rather than duplicating them. No full implementation
code or test bodies are included here — those live in `tasks.md` and
the implementation phase.

---

## Prerequisites

- Local stack: `make up` brings up Postgres + Redis via Docker Compose
  (per the constitution's local-first rule).
- Go backend: `make run-backend` (or via `process-hive`).
- Dashboard: `make run-frontend` (or via `process-hive`).
- No `DJINNI_EMAIL`/`DJINNI_PASSWORD` in `.env` — the rewrite runs
  anonymous and these env vars are deleted. (The validation scenarios
  below intentionally do not set them.)
- `DJINNI_DETAIL_DELAY_MS` stays at its default (1500 ms); do not edit
  it just for these tests.

---

## Scenario 1 — Save and run a preset subscription (P1, SC-001/SC-002)

1. Open the dashboard → Sources screen → New Subscription.
2. Paste the spec's Golang example:
   `https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote`.
3. **Expected**: the form accepts the URL; the row appears in the
   Subscriptions list with the label
   `Golang · $1500 · 1–3 years · remote` (matches [contracts/README.md](./contracts/README.md) C4
   row 2 — verify the en-dash renders as `–` U+2013, not `-`).
4. Trigger a manual run for the subscription (Sources screen → Run, or
   `make run-backend` cron picks it up).
5. **Expected**: within one run cycle, the SourceRun status is
   `ok` with the actual job count (not `failed`, not `blocked`). For
   the Golang example with salary=1500 and exp_level 1y–3y the search
   typically fits on a single page — verify the run made exactly **2**
   `FetchHTML` calls (page 1 + the empty page 2) via the backend logs,
   i.e. no pagination loop, no retry (SC-002). Confirm there is no
   error capturing a session/login path — the log should show only the
   anonymous fetch.
6. **Expected**: Djinni listings appear in the job feed within 5 minutes
   (SC-001), attributed to Djinni.

### Automated equivalent

- `go test ./apps/api/internal/jobsources/adapters/ -run
  TestDjinniSearchBasicSearch` — covers single-page, multi-page,
  redirect-loop, param-preserve, page-strip cases (per R10 of
  [research.md](./research.md)).
- `go test ./apps/api/internal/jobsources/adapters/ -run
  TestDjinniDetect` — covers the preset-shape discriminator (minus
  the dashboard cases that are pruned).

---

## Scenario 2 — Multi-page preset subscription (SC-002 multi-page branch)

1. Save a preset URL whose filters return many results (e.g. drop the
   `salary` filter, broaden the keyword):
   `https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Go&employment=remote`.
2. Run it.
3. **Expected**: backend logs show page 1, page 2, page 3, … in
   sequence until an empty page or the 50-page cap fires (the existing
   `djinniMaxSubscriptionPages` loop guard). No parallel-page fetches
   (sequential pacing per FR-016). The first-card-href-equals-previous
   redirect-loop guard should NOT trigger for a legitimate multi-page
   search.

### Automated equivalent

- `go test ... -run TestDjinniSearchBasicSearchMultiPage` (kept from
  v015).
- `go test ... -run TestDjinniSearchBasicSearchLoopGuard` — the
  redirect-to-page-1 detection (kept from v015).

---

## Scenario 3 — Single page, no loop (SC-002, edge case)

1. Save a preset URL that returns fewer jobs than one full page (the
   spec's Golang example with `salary=1500&exp_level=1y&2y&3y`).
2. Run it.
3. **Expected**: the SourceRun status is `ok` with the actual count, the
   run made exactly 2 `FetchHTML` calls (page 1 + the empty page 2),
   and there is no `failed`/`blocked` verdict. Re-running immediately
   adds **zero** new feed entries (SC-005 dedup).

### Automated equivalent

- `go test ... -run TestDjinniSearchBasicSearchSinglePage` (kept from
  v015; the test asserts exactly 2 fetches and 2 cards).

---

## Scenario 4 — Save-time rejection of `subs/{id}/` (FR-008 / SC-007)

1. Open the New Subscription form and paste
   `https://djinni.co/my/dashboard/subs/42/`.
2. **Expected**: the save is rejected with the specific reason from
   [contracts/README.md](./contracts/README.md) C1:

   > "Djinni subscriptions support only preset-search URLs
   > (`djinni.co/jobs/?search_type=basic-search&…`); dashboard URLs are
   > no longer supported."

3. Try other invalid shapes:
   - `https://djinni.co/jobs/?primary_keyword=Golang` (no `search_type`)
     → rejected.
   - `https://djinni.co/jobs/123-senior-go` (single posting) →
     rejected.
   - `https://djinni.io/jobs?...` (wrong host) → rejected.

4. Try a valid preset with an unrecognized extra param:
   `https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Go&foo=bar`
   → **accepted** (the unrecognized `foo` is preserved and re-issued,
   not interpreted — see C1).

### Automated equivalent

- `go test ./apps/api/internal/subscriptions/ -run
  TestValidateDjinniSubscriptionURL` (existing test, updated to the
  collapsed one-branch validator).

---

## Scenario 5 — Legacy `subs/{id}/` subscriptions are pruned (FR-009 / SC-009)

> ⚠️ This scenario runs the destructive migration. Use a throwaway
> local database; do not run against a database with real saved subs
> you can't afford to lose.

1. With the **old** code (before the rewrite), save a
   `https://djinni.co/my/dashboard/subs/42/` subscription. Confirm the
   row exists in `Subscription`.
2. Apply the rewrite (including migration `00027_…`). Run
   `make migrate-up` (or whatever the repo's goose entry point is).
3. **Expected**:
   - The `subs/42/` row is gone from `Subscription`.
   - One row exists in the new `DjinniLegacySubAudit` table with
     `subscriptionId`, `name`, `url="https://djinni.co/my/dashboard/subs/42/"`,
     `deletedAt` set to the migration run time.
   - Any preset-search `Subscription` rows are **untouched** (the
     migration's `WHERE url LIKE '%/my/dashboard/subs/%'` excludes
     them — see [data-model.md](./data-model.md) §3).
   - The `JobSource` row with `key="djinni"` still exists (the adapter
     still runs preset searches against it).
4. Re-running `make migrate-up` should be a no-op on a clean DB
   (idempotent — see [contracts/README.md](./contracts/README.md) C3).

### Validation query

```sql
SELECT "subscriptionId", "name", "url", "deletedAt"
FROM "DjinniLegacySubAudit"
ORDER BY "deletedAt";
```

should list exactly the deleted dashboard subs.

```sql
SELECT count(*) FROM "Subscription" WHERE "sourceKey" = 'djinni'
  AND "url" LIKE '%/my/dashboard/subs/%';
```

should return `0`.

```sql
SELECT count(*) FROM "Subscription" WHERE "sourceKey" = 'djinni'
  AND "url" LIKE '%/jobs/?search_type=basic-search%';
```

should return the count of valid preset subs (unchanged by the
migration).

---

## Scenario 6 — No-login requirement (FR-005 / SC-001)

1. Ensure `.env` has **no** `DJINNI_EMAIL`/`DJINNI_PASSWORD` (the
   rewrite deletes those env vars anyway; if they linger, unset them).
2. Save the preset URL from Scenario 1 and run it.
3. **Expected**: the run completes against the public `djinni.co/jobs/`
   page without ever attempting a login or consulting session cookies.
   The backend logs should show fetches against `djinni.co/jobs/...`
   and **no** `POST /my/login/...` call, **no** cookie-set attempt.

### Automated equivalent

- The existing `TestDjinniSearchBasicSearch*` tests already run with
  no session; the rewrite's pruned `DjinniAdapter{Scraping: ...}` no
  longer carries the `Session` field, so compilation alone is the
  guard.

---

## Scenario 7 — Display summary rules (SC-003 / SC-004)

1. Save each of the following preset URLs and check each row's label:
   - `?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote`
     → `Node.js · $3000 · 2–5 years · remote`
   - `?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote`
     → `Golang · $1500 · 1–3 years · remote`
   - `?search_type=basic-search&primary_keyword=Go&exp_level=1y&exp_level=3y`
     (non-consecutive) → `Go · 1, 3 years`
   - `?search_type=basic-search&primary_keyword=Golang` (only keyword)
     → `Golang`
   - `?search_type=basic-search&primary_keyword=Go&exp_level=2y&exp_level=2y`
     (duplicate) → `Go · 2 years` (deduped)
   - `?search_type=basic-search&primary_keyword=Go&exp_level=3y&exp_level=1y&exp_level=2y`
     (out-of-order consecutive) → `Go · 1–3 years`

2. **Expected**: every label matches the C4 contract exactly, including
   the en-dash (`–` U+2013), not a hyphen.

### Automated equivalent

- `pnpm --filter @job-finder/dashboard test -- src/features/sources/djinniSearchSummary.test.ts`
  (existing vitest suite, kept verbatim from v015 — covers every
  contract row above).

---

## Scenario 8 — Failure posture (FR-015 / SC-006)

1. Configure a preset subscription whose run is **blocked** by Djinni
   (rate limit / anti-bot): easiest way is a temporary block by hitting
   a real Djinni URL many times in quick succession against the same
   retrieval rung, OR use a local fake server that returns a 403 with
   an anti-bot page body.
2. Run the subscription.
3. **Expected**: the SourceRun verdict is `blocked` (not `ok`, not
   `failed-no-results`), with a human-readable reason. After 3
   consecutive blocked/failed runs, the source is flagged `unhealthy`
   on the Sources screen. No other source's runs are affected (the
   generic ingestion handler isolates per-source verdicts).

### Automated equivalent

- The v015 test suite already covers the block detection path; the
  rewrite doesn't change `computeVerdict`/`flagIfUnhealthy`. No new
  automated test needed unless the existing one is removed.

---

## Scenario 9 — Non-Djinni sources are unaffected (FR-019 / SC-010)

1. With the rewrite applied, run a RemoteOK and a Himalayas
   subscription before and after the code change.
2. **Expected**: identical run outcomes (same job counts, same verdicts,
   same SourceRun status transition). The Sources screen lists them in
   the same position (registry ordering unchanged).

### Automated equivalent

- `make test-lint` runs the full Go + vitest suite; the v016 diff should
  not touch any non-Djinni adapter or test. A pre-rewrite run vs.
  post-rewrite run of `make test-lint` should pass both.

---

## Scenario 10 — Static checks (FR-006 / FR-007 / SC-008)

After the rewrite is applied, run:

```bash
test ! -f apps/api/internal/jobsources/adapters/djinni_session.go   # file deleted
! grep -n "DjinniModeDashboard\|scrapeDashboard" apps/api/internal/jobsources/adapters/djinni.go apps/api/internal/jobsources/adapters/djinni_searchmode.go
! grep -rn "DJINNI_EMAIL\|DJINNI_PASSWORD" apps/api/ .env.example
! grep -n "Platform.DjinniSession\|DjinniSession " apps/api/cmd/server/platform.go apps/api/cmd/server/compose_sources.go
! grep -n "UsesUserAccount" apps/api/internal/jobsources/adapters/djinni.go
! grep -n "sessionCookie" apps/api/internal/jobsources/adapters/djinni.go        # orphaned blob documented, not read by adapter
```

All should be empty results (zero matches) — the static confirmation
that the legacy paths and login config are gone (SC-008).