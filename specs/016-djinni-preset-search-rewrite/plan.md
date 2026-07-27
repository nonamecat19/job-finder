# Implementation Plan: Djinni Preset-Search Rewrite

**Branch**: `016-djinni-preset-search-rewrite` | **Date**: 2026-07-28 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/016-djinni-preset-search-rewrite/spec.md`

## Summary

Rewrite the Djinni job-source adapter so that the **only** supported
Djinni subscription shape is the public preset-search URL
(`djinni.co/jobs/?search_type=basic-search&...`), which runs anonymously
(no login) and paginates 1..N pages. Delete the logged-in dashboard
(`subs/{id}/`) fetch path, the session-login machinery, the dual-mode
routing, and their config/env wiring. Reject `subs/{id}/` URLs at save
time and delete pre-existing `subs/{id}/` subscriptions via a one-time
goose migration that records what was removed. Preserve the shared
fetch/pagination/parse/enrich/dedup/match/ghost pipeline, the existing
display-summary rules (range-vs-list for `exp_level`), and all
non-Djinni sources.

## Technical Context

**Language/Version**: Go 1.x (apps/api backend), TypeScript (apps/dashboard, Vite + React + TanStack Query + Tailwind)

**Primary Dependencies**: gocolly/scraper-style HTML fetch via the shared
`scraping.Service` (browser-identity retrieval ladder), `goquery` for HTML
parsing, sqlc + goose for DB, TanStack Query for the dashboard, the
in-house `packages/shared` TS types. No new dependencies introduced.

**Storage**: PostgreSQL (existing `Subscription` table, `JobSource`
encrypted-`config` JSON). goose for migrations — new migration
`00027_drop_djinni_dashboard_subs.sql` follows the sequential,
never-reuse numbering enforced by the constitution.

**Testing**: `go test` for the api; `vitest` for the dashboard. The
constitution's quality gate requires `make test-lint` (both suites) to
pass before a cross-app change is "done"; integration tests exercise
real Postgres/Redis via Docker Compose (not mocks).

**Target Platform**: Linux server via Docker Compose (dev) / prod
compose. The adapter is a backend service; the dashboard ships in the
same stack.

**Project Type**: web service (Go API) + dashboard (React/Vite SPA) in a
pnpm monorepo.

**Performance Goals**: Source behavior unchanged from today — best-effort
scraping of a public page, bounded to `djinniMaxSubscriptionPages = 50`,
paced by `DJINNI_DETAIL_DELAY_MS` (kept). The rewrite does not change
throughput targets.

**Constraints**: Anonymous (`UsesUserAccount()` flips to false) — lets
the retrieval ladder escalate to browser/FlareSolverr rungs for anon
public pages, which is *more* resilient than today's credentialed
direct-rung-only path. Per-scrape rate is bounded by the existing
inter-page politeness (the loop's empty-page/loop-guard/50-cap). No
auto-apply (constitution Principle I).

**Scale/Scope**: One adapter rewrite (~351 → ~200 LOC), one deleted
session file (~181 LOC), one deleted search-mode enum arm, one
validator arm rewrite, one dashboard label simplification, one goose
migration, config/env cleanup, two test files pruned. No DB schema
change beyond the data-only delete migration. No `packages/shared`
regeneration needed (no DTO field changes) and no `sqlc`/`tygo`
regeneration needed.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Project constants from `.specify/memory/constitution.md` v1.0.0:

| Principle | Status | Evidence |
|---|---|---|
| **I. No Auto-Apply** (NON-NEGOTIABLE) | ✅ Pass | The rewrite is discovery-only. `Search`/`FetchDetail`/`paginateDjinni` ingest listings; no apply/message/message-employer path is introduced or kept. FR-018 restates this. The deleted `subs/{id}/` mode was also discovery-only, so deletion does not regress Principle I. |
| **II. Grounded Generation** | ✅ Pass (unchanged scope) | The adapter produces `dto.NormalizedJob` rows that feed the existing enrich/match/generate pipeline; it does not generate resume/cover-letter content. The LLM-grounding invariant is enforced downstream and is untouched. |
| **III. Typed Contracts Across Service Boundaries** | ✅ Pass | No `dto.go` field is added, removed, or renamed. `packages/shared` carries no Djinni-specific DTO today (grep-confirmed). The dashboard's `summarizeDjinniBasicSearch` is a local TS helper over `sub.url: string`, not a shared DTO. No `tygo` regeneration; no hand-maintained TS duplication introduced. The deletion removes the dashboard-side `djinniModeMarker` dashboard branch (a local UI label), not a shared type. |
| **IV. Test Discipline Per Language** | ✅ Pass | Go adapter tests (`go test`) prune dashboard/session/login cases and keep the basic-search/pagination/detect/parse cases. Dashboard helper tests (`vitest`) keep the `summarizeDjinniBasicSearch`/`summarizeExpLevels` cases; the dashboard-mode `null` case stays as a "not a preset URL" assertion. `make test-lint` is the gate before merge (FR cross-app). |
| **V. Local-First, Self-Hosted** | ✅ Pass | The rewrite removes a *third-party paid* dependency surface (Djinni login credentials). The adapter runs against the public web page; the core local Ollama/Postgres/Redis flow is unchanged. Removing `DJINNI_EMAIL`/`DJINNI_PASSWORD` config strictly reduces third-party coupling. ✅ aligned with V. |

**Technology & Architecture Constraints**: ✅ Go + sqlc + goose (new
migration `00027_…` — next sequential after `00026_host_retrieval_state.sql`,
never reused); ✅ dashboard React+Vite+TanStack+Tailwind; ✅
scraping-based source treated as best-effort/unstable (unchanged); ✅
`packages/shared` single source of truth for TS types (no change); ✅
Docker Compose stack (no topological change). No new external paid
service; the single "third-party paid" surface, the Djinni login, is
*removed* — strict improvement under V.

**Development Workflow & Quality Gates**: ✅ `pnpm install` + `pnpm
--filter @job-finder/shared build` are not needed (no shared DTO change);
✅ use `make test-lint` as the canonical cross-app gate; ✅ Plan written
under `specs/016-…/plan.md` before implementation (this document);
trivial fixes/refactors do not require a separate plan, but this is a
cross-app deletion and does.

**Gate verdict**: ✅ All gates pass pre-Phase-0; no Complexity Tracking
violations to justify.

## Project Structure

### Documentation (this feature)

```text
specs/016-djinni-preset-search-rewrite/
├── plan.md              # this file
├── research.md          # Phase 0 output (see below)
├── data-model.md        # Phase 1 output (see below)
├── quickstart.md        # Phase 1 output (see below)
├── contracts/           # Phase 1 output (see below)
└── tasks.md             # /speckit.tasks output — not created here
```

### Source Code (repository root)

```text
apps/api/
├── internal/db/migrations/
│   └── 00027_drop_djinni_dashboard_subs.sql   # NEW: data-only delete + audit
├── internal/config/
│   ├── config.go                                  # drop DjinniEmail/DjinniPassword fields
│   └── defaults.go                              # drop DJINNI_EMAIL/DJINNI_PASSWORD from optional list
├── internal/subscriptions/service.go            # collapse validateDjinniSubscriptionURL to preset-only
├── internal/jobsources/adapters/
│   ├── djinni.go                                  # rewrite: trim to preset-only + anonymous fetch
│   ├── djinni_searchmode.go                       # drop DjinniModeDashboard, keep basic-search
│   ├── djinni_session.go                           # DELETE whole file
│   ├── djinni_test.go                              # prune session/login/dashboard cases
│   └── djinni_searchmode_test.go                  # prune dashboard-detect cases
├── internal/enrichment/handler.go               # drop session config fetch for the djinni case
├── cmd/server/
│   ├── platform.go                                # drop Platform.DjinniSession field + construction
│   ├── compose_sources.go                         # drop DjinniSession wiring
│   └── compose_tasks.go                           # keep DjinniDetailDelayMs (kept, comment reworded)
└── (cmd/seed/main.go: DjinniAdapter{} already anon — no edit needed)
.env.example                                          # drop DJINNI_EMAIL/DJINNI_PASSWORD lines
apps/dashboard/src/features/sources/
├── SourcesPage.tsx                                  # update placeholder + simplify djinniModeMarker
├── djinniSearchSummary.ts                          # no behavior change (keep verbatim)
└── djinniSearchSummary.test.ts                    # keep as-is (acts as "only preset URL" contract)
packages/shared/                                     # NO REGENERATION (no DTO change)
```

**Structure Decision**: Monorepo with Go backend (`apps/api`) and React
dashboard (`apps/dashboard`) sharing `packages/shared`. The rewrite is
confined to those three trees; the **only** new file in the repo is the
goose migration `00027_drop_djinni_dashboard_subs.sql`. Everything else
is edits to or deletions of existing Djinni-specific files, plus the
two-dashboard-file edit. No new package, no new app, no new
dependency.

## Complexity Tracking

> No constitution violations to justify. The rewrite is a *removal* of a
> project sub-feature (logged-in Djinni session mode), strictly reducing
> architectural complexity while keeping one of its two search modes.
> The three-app layout (`apps/api`, `apps/dashboard`, `packages/shared`)
> and every shared infrastructure piece (scraping ladder, sqlc, goose,
> tygo pipeline, shared types) remains unchanged.