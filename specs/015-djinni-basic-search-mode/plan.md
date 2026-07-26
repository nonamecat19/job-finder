# Implementation Plan: Djinni Basic-Search Mode

**Branch**: `015-djinni-basic-search-mode` | **Date**: 2026-07-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/015-djinni-basic-search-mode/spec.md`

## Summary

Add a second Djinni subscription shape — a public
`djinni.co/jobs/?search_type=basic-search&...` URL — alongside the existing logged-in
`djinni.co/my/dashboard/subs/{id}/` dashboard shape. The work is three connected
corrections, not a new pipeline:

1. **Backend — recognize the new URL shape, validate it at save time, and paginate it
   correctly.** Today the Djinni adapter's `scrapeSubscription` already pages through any
   saved URL by appending `page=N`, and its loop guards (empty page + first-card-stable-href
   + 50-page cap) already end a single-page search cleanly — so single-page pagination
   "just works" once a basic-search URL is accepted. The gap is that the new URL has no
   save-time validator, so a malformed paste fails at run time; and the run path doesn't
   *recognize* the mode, so the dashboard mode's behavior is co-mingled with the new mode
   in code. We add `validateDjinniSubscriptionURL` (accepting both shapes, rejecting
   nothing-recognizable at save time per FR-007) and split the adapter's subscription
   handling into `scrapeDashboard` vs `scrapeBasicSearch` so the two modes are
   distinguishable in code and logs (FR-002), while sharing the pagination guard.
   The public `/jobs/` path is reachable anonymously, so the basic-search mode works
   with no `DJINNI_EMAIL`/`DJINNI_PASSWORD` configured (FR-018) — the session cookie is
   reused opportunistically when present, as today.

2. **Frontend — display every filter from a saved basic-search URL, with consecutive
   `exp_level` values collapsed to a range.** The `SubscriptionDto` already carries the
   raw `url` string (no new field, no schema change), so the parsing happens in a small
   pure-TS helper in `apps/dashboard/src/features/sources/djinniSearchSummary.ts`. The
   helper replaces the truncated-url row label for any Djinni subscription whose URL
   parses as the basic-search shape, rendering keyword, salary, `employment`, and either
   `"2–5 years"` (when the `exp_level` set is consecutive) or `"1, 3 years"` (when it isn't).
   Dashboard `subs/{id}/` URLs keep their existing label unchanged.

3. **No new cross-language boundary, no DB migration, no new source key, no new API
   endpoint, no shared-package change.** The `SubscriptionDto.url` field is the one
   contract the two apps already share; both sides parse the same URL shape from it.

## Technical Context

**Language/Version**: Go 1.x (`apps/api`), TypeScript 5.x (`apps/dashboard`), matching existing
adapters and the dashboard.

**Primary Dependencies**: `goquery` (HTML parsing), the existing `internal/scraping.Service`
(plain HTTP fetch), `internal/jobsources` (Adapter interface + registry), the existing
`internal/subscriptions` validation switch, the existing `SubscriptionsPanel` /
`SubscriptionRow` components in `apps/dashboard/src/features/sources/SourcesPage.tsx`.

**Storage**: Postgres — no schema change; the `subscriptions` row already stores the URL
string, and `job_sources` already has its `djinni` row (upserted at startup). No migration,
no new columns, no new typed `SearchQuery` fields. The `SubscriptionDto.URL` field is the
single shared contract surface.

**Testing**: `go test ./...` (`apps/api`) — table-driven tests for the URL-shape discriminator
and validator, plus basic-search pagination fixture tests following the existing
`djinni_test.go` inline-HTML test pattern. `vitest` for the dashboard's
`djinniSearchSummary` helper (unit tests on the pure URL→summary function), and a
`SourcesPage` integration-style snapshot for the new row label. `make test-lint` because
this change crosses the Go/TS boundary (constitution IV).

**Target Platform**: existing `apps/api` and `apps/dashboard` containers.

**Project Type**: Web service (Go API + React dashboard), no new project, no new adapter
directory beyond the files already under `apps/api/internal/jobsources/adapters/`.

**Performance Goals**: N/A — bounded by the existing Djinni pacing and the same 50-page cap
already enforced for the dashboard path.

**Constraints**:
- The dashboard (`subs/{id}/`) mode MUST remain behaviorally unchanged (FR-013, SC-008).
- Basic-search MUST run with no login configured (FR-018, SC-009), reusing the
  login-aware fetch path opportunistically when the session is present.
- Save-time validation MUST reject Djinni URLs that are neither shape (FR-007, SC-007)
  with a human-readable reason, not defer to run time.
- Single-page basic-search MUST NOT loop and MUST report success (FR-004, SC-002).
- Display MUST collapse consecutive `exp_level` sets to a range and MUST NOT collapse
  non-consecutive ones (FR-009, SC-004).

**Scale/Scope**: ~2 new Go files (one for the URL-shape helper + tests), one modified Go
file (`djinni.go` split into two scrape functions), one modified file (`subscriptions/
service.go` adds the `case "djinni"`), one new TS file + tests, one modified TSX file. No
migration, no shared-package rebuild, no swagger/openapi doc change.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — PASS. The change is discovery-only (`Search` /
  `FetchDetail` / `HealthCheck` paths on the Go side, display-only on the React side). It
  never submits, messages, or acts on a listing (FR-017).
- **II. Grounded Generation** — PASS. Not applicable — this feature adds raw listing
  discovery and a display summary, not generated content; downstream generation already
  sources only from ingested listing fields and the user's profile, unchanged by this
  feature.
- **III. Typed Contracts Across Service Boundaries** — PASS. **No new cross-language
  boundary is introduced.** The `SubscriptionDto.url` field is already a string in both the
  Go DTO (`internal/dto/jobs.go:99`) and the TS DTO (`packages/shared/src/index.ts:351`),
  generated/shared via the existing tygo → `generated.ts` flow. The new "URL → basic-search
  filter summary" mapping is a *pure display* function on the TS side and a *pure validation*
  function on the Go side; both parse the same URL shape, but neither introduces a new
  shared structured type. No hand-maintained duplicate type is added. No
  `packages/shared` rebuild is required.
- **IV. Test Discipline Per Language, Enforced at the Boundary** — PASS. Go tests grow in
  `apps/api` (`go test ./...`), TS tests grow in `apps/dashboard` (`vitest`). The feature
  touches both apps, so `make test-lint` is mandatory before merge — explicitly called out
  in the quickstart validation.
- **V. Local-First, Self-Hosted by Default** — PASS. Djinni is *external discovery* only
  (already named in the constitution's Technology & Architecture Constraints); the public
  `/jobs/` path is reachable anonymously and introduces no third-party paid AI dependency.
  Matching/scoring/generation remain local-Ollama, unchanged.

No violations. Complexity Tracking not needed.

## Project Structure

### Documentation (this feature)

```text
specs/015-djinni-basic-search-mode/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── djinni-url-shapes.md   # Phase 1: the URL-shape contract both apps recognize
└── tasks.md             # Phase 2 output (/speckit.tasks — not created here)
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── jobsources/
│   │   └── adapters/
│   │       ├── djinni.go                # MODIFIED — split subscription handling into
│   │       │                              #   scrapeDashboard (existing) vs scrapeBasicSearch
│   │       │                              #   (new), both reusing fetchDoc + the existing
│   │       │                              #   pagination guards; Search() routes by URL shape
│   │       ├── djinni_searchmode.go      # NEW — pure helpers: DjinniSearchMode enum
│   │       │                              #   (Dashboard | BasicSearch | Unknown), Detect(),
│   │       │                              #   and BasicSearchFilters parse of the URL query
│   │       ├── djinni_searchmode_test.go  # NEW — table-driven tests for the URL-shape
│   │       │                              #   discriminator + filter parse, including
│   │       │                              #   single-page, duplicate exp_level, out-of-order
│   │       │                              #   exp_level, missing filters, and unknown params
│   │       └── djinni_test.go            # MODIFIED — add basic-search pagination test
│   │                                       #   (single-page ends cleanly) and a save-time
│   │                                       #   validation interaction if needed
│   ├── subscriptions/
│   │   └── service.go                    # MODIFIED — add `case "djinni":` to
│   │                                       #   validateSubscriptionURL, calling a new
│   │                                       #   validateDjinniSubscriptionURL that accepts
│   │                                       #   both the dashboard URL and the basic-search
│   │                                       #   URL and rejects neither-shape URLs
│   └── seed/
│       └── subscriptions.go              # MODIFIED (optional) — add a basic-search
│                                          #   seed subscription alongside the existing
│                                          #   djinni dashboard seed, mirroring the spec's
│                                          #   two example URLs
└── cmd/server/
    └── compose.go                        # NOT MODIFIED — Djinni adapter already
                                             registered; no new adapter, no new source key

