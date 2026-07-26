# Phase 1 Data Model: Djinni Basic-Search Mode

No schema migration. No new source key. No new cross-language DTO field. This feature
reuses the existing `subscriptions` table, the existing `job_sources.djinni` row, and the
existing `SubscriptionDto.url` string field; only new *values* (basic-search URLs in
`subscriptions.url`) and new *pure display/validation logic* are introduced.

## Entities

### Source Subscription (existing table `subscriptions`, existing Go type
`sqlcgen.Subscription`, existing `dto.SubscriptionDto`)

| Field | Basic-search mode behavior |
|---|---|
| `source_key` | `"djinni"` — unchanged |
| `url` | operator-pasted `https://djinni.co/jobs/?search_type=basic-search&primary_keyword=...&salary=...&exp_level=...&employment=remote` URL; validated at save by the new `validateDjinniSubscriptionURL` (research.md R5) — accepts both this shape and the existing `/my/dashboard/subs/<id>/` shape, rejects neither-shape URLs with a human-readable reason |
| `enabled` | operator-controlled, unchanged |
| `cron` | operator-controlled, unchanged |
| `last_run_at` | set by run outcome, unchanged |
| `name` | operator-supplied, unchanged |

No new columns. The `url` string already encodes which Djinni mode the row uses; the new
discriminator (`DjinniDetect`, research.md R2) derives the mode on demand at run time and
on display, replacing the implicit "it's whatever `scrapeSubscription` reads" coupling
today. The single source of truth stays the URL the operator pasted.

### Job Source (existing table `job_sources`, existing `djinni` row)

No change. The row keyed `djinni` already exists (upserted at startup,
`apps/api/cmd/server/compose.go`). The basic-search mode is *not* a new source — it reuses
the same source row, the same adapter, the same `Kind == "scrape"`, the same health/run
recording. The dashboard mode's `enabled`/`healthy`/`config` semantics carry over
unchanged (FR-013, SC-008).

A `DjinniConfigStore` password (`DJINNI_EMAIL`/`DJINNI_PASSWORD`) is no longer required
for the source to be operable: the basic-search mode runs against the public `/jobs/` page
anonymously (FR-018, SC-009). The dashboard mode still needs credentials when its
subscriptions run; the existing empty-credentials degrades-to-anonymous behavior in
`DjinniSession.Ensure` is unchanged.

### Djinni Search Mode (new, in-memory discriminator — not persisted)

A pure-logic enumeration derived from a saved `subscriptions.url`:

| Mode | URL shape |
|---|---|
| `DjinniModeDashboard` | host `djinni.co` (or `www.djinni.co`), path `/my/dashboard/subs/<id>/` |
| `DjinniModeBasicSearch` | host `djinni.co` (or `www.djinni.co`), path `/jobs` or `/jobs/`, query `search_type=basic-search` present |
| `DjinniModeUnknown` | anything else — rejected at save time per R5; never persisted |

The discriminator is consumed in two places only:
1. `subscriptions.validateDjinniSubscriptionURL` (Go, save time): `Unknown` → reject with
   a human-readable reason; `Dashboard` or `BasicSearch` → accept.
2. `DjinniAdapter.Search` (Go, run time): branches to `scrapeDashboard` (`Dashboard`) or
   `scrapeBasicSearch` (`BasicSearch`). Both call a shared pagination helper so the
   existing single-page / loop / 50-page cap guards (R1) are reused; only the function
   name differs in logs.

No `Mode` column is added to `subscriptions`; the discriminator is re-derived from `url`
whenever needed. This avoids drift between a denormalized column and the URL itself
(constitution III).

### Basic-Search Filters (new, in-memory parse — not persisted)

When a `url` is in `DjinniModeBasicSearch`, the URL's query parameters are parsed into a
small non-persisted struct, used for **logging clarity** on the Go side and for **display**
on the TS side (each side parses its own copy from the same `url` string; the URL remains
the single source of truth):

