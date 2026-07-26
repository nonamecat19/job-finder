# Quickstart: Validate the Djinni Basic-Search Mode

## Prerequisites

- Local stack running: `make up` (Postgres, Redis, `apps/api`, `apps/dashboard`)
- Implementation complete:
  - `validateDjinniSubscriptionURL` added to `apps/api/internal/subscriptions/service.go`
  - `DjinniSearchMode` discriminator added in
    `apps/api/internal/jobsources/adapters/djinni_searchmode.go`
  - `scrapeDashboard` / `scrapeBasicSearch` split in
    `apps/api/internal/jobsources/adapters/djinni.go`
  - `summarizeDjinniBasicSearch` added in
    `apps/dashboard/src/features/sources/djinniSearchSummary.ts` and wired into
    `SourcesPage.tsx`'s `SubscriptionRow`

## 1. Confirm both modes are accepted at save time

Save both example URLs from the spec:

```sh
curl -s -X POST localhost:8080/api/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{"sourceKey":"djinni","name":"Node.js 2y-5y remote","url":"https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote"}'

curl -s -X POST localhost:8080/api/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{"sourceKey":"djinni","name":"Golang 1y-3y $1500","url":"https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote"}'
```

Expected: both `201` with the created subscription. Then confirm rejection of a
neither-shape URL (SC-007):

```sh
curl -s -X POST localhost:8080/api/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{"sourceKey":"djinni","url":"https://example.com/jobs"}'
# expected: 4xx with "djinni subscription url must be a djinni.co url"

curl -s -X POST localhost:8080/api/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{"sourceKey":"djinni","url":"https://djinni.co/jobs/12345"}'
# expected: 4xx with "...looks like a single job posting, not a search results page"
```

## 2. Confirm the existing dashboard mode still saves and runs unchanged (SC-008, FR-013)

```sh
curl -s -X POST localhost:8080/api/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{"sourceKey":"djinni","name":"Dashboard sub","url":"https://djinni.co/my/dashboard/subs/123/"}'
```

Expected: `201` — same as before this feature — and a subsequent run takes the same
authenticated `scrapeDashboard` path. (Full verification of this step is step 6 below.)

## 3. Run the single-page search and confirm a clean stop (FR-004, SC-002)

```sh
curl -s -X POST localhost:8080/api/sources/djinni/enable
# Find the Golang subscription ID from step 1, then trigger it:
curl -s -X POST localhost:8080/api/subscriptions/<golang-sub-id>/run
```

Expected: run outcome `succeeded`, a non-negative `listingsFound` count (it may be small
or zero), and **no run recorded as `failed`**. Run `apps/api` logs after the run and
confirm `scrapeBasicSearch` exited after page 1 with a single `len(cards) == 0` break or
first-card-stable break — no loop, no error (FR-004). This is the **single page** case
explicitly called out by the user.

```sh
curl -s 'localhost:8080/api/runs/recent?source=djinni' | jq '.runs[0]'
```

## 4. Run the multi-page basic-search and confirm pagination + dedup (SC-001, SC-005)

```sh
curl -s -X POST localhost:8080/api/subscriptions/<nodejs-sub-id>/run
curl -s 'localhost:8080/api/jobs?source=djinni' | jq '.items | length'
# run again immediately:
curl -s -X POST localhost:8080/api/subscriptions/<nodejs-sub-id>/run
curl -s 'localhost:8080/api/jobs?source=djinni' | jq '.items | length'
```

Expected (SC-001): after the first run, the feed contains Node.js Djinni listings
attributed `sourceKey: "djinni"`. Expected (SC-005): after the immediate re-run, the
feed's total length is unchanged — `listingsFound` similar to before, but `listingsAdded`
(new-to-feed count) is `0`. Verified via the run record:

```sh
curl -s 'localhost:8080/api/runs/recent?source=djinni' | \
  jq '.runs[0] | {ok, found, new}'
```

## 5. Confirm the dashboard renders the full basic-search summary (FR-008, FR-009,
SC-003, SC-004)

Open the dashboard's Sources screen. For each of the two basic-search subscriptions saved
in step 1, the `SubscriptionRow` should show:

- **Node.js sub**: a summary that includes `Node.js`, the salary, the experience range as
  `"2–5 years"` (NOT `"2y, 3y, 4y, 5y"`), and `remote`. The exact delimiter is
  implementation-time (research.md R4 example: `"Node.js · $3000 · 2–5 years · remote"`).
- **Golang sub**: a summary with `Golang`, the salary, `"1–3 years"`, and `remote`.

For the dashboard subscription from step 2, the row's label should be unchanged from
before this feature shipped (FR-013, SC-008) — the existing default `sub.name ??
sub.sourceKey` + truncated-url rendering.

Inspect the row text from the dashboard's DOM or via the Sources page test snapshot.
Verify all four SC-004 verbatim shapes collapse correctly:

| URL `exp_level` values | Expected label fragment |
|---|---|
| `2y,3y,4y,5y` (Node.js sub) | `"2–5 years"` |
| `1y,2y,3y` (Golang sub) | `"1–3 years"` |
| `1y,3y` (non-consecutive, save a fresh test sub) | `"1, 3 years"` |
| `2y,2y` (duplicates, save a fresh test sub) | `"2 years"` |
| `3y,1y,2y` (out of order, save a fresh test sub) | `"1–3 years"` |

## 6. Confirm cross-app tests pass (constitution IV — `make test-lint`)

```sh
make test-lint
```

Expected: both the Go suite (`go test ./...`, including the new
`djinni_searchmode_test.go` cases for URL-shape discrimination and the new
`djinni_test.go` case for single-page pagination) and the React suite (`vitest`, including
the new `djinniSearchSummary.test.ts` cases) pass. No `packages/shared` rebuild is
required (research.md R3 — no DTO field added), but `pnpm --filter @job-finder/shared
build` runs harmlessly if make does it.

## 7. Confirm basic-search works with no Djinni login (FR-018, SC-009)

Stop the stack, unset `DJINNI_EMAIL`/`DJINNI_PASSWORD` in your environment (or temporarily
blank the source config), restart `apps/api`, then re-run step 3.

Expected: the Golang basic-search run still completes successfully against the public
`/jobs/` page — no "djinni requires login but no credentials configured" error from
`fetchDoc`. The dashboard `subs/{id}/` mode, by contrast, is *expected* to fail that same
run with the existing login-required error from `djinni.go:70-72` — confirming the two
modes separate their session requirements cleanly.

## 8. Confirm a block/challenge is reported distinctly (FR-014, SC-006)

If/when Djinni blocks the public `/jobs/` request during a run (rate limit, bot
challenge — the existing the dashboard-mode posture already covers this), the
subscription's status should show unhealthy with a human-readable reason within one run
cycle, distinct from "no matching listings":

```sh
curl -s 'localhost:8080/api/runs/recent?source=djinni' | \
  jq '.runs[0] | {ok, verdict, blockReason}'
```

Expected on a block: `ok: false`, `verdict: "blocked"` (or the existing equivalent), and
a non-empty `blockReason` string. The same is true of the dashboard mode today; the new
mode just verifies it uses the existing failure channel rather than introducing a new one.