# Implementation Plan: Jobgether Job Source

**Branch**: `012-jobgether-job-provider` | **Date**: 2026-07-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/012-jobgether-job-provider/spec.md`

## Summary

Add Jobgether as a first-class job source, structurally identical to the Glassdoor source
(`004-glassdoor-job-provider`): a new `adapters.JobgetherAdapter` implementing
`jobsources.Adapter` (`Key()`, `Kind()`, `Search()`, `HealthCheck()`) plus a `FetchDetail`
method for enrichment — plain-HTTP scrape of Jobgether's public search-results and
listing-detail pages via the existing `scraping.Service`, no login, no session, no new schema.
Jobgether is subscription-URL-only (mirrors Indeed/Glassdoor's stance: no ad hoc keyword
search). The one Jobgether-specific nuance is its own AI match-percentage score, present on
some listings: it is captured into `dto.NormalizedJob.Raw` as descriptive metadata only and
never substituted for or blended into this product's own matching/scoring (FR-012, edge case).
The adapter is registered in `compose.go`'s adapter list, wired into `enrichment.Handler`
alongside the other scrape sources, added to the ingestion handler's enrich-eligible
source-key allowlist, and given a `validateJobgetherSubscriptionURL` host check in
`subscriptions/service.go`. Dashboard's `SUBSCRIPTION_SOURCES` list in `SourcesPage.tsx` gets
a `jobgether` entry. No new tables, no new migration, no new cross-language types: reuses
`JobSource`, `Subscription`, `Job`, `SourceRun` exactly as Glassdoor does.

## Technical Context

**Language/Version**: Go 1.x (`apps/api`), matching existing adapters

**Primary Dependencies**: `goquery` (HTML parsing — Jobgether is server-rendered, like
Glassdoor/Indeed/DOU; no public JSON API is known to exist), the existing
`internal/scraping.Service` (plain HTTP fetch, no auth), `internal/jobsources` (Adapter
interface + registry)

**Storage**: PostgreSQL — no schema change; a `job_sources` row for `jobgether` is upserted at
startup the same way every other adapter's row is (no migration required, same as
Glassdoor/RemoteOK); Jobgether's match-percentage score, when present, is stashed in the
existing `jobs.raw` JSON column, not a new column (FR-012 edge case)

**Testing**: `go test ./apps/api/...`, table-driven adapter tests against fixture HTML under
`internal/jobsources/adapters/testdata/`, matching `glassdoor_test.go`/`indeed_test.go`

**Target Platform**: Linux server (existing `apps/api` container)

**Project Type**: Web service (Go API + React dashboard), single new adapter within the
existing `apps/api` monolith — no new project/service

**Performance Goals**: N/A beyond existing FR-010 pacing (no faster than one request per
500ms per run) and a bounded per-run request/page cap (FR-010); no measurable (>10%)
regression to other sources' ingestion cycle time (SC-007)

**Constraints**: Conservative request pacing and a bounded page cap per run (FR-010); no
anti-bot evasion, no authentication (Assumptions: publicly viewable without an account); must
distinguish "blocked/throttled" from "no matching listings" from "content could not be
interpreted" in run/health outcomes (FR-011, edge cases); descriptive client identifier on
every request (FR-017); Jobgether's own match-percentage score MUST NOT feed this product's
scoring (FR-012, edge case)

**Scale/Scope**: One new adapter file (`jobgether.go`) + one test file + fixture HTML under
`testdata/`; edits to `compose.go` (registration), `enrichment/handler.go` (dispatch +
constructor param), `ingestion/handler.go` (enrich-eligibility allowlist),
`subscriptions/service.go` (URL validation), seed files (`subscriptions.go`, `testdata.go`,
`sourceruns.go`), and `apps/dashboard/.../SourcesPage.tsx` (subscription source list); no DB
migration

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — PASS. Adapter is discovery-only (`Search`/`HealthCheck`/
  `FetchDetail`); it never submits, messages, or acts on a listing on the user's behalf
  (FR-013).
- **II. Grounded Generation** — PASS (not applicable to the adapter itself). Jobgether
  listings flow through the same normalization → matching → generation pipeline as every
  other source (FR-012); no new generation logic introduced here, so no new hallucination
  surface. Jobgether's own AI match-percentage score is explicitly kept out of this product's
  scoring (FR-012 edge case), so it cannot silently bias grounded generation either.
- **III. Typed Contracts Across Service Boundaries** — PASS. `JobgetherAdapter` emits
  `dto.NormalizedJob` (the existing shared DTO), same as DOU/Djinni/Indeed/RemoteOK/Glassdoor/
  JobLeads; no new hand-rolled cross-language type, no changes to `packages/shared`, sqlc, or
  tygo-generated types required.
- **IV. Test Discipline Per Language, Enforced at the Boundary** — PASS. New adapter gets
  `go test` coverage with recorded HTML fixtures (matching `glassdoor_test.go` convention);
  this feature does not touch the dashboard test suite beyond the one-line
  `SUBSCRIPTION_SOURCES` addition, which is covered by existing `SourcesPage` tests if any
  exercise that list.
- **V. Local-First, Self-Hosted by Default** — PASS. Jobgether is an external discovery-only
  source, same category as Glassdoor/Indeed/DOU/RemoteOK/JobLeads; this feature keeps it
  discovery-only and does not introduce a paid third-party *AI* API call for core
  matching/generation — Jobgether's own AI match score is stored as inert metadata, never
  invoked as a scoring service.

No violations. Complexity Tracking not required.

## Project Structure

### Documentation (this feature)

```text
specs/012-jobgether-job-provider/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md         # Phase 1 output
├── quickstart.md         # Phase 1 output
├── contracts/            # Phase 1 output
└── tasks.md              # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── jobsources/
│   │   ├── adapters/
│   │   │   ├── jobgether.go            # NEW — Search, HealthCheck, FetchDetail
│   │   │   ├── jobgether_test.go       # NEW
│   │   │   └── testdata/
│   │   │       ├── jobgether_list.html         # NEW fixture
│   │   │       ├── jobgether_list_page2.html   # NEW fixture
│   │   │       ├── jobgether_empty.html        # NEW fixture
│   │   │       ├── jobgether_blocked.html      # NEW fixture (rate-limit/interstitial page)
│   │   │       └── jobgether_detail.html       # NEW fixture
│   │   └── ... (Adapter interface, registry — unchanged)
│   ├── subscriptions/
│   │   └── service.go                  # MODIFIED — add validateJobgetherSubscriptionURL
│   ├── ingestion/
│   │   └── handler.go                  # MODIFIED — add "jobgether" to enrich-eligible
│   │                                    #   source-key check (persistIfNew)
│   ├── enrichment/
│   │   └── handler.go                  # MODIFIED — add `jobgether adapters.JobgetherAdapter`
│   │                                    #   field, constructor param, dispatch case,
│   │                                    #   enrichJobgether()
│   └── seed/
│       ├── subscriptions.go            # MODIFIED — seed a jobgether subscription
│       ├── testdata.go                 # MODIFIED — seed one jobgether sample job
│       └── sourceruns.go               # MODIFIED — seed a jobgether source-run record
└── cmd/server/
    └── compose.go                      # MODIFIED — construct + register JobgetherAdapter,
                                         #   thread into enrichment.NewHandler

apps/dashboard/
└── src/features/sources/
    └── SourcesPage.tsx                 # MODIFIED — add { key: 'jobgether', label: 'Jobgether',
                                         #   placeholder: ... } to SUBSCRIPTION_SOURCES
```

**Structure Decision**: Primarily a single-project change inside the existing `apps/api` Go
monolith, following the Glassdoor adapter precedent exactly
(`internal/jobsources/adapters/*.go` + registry wiring) — chosen over the Djinni/JobLeads
login-session pattern because Jobgether's listing and detail pages are publicly viewable
without an account (per spec Assumptions), so no session/credentials layer is needed. The one
dashboard edit is additive (a new option in an existing source-picker list), not a new page or
component. No changes to `apps/jobspy-sidecar` or `packages/shared` — no new cross-language
types.

## Complexity Tracking

*(No violations — table not needed.)*
