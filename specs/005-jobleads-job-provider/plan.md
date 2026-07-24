# Implementation Plan: JobLeads Job Source

**Branch**: `005-jobleads-job-provider` | **Date**: 2026-07-24 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-jobleads-job-provider/spec.md`

## Summary

Add JobLeads as a new job source, structurally parallel to the existing
`DouAdapter`/`DjinniAdapter`/`IndeedAdapter`/`RemoteOKAdapter`/`GlassdoorAdapter` set but
following the **Djinni** pattern specifically, because JobLeads (like Djinni, unlike
RemoteOK/Indeed/DOU) is login-gated: listings are only visible to an authenticated account. A
new `adapters.JobLeadsAdapter` implements `jobsources.Adapter` (`Key()`, `Kind()`, `Search()`,
`HealthCheck()`) plus a `FetchDetail` method for enrichment, and is paired with a
`JobLeadsSession` (mirroring `DjinniSession`) that logs in with env-supplied credentials
(`JOBLEADS_EMAIL`/`JOBLEADS_PASSWORD`), storing only the resulting session cookie —
encrypted, alongside other per-source config — in the `job_source.config` column via the
existing `jobsources.Service.Config`/`Update` encrypt-on-write path. `Search` is driven
exclusively by an operator-saved subscription (a JobLeads saved-search URL), following the
Indeed/RemoteOK/Djinni subscription-only stance. On an expired/absent session, the adapter
detects JobLeads's login redirect (mirroring `djinniIsLoginPage`), triggers one re-login via
`Session.Refresh`, and retries once before failing with a distinguishable
"authentication required" error (FR-011). The adapter is registered in `compose.go`'s adapter
list, wired into `enrichment.Handler` alongside the other sources, and added to the ingestion
handler's enrich-eligible source-key allowlist. Dashboard's `SUBSCRIPTION_SOURCES` gets a
`jobleads` entry, and `subscriptions.Service.validateSubscriptionURL` gains a JobLeads
host/shape check. No new tables and no new migration: reuses `JobSource`, `Subscription`,
`Job`, `SourceRun` exactly as Djinni does, with credentials living in env vars (not the DB)
and only the derived session cookie persisted in the existing encrypted `config` JSON column.

## Technical Context

**Language/Version**: Go 1.22+ (apps/api), matches existing adapters package

**Primary Dependencies**: `goquery` for HTML parsing (JobLeads is server-rendered, like
Djinni/DOU/Indeed — no public JSON API is known to exist); `net/http` + `net/http/cookiejar`
for the login flow (mirrors `djinniLogin`); `internal/scraping.Service.FetchHTML` for
authenticated GET requests with the session cookie attached via headers; existing
`internal/crypto` encrypt-on-write path (already used by `jobsources.Service`) for the stored
session cookie — no new crypto code needed

**Storage**: PostgreSQL — no schema changes; reuses `JobSource`, `Subscription`, `Job`,
`SourceRun` tables; session cookie stored in the existing encrypted `job_source.config` JSON
column (same mechanism Djinni already uses)

**Testing**: `go test ./apps/api/...` — fixture-based unit tests using recorded HTML
responses (`testdata/jobleads_list.html`, `testdata/jobleads_detail.html`,
`testdata/jobleads_empty.html`, `testdata/jobleads_login.html`), following the
`djinni_test.go`/`indeed_test.go` pattern; login flow tested against a fake HTTP server (no
live network calls or real JobLeads credentials in unit tests)

**Target Platform**: Linux server (existing `apps/api` Go service, Docker Compose)

**Project Type**: Web application (existing `apps/api` backend + `apps/dashboard` frontend
monorepo) — this feature touches backend primarily, with a one-entry dashboard change

**Performance Goals**: One JobLeads run completes within the existing ingestion cycle window;
request pacing matches Indeed's fixed inter-page delay (FR-010: no faster than one request
per 500ms) and is bounded by a page cap so a single run cannot run unbounded (mirrors
`indeedMaxSubscriptionPages`/`djinniMaxSubscriptionPages`); no measurable (>10%) regression to
other sources' cycle time (SC-007)

**Constraints**: Requires an operator-supplied JobLeads account (`JOBLEADS_EMAIL`/
`JOBLEADS_PASSWORD` env vars) — with neither set, `Search`/`HealthCheck` degrade to a clear
"credentials not configured" error rather than attempting anonymous access (JobLeads has no
meaningful public listing view, unlike Djinni's degrade-to-anonymous option); session cookie
MUST NOT be logged or exposed in API responses (FR-018); requests MUST carry a descriptive
client identifier (FR-017); best-effort — JobLeads's markup/login flow can change without
notice, and such failures must not fail other sources' runs (constitution: Local-First /
discovery-only sources are best-effort)

**Scale/Scope**: Two new adapter files (`jobleads.go`, `jobleads_session.go`) + two test files
+ four testdata HTML fixtures; edits to `compose.go` (registration + session wiring),
`enrichment/handler.go` (dispatch + constructor param), `ingestion/handler.go`
(enrich-eligibility allowlist), `subscriptions/service.go` (URL validation), `config/config.go`
(`JOBLEADS_EMAIL`/`JOBLEADS_PASSWORD` env vars, secret-list registration),
`apps/dashboard/.../SourcesPage.tsx` (subscription source list); no DB migration

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — PASS. Adapter only authenticates to read listings (`Search`,
  `FetchDetail`, `HealthCheck`); no submission, no messaging, no action taken on the user's
  behalf (FR-013). Login exists solely to unlock read access, mirroring Djinni; no code path
  writes back to JobLeads beyond the login POST itself.
- **II. Grounded Generation** — PASS (not applicable to adapter itself). JobLeads listings
  flow through the same normalization → matching → generation pipeline as every other source
  (FR-012); no new generation logic introduced here, so no new hallucination surface.
- **III. Typed Contracts Across Service Boundaries** — PASS. Adapter emits
  `dto.NormalizedJob` (existing shared DTO), same as DOU/Djinni/Indeed/RemoteOK/Glassdoor; no
  new hand-rolled cross-language type. No changes to `packages/shared`, sqlc, or
  tygo-generated types required.
- **IV. Test Discipline Per Language, Enforced at the Boundary** — PASS. New adapter gets
  `go test` coverage with recorded HTML fixtures (matching `djinni_test.go` convention,
  including a login-flow fake-server test); this feature does not touch the dashboard or
  jobspy-sidecar test suites beyond the one-line `SUBSCRIPTION_SOURCES` addition.
- **V. Local-First, Self-Hosted by Default** — PASS. JobLeads is an external discovery-only
  source, same category as Djinni/Indeed/DOU/RemoteOK/Glassdoor; this feature keeps it
  discovery-only and does not introduce a paid third-party *AI* API call for core
  matching/generation. The JobLeads account itself may be a paid subscription the operator
  already holds, but that is a data-source cost, not an AI-inference dependency, and is
  opt-in via env vars (system remains fully operational without it).

No violations. Complexity Tracking not required.

## Project Structure

### Documentation (this feature)

```text
specs/005-jobleads-job-provider/
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
│   ├── djinni.go                     # reference pattern: login-gated Search + fetchDoc retry
│   ├── djinni_session.go             # reference pattern: DjinniSession (Ensure/Refresh)
│   ├── jobleads.go                   # NEW: JobLeadsAdapter (Search, FetchDetail, HealthCheck)
│   ├── jobleads_session.go           # NEW: JobLeadsSession (Ensure/Refresh, mirrors DjinniSession)
│   ├── jobleads_test.go              # NEW: fixture-based unit tests
│   ├── jobleads_session_test.go      # NEW: login-flow tests against a fake HTTP server
│   └── testdata/
│       ├── jobleads_list.html        # NEW: recorded saved-search results fixture
│       ├── jobleads_detail.html      # NEW: recorded listing detail fixture
│       ├── jobleads_empty.html       # NEW: recorded zero-results fixture
│       └── jobleads_login.html       # NEW: recorded login-page fixture (CSRF/form probe)