apps/dashboard/
└── src/features/sources/
    ├── djinniSearchSummary.ts            # NEW — pure URL → human-readable summary for
    │                                       #   basic-search URLs; collapses consecutive
    │                                       #   exp_level sets to a range, lists non-
    │                                       #   consecutive sets discretely, omits absent
    │                                       #   filters cleanly
    ├── djinniSearchSummary.test.ts        # NEW — vitest unit tests for the collapse
    │                                       #   range-vs-list rule and the omit-absent rule
    ├── SourcesPage.tsx                   # MODIFIED — `SubscriptionRow` uses the helper
    │                                       #   for any Djinni subscription whose URL is a
    │                                       #   basic-search shape; keeps existing label for
    │                                       #   the dashboard `subs/{id}/` shape
    └── SourcesPage.test.tsx             # MODIFIED (if exists) — add a row snapshot for
                                          #   a basic-search subscription proving the range
                                          #   label renders
```

**Structure Decision**: A minimal two-app change following the existing adapter +
subscription-validation + Sources-page precedent. The only new file on the Go side is a
small, pure-function `djinni_searchmode.go` for URL-shape discrimination and filter
extraction; the existing `djinni.go` is reorganized (not rewritten) to call into it. The
only new file on the TS side is a pure-URL-parsing helper plus its tests. No migration, no
shared package, no API endpoint, no new source key, no new `compose_job_sources` change.

## Complexity Tracking

*(No constitution violations — table not needed.)*