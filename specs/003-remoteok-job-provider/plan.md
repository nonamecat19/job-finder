# Implementation Plan: RemoteOK Job Source

**Branch**: `003-remoteok-job-provider` | **Date**: 2026-07-24 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-remoteok-job-provider/spec.md`

## Summary

Add RemoteOK as a sixth job source, structurally parallel to the existing
`DouAdapter`/`DjinniAdapter`/`IndeedAdapter` trio but backed by RemoteOK's public JSON API
(`https://remoteok.com/api`) instead of HTML scraping. A new `adapters.RemoteOKAdapter`
implements `jobsources.Adapter` (`Key()`, `Kind()`, `Search()`, `HealthCheck()`) plus a
`FetchDetail` method matching the `IndeedDetailPatch`/`DouDetailPatch` shape used for
enrichment. `Search` is driven exclusively by an operator-saved subscription — a RemoteOK
tag/category or a `remoteok.com/remote-<tag>-jobs` listing URL, following Indeed's
subscription-only stance — resolves it to a tag, fetches the JSON API (single request, no
pagination since RemoteOK returns its full recent-listing set per call), and maps each
object to `dto.NormalizedJob` with `Remote` always `true`. Because RemoteOK returns full
description/tags/salary in the same JSON payload as the listing, `FetchDetail` is a thin
re-fetch-and-locate-by-id rather than a second HTML parse. `Kind()` reports
`dto.SourceKindAPI` (not `Scrape`) since there is no HTML to parse. The adapter is
registered in `compose.go`'s adapter list, wired into `enrichment.Handler` alongside
djinni/dou/indeed, and added to the ingestion handler's enrich-eligible source-key
allowlist. Dashboard's `SUBSCRIPTION_SOURCES` gets a `remoteok` entry, and
`subscriptions.Service.validateSubscriptionURL` gains a RemoteOK host/shape check mirroring
the existing Indeed check. No new tables: reuses `JobSource`, `Subscription`, `Job`,
`SourceRun` exactly as the other scrape/API sources do.

## Technical Context

**Language/Version**: Go 1.22+ (apps/api), matches existing adapters package

**Primary Dependencies**: standard library `encoding/json` + `net/http` via
`internal/scraping.Service.FetchHTML` (already used by every adapter for GET requests; it
returns the raw response body regardless of content type, so it's reused here for JSON
too — no new HTTP client)

**Storage**: PostgreSQL — no schema changes; reuses `JobSource`, `Subscription`, `Job`,
`SourceRun` tables

**Testing**: `go test ./apps/api/...` — fixture-based unit tests using a recorded JSON
response (`testdata/remoteok_api.json`, `testdata/remoteok_empty.json`), following the
`indeed_test.go` pattern; no live network calls in unit tests

**Target Platform**: Linux server (existing `apps/api` Go service, Docker Compose)

**Project Type**: Web application (existing `apps/api` backend + `apps/dashboard` frontend
monorepo) — this feature touches backend primarily, with a one-entry dashboard change

**Performance Goals**: One RemoteOK run completes within the existing ingestion cycle
window; a single run issues at most a small, bounded number of requests (typically one) so
request pacing (FR-010) is trivially satisfied; no measurable (>10%) regression to other
sources' cycle time (SC-007)

**Constraints**: No RemoteOK login/session/cookie storage (public API only); requests MUST
carry a descriptive client identifier per RemoteOK's documented API etiquette (FR-017); no
pagination construct exists in the RemoteOK API — a single fetch returns RemoteOK's current
recent-listing set, so "bounded requests per run" (FR-010) is satisfied by not looping, not
by a page cap; best-effort — RemoteOK's response shape can change or the endpoint can be
rate-limited without notice, and such failures must not fail other sources' runs
(constitution: Local-First / discovery-only sources are best-effort)

**Scale/Scope**: One new adapter file + one test file + two testdata JSON fixtures; edits to
`compose.go` (registration), `enrichment/handler.go` (dispatch + constructor param),
`ingestion/handler.go` (enrich-eligibility allowlist), `subscriptions/service.go`
(URL validation), `apps/dashboard/.../SourcesPage.tsx` (subscription source list); no DB
migration

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — PASS. Adapter only discovers/reads listings (`Search`,
  `FetchDetail`, `HealthCheck`); no submission, no messaging, no action taken on the user's
  behalf (FR-013). No code path added that writes back to RemoteOK.
- **II. Grounded Generation** — PASS (not applicable to adapter itself). RemoteOK listings
  flow through the same normalization → matching → generation pipeline as every other
  source (FR-012); no new generation logic introduced here, so no new hallucination
  surface.
- **III. Typed Contracts Across Service Boundaries** — PASS. Adapter emits
  `dto.NormalizedJob` (existing shared DTO), same as DOU/Djinni/Indeed; no new hand-rolled
  cross-language type. No changes to `packages/shared`, sqlc, or tygo-generated types
  required — RemoteOK jobs are rows in the existing `Job` table shape.
- **IV. Test Discipline Per Language, Enforced at the Boundary** — PASS. New adapter gets
  `go test` coverage with a recorded JSON fixture (matching `indeed_test.go` convention);
  this feature does not touch the dashboard or jobspy-sidecar test suites beyond the
  one-line `SUBSCRIPTION_SOURCES` addition.
- **V. Local-First, Self-Hosted by Default** — PASS. RemoteOK is an external
  discovery-only source, same category as Indeed/DOU/Djinni; this feature keeps it
  discovery-only and does not introduce a paid third-party API call for core
  matching/generation. RemoteOK's API is free and unauthenticated, adding no new paid
  dependency.

No violations. Complexity Tracking not required.

## Project Structure

### Documentation (this feature)

```text
specs/003-remoteok-job-provider/
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
│   ├── indeed.go                     # reference pattern for subscription-only Search
│   ├── remoteok.go                   # NEW: RemoteOKAdapter (Search, FetchDetail, HealthCheck)
│   ├── remoteok_test.go              # NEW: fixture-based unit tests
│   └── testdata/
│       ├── remoteok_api.json         # NEW: recorded API-response fixture
│       └── remoteok_empty.json       # NEW: recorded zero-results fixture

apps/api/internal/enrichment/
└── handler.go                       # EDIT: add `remoteok adapters.RemoteOKAdapter` field,
                                       #       constructor param, dispatch case,
                                       #       enrichRemoteOK()

apps/api/internal/ingestion/
└── handler.go                       # EDIT: add "remoteok" to the enrich-eligible
                                       #       source-key check (persistIfNew)

apps/api/internal/subscriptions/
└── service.go                        # EDIT: add a RemoteOK case to
                                       #       validateSubscriptionURL (host check)

apps/api/cmd/server/
└── compose.go                        # EDIT: construct RemoteOKAdapter, add to registry
                                       #       list, thread into enrichment.NewHandler

apps/dashboard/src/features/sources/
└── SourcesPage.tsx                   # EDIT: add { key: 'remoteok', label: 'RemoteOK', ... }
                                       #       to SUBSCRIPTION_SOURCES
```

**Structure Decision**: This is an addition inside the existing monorepo's Go backend
(`apps/api`), following the established one-file-per-source adapter pattern under
`internal/jobsources/adapters/`. No new top-level module, package, or app is introduced —
RemoteOK slots into the same registry, enrichment dispatch, and subscription flow already
built for DOU/Djinni/Indeed. The one dashboard edit is additive (a new option in an
existing source-picker list), not a new page or component.

## Complexity Tracking

*No constitution violations — table not needed.*
