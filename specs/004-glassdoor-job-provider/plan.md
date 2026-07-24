# Implementation Plan: Glassdoor Job Source

**Branch**: `004-glassdoor-job-provider` | **Date**: 2026-07-24 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/004-glassdoor-job-provider/spec.md`

## Summary

Add Glassdoor as a first-class job source, mirroring the Indeed and RemoteOK sources exactly:
a dedicated Go adapter (`GlassdoorAdapter`) implementing the `jobsources.Adapter` interface —
plain-HTTP scrape of Glassdoor's public search-results pages via the existing
`scraping.Service`, no login — plus a `FetchDetail` method for enrichment, wired into the
same registry/ingestion/enrichment/subscription-validation/seed touch points every prior
source used. No new pipeline, no DB schema change, no jobspy-sidecar involvement.

## Technical Context

**Language/Version**: Go 1.x (`apps/api`), matching existing adapters

**Primary Dependencies**: `goquery` (HTML parsing), the existing `internal/scraping.Service`
(plain HTTP fetch), `internal/jobsources` (Adapter interface + registry)

**Storage**: Postgres — no schema change; `job_sources` row for `glassdoor` is
upserted at startup the same way every other adapter's row is (no migration required, same
as RemoteOK)

**Testing**: `go test ./...` (`apps/api`), table-driven adapter tests against fixture HTML
under `internal/jobsources/adapters/testdata/`, matching `indeed_test.go`/`remoteok_test.go`

**Target Platform**: Linux server (existing `apps/api` container)

**Project Type**: Web service (Go API + React dashboard), single new adapter within the
existing `apps/api` monolith — no new project/service

**Performance Goals**: N/A beyond existing FR-010 pacing (≥500ms between requests per run)

**Constraints**: Conservative request pacing and a bounded page cap per run (FR-010); no
anti-bot evasion, no authentication (FR-013, Assumptions); must distinguish "blocked" from
"no results" from "unparsable" in run/health outcomes (FR-011, FR-018)

**Scale/Scope**: One new adapter file + wiring in ~6 existing files; no new services, no new
tables

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — PASS. Adapter is discovery-only (`Search`/`HealthCheck`/
  `FetchDetail`); it never submits, messages, or acts on a listing (FR-013).
- **II. Grounded Generation** — PASS. Not applicable — this feature adds raw listing data,
  not generated content; downstream generation already sources only from ingested listing
  fields and the user's profile, unchanged by this feature.
- **III. Typed Contracts Across Service Boundaries** — PASS. `GlassdoorAdapter` produces
  `dto.NormalizedJob` (the existing shared shape) exactly like every other Go-side adapter;
  no new cross-language boundary is introduced (no sidecar, no new TS types needed beyond
  what `packages/shared` already has for job listings).
- **IV. Test Discipline Per Language, Enforced at the Boundary** — PASS. Adapter tests live
  in `go test ./...` per existing convention; this feature touches only `apps/api`, so
  `make test-lint`'s cross-app requirement doesn't add new obligations beyond the Go suite
  passing.
- **V. Local-First, Self-Hosted by Default** — PASS. Glassdoor is external *discovery* only,
  explicitly named in the constitution's Technology & Architecture Constraints as an
  external source category; no third-party paid AI API involved; matching/scoring/generation
  remain local-Ollama, unchanged.

No violations. Complexity Tracking not needed.

## Project Structure

### Documentation (this feature)

```text
specs/004-glassdoor-job-provider/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md         # Phase 1 output
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks — not created here)
```

(No `contracts/` — this feature adds no new external-facing API; it implements the existing
internal `jobsources.Adapter` interface, which is already the contract `data-model.md`
documents.)

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── jobsources/
│   │   ├── adapters/
│   │   │   ├── glassdoor.go            # NEW — Search, HealthCheck, FetchDetail
│   │   │   ├── glassdoor_test.go       # NEW
│   │   │   └── testdata/
│   │   │       ├── glassdoor_list.html         # NEW fixture
│   │   │       ├── glassdoor_list_page2.html   # NEW fixture
│   │   │       ├── glassdoor_empty.html        # NEW fixture
│   │   │       ├── glassdoor_blocked.html      # NEW fixture (bot-challenge page)
│   │   │       └── glassdoor_detail.html       # NEW fixture
│   │   └── ... (Adapter interface, registry — unchanged)
│   ├── subscriptions/
│   │   └── service.go                  # MODIFIED — add validateGlassdoorSubscriptionURL
│   ├── ingestion/
│   │   └── handler.go                  # MODIFIED — no change expected (registry-driven);
│   │                                    #   verify during implementation
│   ├── enrichment/
│   │   └── handler.go                  # MODIFIED — add enrichGlassdoor + switch case
│   └── seed/
│       ├── subscriptions.go            # MODIFIED — seed a glassdoor subscription
│       └── testdata.go                 # MODIFIED — seed one glassdoor sample job
└── cmd/server/
    └── compose.go                      # MODIFIED — construct + register GlassdoorAdapter

apps/dashboard/
└── src/features/sources/
    └── SourcesPage.tsx                 # MODIFIED — add Glassdoor to the subscription-form
                                         #   source dropdown (key/label/placeholder), same
                                         #   one-line entry as the existing RemoteOK addition
```

**Structure Decision**: Primarily a single-project change inside the existing `apps/api` Go
monolith, following the RemoteOK/Indeed adapter precedent exactly
(`internal/jobsources/adapters/*.go` + registry wiring). `apps/dashboard` needs the matching
one-line addition to `SourcesPage.tsx`'s source dropdown (`{ key: 'glassdoor', label:
'Glassdoor', placeholder: ... }`) — confirmed by inspecting `SourcesPage.tsx`, which lists
DOU/Djinni/Indeed/RemoteOK explicitly in that array (`002`/`003` added their entries the same
way). No changes to `apps/jobspy-sidecar` or `packages/shared` — no new cross-language types.

## Complexity Tracking

*(No violations — table not needed.)*
