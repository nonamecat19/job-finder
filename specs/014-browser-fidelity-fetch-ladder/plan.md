# Implementation Plan: Browser-Fidelity Retrieval and Escalation Ladder

**Branch**: `014-browser-fidelity-fetch-ladder` | **Date**: 2026-07-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/014-browser-fidelity-fetch-ladder/spec.md`

## Summary

Scraped job-board adapters fail silently because outbound requests look like a bot (stale UA,
partial headers, no TLS fingerprint match, no persisted visitor state) and because a challenge
page is currently indistinguishable from "zero listings" to the caller. This plan replaces the
single fixed-UA `scraping.Service.FetchHTML`/`HTTPClient` path with a shared **retrieval
service** that (1) issues requests with a consistent, current, real-browser fingerprint
(headers, order, and TLS/HTTP2 characteristics) and persists per-host cookies/visitor state
across runs, (2) climbs an ordered, per-host-remembered escalation ladder — direct
browser-fidelity request → chromedp-driven real browser (isolated from the existing
resume-PDF browser context) → FlareSolverr (already deployed under the `scraping-extras`
compose profile, currently unwired) — only on a detected challenge, and (3) returns a typed
per-page `Outcome` (read / challenged / refused / unparseable / deferred) and a per-run
`Verdict` (success / partial / blocked) instead of a bare `([]NormalizedJob, error)`, so a
fully- or partially-blocked run can never be reported as an empty success. Per-host state
(rung preference, cookies, consecutive-block count, cooling-off expiry, budget usage) persists
on a new `HostRetrievalState` table keyed by host, extending — not replacing — the existing
`ratelimit.Transport` per-host pacing. The dashboard Sources screen gains a per-host retrieval
status view and controls to clear rung preference / stored cookies.

## Technical Context

**Language/Version**: Go 1.26 (`apps/api`), TypeScript/React 18 + Vite (`apps/dashboard`)

**Primary Dependencies**: `github.com/chromedp/chromedp` v0.16.0 (already a direct dependency,
today used only to render the user's own resume/cover-letter PDFs — this feature adds a second,
isolated chromedp allocator for third-party pages); `golang.org/x/time/rate` (already used by
`ratelimit.Transport`); FlareSolverr, already declared as service `flaresolverr` in
`docker-compose.yml`/`docker-compose.prod.yml` under the opt-in `scraping-extras` profile, and
already referenced by an unused `FLARESOLVERR_URL` config key
(`apps/api/internal/config/config.go:113`). A Go TLS-fingerprinting HTTP client
(`github.com/bogdanfinn/tls-client` or `github.com/refraction-networking/utls`) is a new
dependency needed to satisfy FR-003 (connection-level characteristics) — resolved in
[research.md](./research.md).

**Storage**: PostgreSQL via existing goose migrations (`apps/api/internal/db/migrations/`) and
sqlc-generated typed access. New `HostRetrievalState` table (per-host rung, cookies, block
count, cooling-off, budget window); `JobSource.config` JSONB is NOT reused for this state since
it's per-host, not per-source, and multiple sources can share a host (FR-031, edge case:
"one source spans several hosts").

**Testing**: `go test` for `apps/api` (existing suite convention), including `httptest.Server`
fixtures that serve challenge-shaped and refusal-shaped responses to exercise escalation and
honest-reporting without hitting real hosts (mirrors existing adapter test patterns);
`vitest` for the dashboard Sources-screen additions; `make test-integration` for the
Postgres-backed `HostRetrievalState` repository and for a FlareSolverr-profile-enabled
Docker Compose escalation path.

**Target Platform**: Existing `apps/api` Go service and `apps/dashboard` React app, both
already shipped via Docker Compose; no new deployable unit beyond the already-declared
`flaresolverr` compose service.

**Project Type**: Web application (existing monorepo: `apps/api` backend + `apps/dashboard`
frontend + `packages/shared` types) — Option 2 structure, extending existing modules rather
than adding new top-level projects.

**Performance Goals**: None speed-related — the spec explicitly deprioritizes throughput
(Assumptions: "slowness is acceptable"). The only quantified goals are correctness/honesty
ones: SC-001 (≥90% success rate over 2 weeks for two currently-dead sources), SC-006 (repeat
runs skip ladder re-walk).

**Constraints**: Free-only — no paid proxy/scraping/anonymizing services (FR-032); per-host
daily request budget and pacing enforced across concurrent sources (FR-030, FR-031); browser
identity must be a single configured value shared by every retrieval path (FR-004); real-browser
rendering of third-party pages MUST be process/context-isolated from the resume/cover-letter
render path (FR-019, SC-012).

**Scale/Scope**: ~15 existing scraped adapters (`apps/api/internal/jobsources/adapters/`),
low single-digit request/sec ceiling per host, dozens of distinct hosts.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — N/A. This feature only changes retrieval/reporting for job
  discovery; it adds no application-submission code path. PASS.
- **II. Grounded Generation** — N/A. No LLM-generated content involved. PASS.
- **III. Typed Contracts Across Service Boundaries** — New DTOs (per-host retrieval status,
  `Outcome`, `Verdict`) that the dashboard consumes MUST be added to `packages/shared` and/or
  generated via tygo from the Go types backing the Sources-screen API additions, matching the
  existing `JobSourceDto`/`dto` package pattern. No hand-duplicated TS types. PASS (must be
  honored in Phase 1 contracts).
- **IV. Test Discipline Per Language, Enforced at the Boundary** — Escalation and honest-reporting
  logic is exactly the kind of cross-service behavior the constitution requires Docker-backed
  integration coverage for (real Postgres for `HostRetrievalState`, and the FlareSolverr
  container for the top rung) rather than mocks. `make test-lint` required since this change
  touches `apps/api` and `apps/dashboard`. PASS (must be honored in task planning).
- **V. Local-First, Self-Hosted by Default** — FlareSolverr and chromedp are both self-hosted;
  FR-032 explicitly forbids third-party paid APIs or anonymizing relays, which is a stricter
  reading of the same principle applied to job discovery rather than core inference (the
  constitution scopes external sources to discovery-only already). PASS.

No violations. Complexity Tracking not required.

## Project Structure

### Documentation (this feature)

```text
specs/014-browser-fidelity-fetch-ladder/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output
├── data-model.md         # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/            # Phase 1 output
└── tasks.md              # Phase 2 output (/speckit.tasks — not created here)
```

### Source Code (repository root)

```text
apps/api/internal/
├── retrieval/                   # NEW package: the escalation ladder + browser identity
│   ├── identity.go              # single configured BrowserIdentity (headers, order, UA, TLS profile)
│   ├── ladder.go                # Rung interface + ordered ladder: direct → chromedp → flaresolverr
│   ├── direct.go                # rung 1: TLS-fingerprinted, header-faithful HTTP client
│   ├── browser.go                # rung 2: isolated chromedp allocator for third-party pages
│   ├── flaresolverr.go           # rung 3: FlareSolverr client (uses FLARESOLVERR_URL)
│   ├── challenge.go              # response-body/shape challenge & refusal detection
│   ├── outcome.go                 # Outcome / Verdict types
│   └── state.go                   # HostRetrievalState repository (rung pref, cookies, budget, cooling-off)
├── ratelimit/                    # EXISTING — extended with daily per-host budget alongside RPS pacing
├── scraping/                     # EXISTING — FetchHTML/HTTPClient callers migrate to retrieval.Service;
│                                  # chromedp resume/cover-letter path stays untouched and isolated
├── jobsources/                    # EXISTING — Adapter.Search return type gains Outcome/Verdict plumbing;
│   └── adapters/                  # adapters stop doing their own challenge-string-matching (wellfound.go,
│                                  # glassdoor.go, jobgether.go today) and report via shared retrieval.Outcome
├── db/migrations/                 # EXISTING goose dir — new migration for host_retrieval_state table
└── httpapi/                       # EXISTING — Sources-screen endpoints extended with per-host status,
                                   # clear-rung-preference, clear-cookies actions

packages/shared/src/                # EXISTING — new DTOs: HostRetrievalStatus, PageOutcome, RunVerdict

apps/dashboard/src/features/sources/
├── SourcesPage.tsx                 # EXISTING — extended with per-host retrieval status panel
└── hooks.ts                        # EXISTING — new hooks for host status + clear actions

apps/api/internal/tests/            # existing convention: httptest challenge/refusal fixtures,
                                     # ladder escalation tests, honest-reporting tests
```

**Structure Decision**: Web application (Option 2), extending the existing `apps/api` +
`apps/dashboard` + `packages/shared` layout. The escalation ladder, browser identity, and
per-host state live in a new `apps/api/internal/retrieval` package rather than inside
`scraping` or `jobsources`, because FR-020 requires one shared interface every adapter uses —
folding it into `jobsources` would keep the coupling the spec explicitly asks to remove
(adapters implementing their own retrieval/challenge handling), and `scraping` remains the
narrower "fetch HTML / render our own PDFs" service that `retrieval` sits in front of for rung 1.

## Complexity Tracking

*No Constitution Check violations — section not needed.*
