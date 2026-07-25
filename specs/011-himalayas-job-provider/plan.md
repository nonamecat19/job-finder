# Implementation Plan: Himalayas Job Source

**Branch**: `011-himalayas-job-provider` | **Date**: 2026-07-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/011-himalayas-job-provider/spec.md`

## Summary

Add Himalayas as a new job source, structurally parallel to the existing
`DouAdapter`/`DjinniAdapter`/`IndeedAdapter`/`RemoteOKAdapter`/`GlassdoorAdapter`/`JobLeadsAdapter`
set but following the **RemoteOK** pattern specifically: a public, unauthenticated JSON feed with
no login/session, fetched through `scraping.Service.FetchHTML` with a descriptive `User-Agent`
(FR-017) rather than raw `jobsources.GetJSON`. Live research (`research.md`) confirms Himalayas
exposes an undocumented but functional JSON endpoint, `https://himalayas.app/jobs/api`, returning
paginated (`offset`/`limit`, server-capped at 20/page), newest-first job records that already
include the **full** posting description (not a teaser) plus salary, location/timezone
restrictions, categories, and a stable canonical URL (`guid`). Because that endpoint ignores every
filter query parameter it was tested with, a new `adapters.HimalayasAdapter` implements
`jobsources.Adapter` (`Key()`, `Kind()`, `Search()`, `HealthCheck()`) by paging through the raw feed
(bounded by a fixed page cap and 500ms inter-request pacing, FR-010) and filtering **locally**
against each job's `categories`/`parentCategories`/`timezoneRestrictions` fields — mirroring
`ArbeitnowAdapter`'s "fetch bounded pages, filter client-side" shape rather than RemoteOK's
"pass the filter through as a query param" shape, since Himalayas's filters don't actually reach the
server. `Search` is driven exclusively by an operator-saved subscription: Himalayas's own public
`/jobs?categories=<slug>[&timezones=<a,b>]` search-page URL, following the
Indeed/RemoteOK/Glassdoor/JobLeads subscription-only stance. Because the feed already returns full
descriptions at ingestion (verified live) and there is no working per-listing detail endpoint to
enrich from, this feature adds **no** `FetchDetail` method and does **not** register Himalayas in
`enrichment.Handler`'s dispatch or `ingestion.Handler`'s enrich-eligible source list — the same
posture as Adzuna/Jooble/Arbeitnow/Remotive/Robota/WorkUa. The adapter is registered in
`compose.go`'s adapter list, and `subscriptions.Service.validateSubscriptionURL` gains a Himalayas
host/shape check. No new tables and no new migration: reuses `JobSource`, `Subscription`, `Job`,
`SourceRun` exactly as RemoteOK does, with no credentials of any kind.

## Technical Context

**Language/Version**: Go 1.22+ (apps/api), matches existing adapters package

**Primary Dependencies**: `internal/scraping.Service.FetchHTML` for the public JSON fetch (same
transport as `RemoteOKAdapter`, including its per-host request pacing — see research.md R2);
`encoding/json` for manual response decoding (no HTML parser needed — this is a JSON feed, unlike
Djinni/DOU/Indeed/Glassdoor); no new third-party dependency

**Storage**: PostgreSQL — no schema changes; reuses `JobSource`, `Subscription`, `Job`, `SourceRun`
tables; `job_source.config` stays empty for Himalayas (no credentials, no session state — same as
RemoteOK/Arbeitnow/Remotive)

**Testing**: `go test ./apps/api/...` — fixture-based unit tests using a recorded JSON response
(`testdata/himalayas_list.json`, `testdata/himalayas_empty.json`), following the
`remoteok_test.go`/`arbeitnow_test.go` pattern (an `httptest.Server` stands in for
`https://himalayas.app/jobs/api` via the adapter's overridable `APIURL` field, mirroring
`RemoteOKAdapter.APIURL`); no live network calls in unit tests (research.md's live probes were a
one-time investigation, not part of the test suite)

**Target Platform**: Linux server (existing `apps/api` Go service, Docker Compose)

**Project Type**: Web application (existing `apps/api` backend + `apps/dashboard` frontend
monorepo) — this feature touches backend primarily, with a one-entry dashboard change

**Performance Goals**: One Himalayas run completes within the existing ingestion cycle window;
request pacing matches Indeed/Glassdoor's fixed inter-page delay (FR-010: no faster than one
request per 500ms) and is bounded by a page cap so a single run cannot sweep unbounded across
Himalayas's ~96k-listing feed hunting for category matches (research.md R7); no measurable (>10%)
regression to other sources' cycle time (SC-007)