| Field | URL query parameter | Display semantics |
|---|---|---|
| `PrimaryKeyword` | `primary_keyword` | rendered verbatim (free text, e.g. `"Node.js"`, `"Golang"`) |
| `Salary` | `salary` | rendered verbatim as a number, no currency conversion (research.md R4 — Djinni's `salary` param is a USD monthly net minimum) |
| `ExpLevels` | `exp_level` (repeated) | integer year set; displayed as `"2–5 years"` if consecutive, else `"1, 3 years"`; single value as `"3 years"`; absent → omitted (R4) |
| `Employment` | `employment` | rendered verbatim (e.g. `"remote"`); absent → omitted |

Absent filters are omitted from display rather than rendered as blank/null
(FR-012). Unknown query parameters (anything other than the four above plus
`search_type`, which is implied by the mode) are preserved in the saved URL and re-issued
by the run verbatim (FR-003), but are not interpreted and not displayed (spec edge case:
"Saved URL is a basic-search URL with an unrecognized extra query parameter").

### Normalized Job Listing (existing `dto.NormalizedJob`, existing `jobs` table)

No schema change, no new fields. A basic-search listing is parsed by the **existing**
`parseDjinniCards` (already shared by `/jobs/` and `/my/dashboard/subs/` markup, per the
comment at `djinni.go:194-197`), and is indistinguishable in storage from a dashboard-mode
listing — same `SourceKey == "djinni"`, same `ExternalID` (the trailing `/jobs/<id>`
segment), same `URL`, same `Description`/`SalaryRaw`/`Location`/`Remote`. The downstream
matching/scoring/generation pipelines see no difference between the two modes
(FR-016); cross-mode deduplication (a posting that appears in both a basic-search and a
dashboard subscription) relies on the existing `ExternalID`-keyed dedupe already in place.

### Source Run (existing table / type / bookkeeping)

No new fields, no new outcome vocabulary. A basic-search run uses the same
`succeeded` / `partial` / `failed` outcomes already produced by `Handler.ProcessTask`'s
`rec.Ok` / `rec.Fail` calls. The single-page case (SC-002) marks `succeeded` via the same
`rec.Ok` path the dashboard mode takes when its pages run out; the blocked-challenge case
uses the same `failed` outcome that the dashboard mode already uses (FR-014,
`djinniIsLoginPage`-driven). No `SourceRunDto` change.

### Djinni Search-Summary (new, presentation-only — TS only)

A pure-TS function `summarizeDjinniBasicSearch(url: string): string | null` in
`apps/dashboard/src/features/sources/djinniSearchSummary.ts`. Returns `null` when the URL
does not parse as the basic-search shape (in which case the existing default row label —
`sub.name ?? sub.sourceKey` plus the truncated `sub.url` — is used unchanged, preserving
dashboard-mode display behavior per FR-013). Returns a single-line summary string when it
does, e.g.:

- `primary_keyword=Node.js&salary=3000&exp_level=2y,3y,4y,5y&employment=remote`
  → `"Node.js · $3000 · 2–5 years · remote"`
- `primary_keyword=Golang&salary=1500&exp_level=1y,2y,3y&employment=remote`
  → `"Golang · $1500 · 1–3 years · remote"`
- `primary_keyword=Golang&exp_level=1y&exp_level=3y` (no salary, no employment,
  non-consecutive levels)
  → `"Golang · 1, 3 years"`

The exact delimiter set (` · `, the leading `$` on salary, the en-dash `–` in ranges,
the `years` suffix) is implementation-time presentation detail, captured here only to
anchor the SC-004 verifications. Whatever the final styling, the rule is: every present
filter appears; absent filters are omitted; consecutive `exp_level` collapses to a range,
non-consecutive displays as a sorted discrete list (R4, FR-009/FR-010).

## State Transitions

None beyond what already exists for every Djinni subscription:
- `enabled` toggled by operator action — unchanged.
- `healthy` flips based on `HealthCheck`/run outcomes — unchanged.
- A run moves through not-started → running → (succeeded | partial | failed) — unchanged.

The basic-search mode introduces one new "decision lane" at the start of a run — URL-shape
discrimination — but it is a pure read of `url`, not a state change.

## Validation Rules

- A saved Djinni subscription URL MUST resolve to host `djinni.co` or `www.djinni.co`
  (FR-007, R5).
- A saved Djinni subscription URL MUST match the dashboard shape (`/my/dashboard/subs/<id>/`)
  OR the basic-search shape (`/jobs/` or `/jobs` with `search_type=basic-search` query
  parameter); anything else is rejected at save time with a human-readable reason
  (FR-007, SC-007).
- A basic-search URL whose path looks like a single job posting (e.g. `/jobs/<id>` with no
  `search_type=basic-search` query) is rejected at save time with reason "looks like a
  single job posting, not a search results page" (mirrors the equivalent check in
  `validateIndeedSubscriptionURL`, `validateGlassdoorSubscriptionURL`).
- The run MUST start pagination from page 1 regardless of any `page=` value present in the
  saved URL — `scrapeBasicSearch` overwrites `page` (FR-003, R1; spec edge case "Basic-
  search URL with `page=N` already present when saved").
- The run MUST complete successfully with a count of zero when a basic-search URL returns
  no listings (spec edge case "Empty basic-search results (zero listings)"), matching the
  existing zero-results behavior already in `parseDjinniCards` (returns an empty slice,
  not an error).
- The run MUST complete successfully after a single page when the search has only one page
  of results (FR-004, SC-002) — guaranteed by the existing pagination guards
  (`len(cards) == 0` break, `cards[0].URL == seenFirstHref` break, 50-page cap) — no new
  guard is required.
- The display MUST deduplicate `exp_level` values and MUST treat them as a set when
  deciding range vs list (FR-010, R4) — verified by the `djinniSearchSummary.test.ts`
  cases for `2y,3y,4y,5y` → `"2–5 years"`, `1y,2y,3y` → `"1–3 years"`,
  `1y,3y` → `"1, 3 years"`, `2y,2y` → `"2 years"`, and `3y,1y,2y` → `"1–3 years"`.
- A dashboard-mode (`subs/{id}/`) subscription MUST keep its existing display label and
  existing run behavior verbatim (FR-013, SC-008) — verified by a SourcesPage test that
  renders a dashboard-mode row before and after the change and asserts identical label.