apps/api/internal/config/
└── config.go                         # EDIT: add JobLeadsEmail/JobLeadsPassword fields,
                                        #       mapstructure tags, secret-list entry

apps/api/internal/enrichment/
└── handler.go                        # EDIT: add `jobleads adapters.JobLeadsAdapter` field,
                                        #       constructor param, dispatch case,
                                        #       enrichJobLeads()

apps/api/internal/ingestion/
└── handler.go                        # EDIT: add "jobleads" to the enrich-eligible
                                        #       source-key check (persistIfNew)

apps/api/internal/subscriptions/
└── service.go                        # EDIT: add a JobLeads case to
                                        #       validateSubscriptionURL (host check)

apps/api/cmd/server/
└── compose.go                        # EDIT: construct JobLeadsSession + JobLeadsAdapter,
                                        #       add to registry list, thread into
                                        #       enrichment.NewHandler

apps/dashboard/src/features/sources/
└── SourcesPage.tsx                   # EDIT: add { key: 'jobleads', label: 'JobLeads', ... }
                                        #       to SUBSCRIPTION_SOURCES
```

**Structure Decision**: This is an addition inside the existing monorepo's Go backend
(`apps/api`), following the established one-file-per-source adapter pattern under
`internal/jobsources/adapters/`, with a companion session file following the Djinni
login-session pattern rather than the simpler stateless RemoteOK/Indeed/DOU pattern. No new
top-level module, package, or app is introduced — JobLeads slots into the same registry,
enrichment dispatch, and subscription flow already built for the other sources. The one
dashboard edit is additive (a new option in an existing source-picker list), not a new page
or component.

## Complexity Tracking

*No constitution violations — table not needed.*