**Constraints**: No credentials of any kind — Himalayas is fully public (research.md R1); requests
MUST carry a descriptive client identifier (FR-017, research.md R8); category/timezone filtering
happens client-side against each fetched page because the upstream endpoint ignores filter query
parameters (research.md R3) — a subscription for a narrow/rare category may legitimately yield
fewer than 20 results within the bounded page-sweep, which SC-002 already tolerates; best-effort —
Himalayas's feed shape can change without notice, and such failures must not fail other sources'
runs (constitution: Local-First / discovery-only sources are best-effort)

**Scale/Scope**: One new adapter file (`himalayas.go`) + one test file + two testdata JSON
fixtures; edits to `compose.go` (registration), `subscriptions/service.go` (URL validation),
`apps/dashboard/.../SourcesPage.tsx` (subscription source list); no DB migration, no config.go
changes (no env vars/secrets to register), no enrichment/ingestion enrich-eligibility changes (see
research.md R6)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — PASS. Adapter only reads Himalayas's public feed (`Search`,
  `HealthCheck`); no submission, no messaging, no action taken on the user's behalf (FR-013). No
  write path to Himalayas exists at all (unlike JobLeads/Djinni's login POST).
- **II. Grounded Generation** — PASS (not applicable to adapter itself). Himalayas listings flow
  through the same normalization → matching → generation pipeline as every other source (FR-012);
  no new generation logic introduced here, so no new hallucination surface.
- **III. Typed Contracts Across Service Boundaries** — PASS. Adapter emits `dto.NormalizedJob`
  (existing shared DTO), same as every other source; no new hand-rolled cross-language type. No
  changes to `packages/shared`, sqlc, or tygo-generated types required.
- **IV. Test Discipline Per Language, Enforced at the Boundary** — PASS. New adapter gets `go test`
  coverage with a recorded JSON fixture (matching `remoteok_test.go`/`arbeitnow_test.go`
  convention, via an `httptest.Server`); this feature does not touch the dashboard test suite beyond
  the one-line `SUBSCRIPTION_SOURCES` addition (covered by existing `SourcesPage` tests, if any
  assert on that list's length/contents).
- **V. Local-First, Self-Hosted by Default** — PASS. Himalayas is an external discovery-only
  source, same category as RemoteOK/Arbeitnow/Remotive; this feature keeps it discovery-only and
  introduces no third-party *AI* API dependency. Himalayas's public feed is free/unauthenticated,
  so this feature adds no new operational cost either.

No violations. Complexity Tracking not required.

## Project Structure

### Documentation (this feature)

```text
specs/011-himalayas-job-provider/
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
├── adapter.go                        # Adapter interface, Registry — unchanged
├── adapters/
│   ├── remoteok.go                   # reference pattern: public JSON API via FetchHTML, subscription-URL-only Search
│   ├── arbeitnow.go                  # reference pattern: no working upstream filter → bounded pages + local filtering
│   ├── himalayas.go                  # NEW: HimalayasAdapter (Search, HealthCheck) — no FetchDetail (research.md R6)
│   ├── himalayas_test.go             # NEW: fixture-based unit tests against an httptest.Server
│   └── testdata/
│       ├── himalayas_list.json       # NEW: recorded /jobs/api page fixture
│       └── himalayas_empty.json      # NEW: recorded zero-results-shape fixture

apps/api/internal/subscriptions/
└── service.go                        # EDIT: add a `case "himalayas":` to
                                        #       validateSubscriptionURL (host + categories-param check)

apps/api/cmd/server/
└── compose.go                        # EDIT: construct HimalayasAdapter, add to registry list

apps/dashboard/src/features/sources/
└── SourcesPage.tsx                   # EDIT: add { key: 'himalayas', label: 'Himalayas', ... }
                                        #       to SUBSCRIPTION_SOURCES
```

**Structure Decision**: This is an addition inside the existing monorepo's Go backend
(`apps/api`), following the established one-file-per-source adapter pattern under
`internal/jobsources/adapters/`, using the simpler stateless RemoteOK/Arbeitnow shape (no session
file, no `FetchDetail`, no enrichment-handler wiring — research.md R1/R6) rather than the
login-gated Djinni/JobLeads shape or the detail-enrichment Indeed/Glassdoor/RemoteOK shape. No new
top-level module, package, app, or DB migration is introduced — Himalayas slots into the same
registry and subscription flow already built for the other sources. The one dashboard edit is
additive (a new option in an existing source-picker list), not a new page or component.

## Complexity Tracking

*No constitution violations — table not needed.*
