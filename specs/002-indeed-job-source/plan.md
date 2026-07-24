# Implementation Plan: Indeed Job Source

**Branch**: `002-indeed-job-source` | **Date**: 2026-07-24 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-indeed-job-source/spec.md`

## Summary

Add Indeed as a fifth scrape-kind job source, structurally identical to the existing
`DouAdapter`/`DjinniAdapter` pair: a new `adapters.IndeedAdapter` implementing
`jobsources.Adapter` (`Key()`, `Kind()`, `Search()`, `HealthCheck()`), driven exclusively by
an operator-pasted Indeed search URL through the existing subscription flow (no keyword
search path). `Search` parses the pasted URL's listing page via `scraping.Service.FetchHTML`
+ goquery, extracts job cards, and paginates by incrementing Indeed's `start=` query
parameter up to a bounded page count, honoring the 500ms request pacing already used by DOU.
A `FetchDetail` method (same shape as `DjinniDetailPatch`/`DouDetailPatch`) fills in full
description, remote flag, and posted date for enrichment. The adapter is registered in
`compose.go`'s adapter list, wired into `enrichment.Handler` alongside djinni/dou/workua, and
added to the two source-key allowlists that currently special-case `"djinni"`/`"dou"`
(ingestion's post-insert enrich trigger, and the enrichment dispatch switch). Dashboard's
`SUBSCRIPTION_SOURCES` gets an `indeed` entry. No new tables: reuses `JobSource`,
`Subscription`, `Job`, `SourceRun` exactly as DOU does.

## Technical Context

**Language/Version**: Go 1.22+ (apps/api), matches existing adapters package

**Primary Dependencies**: `github.com/PuerkitoBio/goquery` (HTML parsing, already a
dependency), `internal/scraping.Service` (HTTP fetch, already used by every scrape adapter)

**Storage**: PostgreSQL — no schema changes; reuses `JobSource`, `Subscription`, `Job`,
`SourceRun` tables (migrations 00001, 00002)

**Testing**: `go test ./apps/api/...` — fixture-based unit tests using recorded HTML
(`testdata/*.html`), following the `workua_test.go` / `dou_test.go` pattern; no live network
calls in unit tests

**Target Platform**: Linux server (existing `apps/api` Go service, Docker Compose)

**Project Type**: Web application (existing `apps/api` backend + `apps/dashboard` frontend
monorepo) — this feature touches backend primarily, with a one-entry dashboard change

**Performance Goals**: One Indeed run completes within the existing ingestion cycle window;
request pacing capped at ≥500ms between requests (FR-010); no measurable (>10%) regression
to other sources' cycle time (SC-007)

**Constraints**: No Indeed login/session/cookie storage (public listing pages only, unlike
Djinni's authenticated subs); bounded pagination (mirrors `douMaxSubscriptionPages` /
`djinniMaxSubscriptionPages` = 50); best-effort scraping — Indeed markup/anti-bot behavior
can change or block without notice, and such failures must not fail other sources' runs
(constitution: Local-First / discovery-only sources are best-effort)

**Scale/Scope**: One new adapter file + one test file + one testdata fixture set; edits to
`compose.go` (registration), `enrichment/handler.go` (dispatch + constructor param),
`ingestion/handler.go` (enrich-eligibility allowlist), `apps/dashboard/.../SourcesPage.tsx`
(subscription source list); no DB migration

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — PASS. Adapter only discovers/reads listings (`Search`,
  `FetchDetail`, `HealthCheck`); no submission, no messaging, no action taken on the user's
  behalf (FR-013). No code path added that writes back to Indeed.
- **II. Grounded Generation** — PASS (not applicable to adapter itself). Indeed listings
  flow through the same normalization → matching → generation pipeline as every other
  source (FR-012); no new generation logic introduced here, so no new hallucination
  surface.
- **III. Typed Contracts Across Service Boundaries** — PASS. Adapter emits
  `dto.NormalizedJob` (existing shared DTO), same as DOU/Djinni; no new hand-rolled cross-
  language type. No changes to `packages/shared`, sqlc, or tygo-generated types required —
  Indeed jobs are rows in the existing `Job` table shape.
- **IV. Test Discipline Per Language, Enforced at the Boundary** — PASS. New adapter gets
  `go test` coverage with recorded fixtures (matching `workua_test.go`/`dou_test.go`
  convention); this feature does not touch the dashboard or jobspy-sidecar test suites
  beyond the one-line `SUBSCRIPTION_SOURCES` addition (no new dashboard test required
  beyond existing `sources.spec.ts` coverage, which is not disturbed).
- **V. Local-First, Self-Hosted by Default** — PASS. Indeed is explicitly named in the
  constitution as an external discovery-only source (alongside Adzuna, LinkedIn/Indeed via
  JobSpy); this feature keeps it discovery-only and does not introduce a paid third-party
  API call for core matching/generation. Direct-scrape approach adds no new external paid
  dependency.

No violations. Complexity Tracking not required.

## Project Structure

### Documentation (this feature)

```text
specs/002-indeed-job-source/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md         # Phase 1 output
├── quickstart.md         # Phase 1 output
├── contracts/            # Phase 1 output
└── tasks.md              # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
apps/api/internal/jobsources/
├── adapter.go                       # Adapter interface, Registry — unchanged
├── adapters/
│   ├── dou.go                       # reference pattern for pagination + FetchDetail
│   ├── djinni.go                    # reference pattern for subscription URL parsing
│   ├── indeed.go                    # NEW: IndeedAdapter (Search, FetchDetail, HealthCheck)
│   ├── indeed_test.go                # NEW: fixture-based unit tests
│   └── testdata/
│       ├── indeed_list.html          # NEW: recorded search-results fixture
│       ├── indeed_detail.html        # NEW: recorded job-detail fixture
│       └── indeed_empty.html         # NEW: recorded zero-results fixture

apps/api/internal/enrichment/
└── handler.go                       # EDIT: add `indeed adapters.IndeedAdapter` field,
                                       #       constructor param, dispatch case, enrichIndeed()

apps/api/internal/ingestion/
└── handler.go                       # EDIT: add "indeed" to the enrich-eligible source-key
                                       #       check (persistIfNew)

apps/api/cmd/server/
└── compose.go                        # EDIT: construct IndeedAdapter, add to registry list,
                                       #       thread into enrichment.NewHandler

apps/dashboard/src/features/sources/
└── SourcesPage.tsx                   # EDIT: add { key: 'indeed', label: 'Indeed', ... } to
                                       #       SUBSCRIPTION_SOURCES
```

**Structure Decision**: This is an addition inside the existing monorepo's Go backend
(`apps/api`), following the established one-file-per-source adapter pattern under
`internal/jobsources/adapters/`. No new top-level module, package, or app is introduced —
Indeed slots into the same registry, enrichment dispatch, and subscription flow already
built for DOU/Djinni/WorkUa. The one dashboard edit is additive (a new option in an existing
source-picker list), not a new page or component.

## Complexity Tracking

*No constitution violations — table not needed.*
