# Implementation Plan: Wellfound Job Source

**Branch**: `010-wellfound-job-provider` | **Date**: 2026-07-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/010-wellfound-job-provider/spec.md`

## Summary

Add Wellfound as a first-class job source, following the **Glassdoor** pattern exactly:
Wellfound's listing pages are publicly viewable (per spec Assumptions — no account required
for the fields this feature needs), so a dedicated `adapters.WellfoundAdapter` implements
`jobsources.Adapter` (`Key()`, `Kind()`, `Search()`, `HealthCheck()`) plus a `FetchDetail`
method for enrichment, using plain unauthenticated HTTP scrape via the existing
`scraping.Service` — no login/session, unlike Djinni/JobLeads. `Search` is driven exclusively
by an operator-pasted Wellfound search-results URL (subscription-only, matching
Indeed/RemoteOK/Glassdoor's stance), paced at no faster than one request per 500ms and capped
at a fixed page count (FR-010, mirroring `glassdoorMaxSubscriptionPages`). The adapter is
registered in `compose.go`'s adapter list, wired into `enrichment.Handler` alongside the other
sources, added to `ingestion/handler.go`'s enrich-eligible source-key allowlist, and given a
`validateWellfoundSubscriptionURL` case in `subscriptions/service.go` (host check for
`wellfound.com`/`angel.co`, rejecting non-search-results URL shapes). Dashboard's
`SourcesPage.tsx` gets a `wellfound` entry in the subscription-form source picker. No new
tables and no new migration: reuses `JobSource`, `Subscription`, `Job`, `SourceRun` exactly as
Glassdoor does, with equity/salary text captured in `SalaryRaw`/`Raw` (no new typed columns)
and a distinguishable "blocked" vs "no results" vs "unparseable" failure vocabulary in run
outcomes (FR-011), since — per Glassdoor's precedent — a public job board with anti-bot
posture may block plain HTTP some or all of the time; that is treated as an expected,
reported outcome, not a defect to route around with headless-browser evasion (constitution:
best-effort external sources).

## Technical Context

**Language/Version**: Go 1.x (`apps/api`), matching existing adapters

**Primary Dependencies**: `goquery` (HTML parsing), the existing `internal/scraping.Service`
(plain HTTP fetch, no cookies/session), `internal/jobsources` (Adapter interface + registry)

**Storage**: Postgres — no schema change; `job_sources` row for `wellfound` is upserted at
startup the same way every other adapter's row is (no migration required, same as
Glassdoor/RemoteOK)

**Testing**: `go test ./...` (`apps/api`), table-driven adapter tests against fixture HTML
under `internal/jobsources/adapters/testdata/` (`wellfound_list.html`,
`wellfound_list_page2.html`, `wellfound_empty.html`, `wellfound_blocked.html`,
`wellfound_detail.html`), matching `glassdoor_test.go`/`indeed_test.go`; no live network calls
or real Wellfound requests in unit tests

**Target Platform**: Linux server (existing `apps/api` container)

**Project Type**: Web application (existing `apps/api` backend + `apps/dashboard` frontend
monorepo) — this feature touches backend primarily, with a one-entry dashboard change

**Performance Goals**: N/A beyond existing FR-010 pacing (no faster than one request per
500ms) and a bounded page cap per run; no measurable (>10%) regression to other sources'
cycle time (SC-007)

**Constraints**: No authentication, no anti-bot evasion (FR-013, Assumptions); requests MUST
carry a descriptive client identifier (FR-017); MUST distinguish "blocked"/"no results"/
"unparseable" in run/health outcomes (FR-011); listings whose company has no public profile
page are still ingested using whatever name/location text is on the listing itself; listings
with equity-only, salary-only, or neither are ingested with the missing field(s) left empty
rather than dropped; best-effort — Wellfound's markup can change without notice, and such
failures must not fail other sources' runs (constitution: Local-First / discovery-only
sources are best-effort)

**Scale/Scope**: One new adapter file (`wellfound.go`) + one test file + five testdata HTML
fixtures; edits to `compose.go` (registration), `enrichment/handler.go` (dispatch +
constructor param), `ingestion/handler.go` (enrich-eligibility allowlist),
`subscriptions/service.go` (URL validation), `seed/subscriptions.go` +
`seed/testdata.go` + `seed/sourceruns.go` (seed data, matching existing sources'
convention), `apps/dashboard/.../SourcesPage.tsx` (subscription source list); no DB
migration

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — PASS. Adapter is discovery-only (`Search`/`HealthCheck`/
  `FetchDetail`); it never submits, messages, or acts on a listing on the user's behalf
  (FR-013).
- **II. Grounded Generation** — PASS (not applicable to the adapter itself). Wellfound
  listings flow through the same normalization → matching → generation pipeline as every
  other source (FR-012); no new generation logic is introduced here, so no new hallucination
  surface.
- **III. Typed Contracts Across Service Boundaries** — PASS. `WellfoundAdapter` emits
  `dto.NormalizedJob` (the existing shared DTO), same as DOU/Djinni/Indeed/RemoteOK/Glassdoor/
  JobLeads; no new hand-rolled cross-language type. No changes to `packages/shared`, sqlc, or
  tygo-generated types required.
- **IV. Test Discipline Per Language, Enforced at the Boundary** — PASS. New adapter gets
  `go test` coverage with recorded HTML fixtures (matching `glassdoor_test.go` convention);
  this feature does not touch dashboard test suites beyond the one-line `SourcesPage.tsx`
  source-list addition, so no new frontend test obligations beyond existing coverage passing.
- **V. Local-First, Self-Hosted by Default** — PASS. Wellfound is an external discovery-only
  source, same category as Djinni/Indeed/DOU/RemoteOK/Glassdoor/JobLeads; this feature keeps
  it discovery-only and introduces no paid third-party *AI* API call for core matching/
  generation — matching/scoring/generation remain local-Ollama, unchanged.

No violations. Complexity Tracking not required.

## Project Structure

### Documentation (this feature)

```text
specs/010-wellfound-job-provider/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit-tasks — not created here)
```

(No `contracts/` — this feature adds no new external-facing API; it implements the existing
internal `jobsources.Adapter` interface, which is already the contract `data-model.md`
documents, matching Glassdoor's precedent.)

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── jobsources/
│   │   ├── adapters/
│   │   │   ├── wellfound.go             # NEW — Search, HealthCheck, FetchDetail
│   │   │   ├── wellfound_test.go        # NEW
│   │   │   └── testdata/
│   │   │       ├── wellfound_list.html         # NEW fixture
│   │   │       ├── wellfound_list_page2.html   # NEW fixture
│   │   │       ├── wellfound_empty.html        # NEW fixture
│   │   │       ├── wellfound_blocked.html      # NEW fixture (bot-challenge/rate-limit page)
│   │   │       └── wellfound_detail.html       # NEW fixture
│   │   └── ... (Adapter interface, registry — unchanged)
│   ├── subscriptions/
│   │   └── service.go                   # EDIT — add validateWellfoundSubscriptionURL
│   ├── ingestion/
│   │   └── handler.go                   # EDIT — add "wellfound" to the enrich-eligible
│   │                                     #        source-key check (persistIfNew)
│   ├── enrichment/
│   │   └── handler.go                   # EDIT — add `wellfound adapters.WellfoundAdapter`
│   │                                     #        field, constructor param, dispatch case,
│   │                                     #        enrichWellfound()
│   └── seed/
│       ├── subscriptions.go             # EDIT — seed a wellfound subscription
│       ├── testdata.go                  # EDIT — seed one wellfound sample job
│       └── sourceruns.go                # EDIT — seed a wellfound source-run record
└── cmd/server/
    └── compose.go                       # EDIT — construct + register WellfoundAdapter,
                                          #        thread into enrichment.NewHandler

apps/dashboard/
└── src/features/sources/
    └── SourcesPage.tsx                  # EDIT — add { key: 'wellfound', label: 'Wellfound',
                                          #        placeholder: ... } to the subscription-form
                                          #        source dropdown, same one-line addition as
                                          #        the existing Glassdoor/JobLeads entries
```

**Structure Decision**: Primarily a single-project change inside the existing `apps/api` Go
monolith, following the Glassdoor/RemoteOK/Indeed adapter precedent exactly
(`internal/jobsources/adapters/*.go` + registry wiring) rather than the login-gated Djinni/
JobLeads pattern, since Wellfound's listing pages are publicly viewable per the spec's
Assumptions. `apps/dashboard` needs only the matching one-line addition to
`SourcesPage.tsx`'s source dropdown (confirmed by inspecting the file, which already lists
DOU/Djinni/Indeed/RemoteOK/Glassdoor explicitly in that array). No changes to
`apps/jobspy-sidecar` or `packages/shared` — no new cross-language types.

## Complexity Tracking

*(No violations — table not needed.)*
