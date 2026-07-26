# Implementation Plan: Employer ATS Board Sources

**Branch**: `013-ats-board-sources` | **Date**: 2026-07-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/013-ats-board-sources/spec.md`

## Summary

Add five new job-discovery sources (Greenhouse, Lever, Ashby, Workable, SmartRecruiters) that
read employer-hosted ATS boards directly and unauthenticated, plus an employer roster the user
builds by accepting system-proposed candidates (mined from apply URLs already in the feed) or by
pasting a board URL. Each vendor is one `jobsources.Adapter` implementation that reads every
employer in the roster on its run (fan-out over the roster instead of a single search query),
normalizes postings with description already populated (no enrichment pass), and reports
per-employer outcomes. Dedup is extended so an employer-board posting merges into an existing
aggregator-sourced job for the same opening, preferring the board's apply URL while preserving
existing user state.

## Technical Context

**Language/Version**: Go 1.x (`apps/api`), TypeScript/React (`apps/dashboard`) — both existing,
no new language.

**Primary Dependencies**: existing `apps/api` stack — `asynq` (Redis queue), `sqlc`, `goose`;
existing `apps/dashboard` stack — TanStack Query, Tailwind. No new third-party dependency
required: Greenhouse/Lever/Ashby/Workable/SmartRecruiters all publish plain JSON endpoints
reachable with the standard library HTTP client already used by other adapters.

**Storage**: PostgreSQL — new tables for `EmployerBoard` (roster) and `BoardCandidate`
(proposed/rejected candidates); `SourceRun` gains per-employer detail (see Data Model). Existing
`Job`/`JobSource` tables reused, not replaced.

**Testing**: `go test` for `apps/api` (adapter unit tests per vendor, roster/candidate service
tests, dedup-merge tests); `vitest` for `apps/dashboard` (roster UI); `make test-integration` for
the merge-across-sources and per-employer-failure-isolation scenarios, which need a real Postgres.

**Target Platform**: Linux server (Docker Compose), same as rest of the system.

**Project Type**: Web application (existing `apps/api` + `apps/dashboard` monorepo layout).

**Performance Goals**: Not throughput-sensitive; bounded by FR-007's run caps (max employers per
run, max postings per employer) and by the existing per-host pacing (FR-006) — no new performance
target beyond "stays within existing pacing."

**Constraints**: No credentials, no stored sessions, no challenge-solving (FR-001) — this rules
out headless-browser fetching for these five vendors; if a vendor turns out not to expose a plain
JSON/HTML endpoint, it is dropped from launch scope per the spec's own Assumptions, not routed
through Feature 014's fetch ladder. FR-021 (never report a run successful with zero employers
read) and FR-019 (per-employer failure isolation) directly shape the run loop's error handling.

**Scale/Scope**: Roster expected to grow to hundreds of employers (Edge Cases); five new adapter
files, one new roster/candidate service, one new dashboard panel, extension to the existing dedup
step.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply** — PASS. Feature only reads and surfaces postings; apply URLs are for the
  user to act on manually (spec Assumptions explicitly reaffirms this). No submission code path
  is introduced.
- **II. Grounded Generation** — N/A. Feature does not generate LLM content; it ingests postings
  that flow into the existing scoring/generation pipeline unchanged.
- **III. Typed Contracts Across Service Boundaries** — PASS, with an obligation: any new roster/
  candidate fields the dashboard needs (vendor, employer id, stale flag, per-run counts) MUST be
  added to `packages/shared` / regenerated via tygo, not hand-duplicated in `apps/dashboard`.
  Tracked as a Phase 1 design constraint, not a violation.
- **IV. Test Discipline Per Language** — PASS, planned: adapter and service tests in `go test`,
  roster UI in `vitest`, cross-service merge/isolation behavior in `make test-integration` against
  real Postgres (per FR-015–FR-019, which are inherently cross-record behaviors mocks would hide).
- **V. Local-First, Self-Hosted by Default** — PASS. All five vendors are free, unauthenticated,
  public read endpoints — no paid third-party service (SC-002 makes this an explicit success
  criterion).

No violations requiring Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/013-ats-board-sources/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md         # Phase 1 output
├── quickstart.md         # Phase 1 output
├── contracts/            # Phase 1 output
└── tasks.md              # Phase 2 output (/speckit.tasks, not this command)
```

### Source Code (repository root)

```text
apps/api/internal/jobsources/
├── adapter.go                     # existing Adapter interface — reused as-is
├── adapters/
│   ├── greenhouse.go              # new
│   ├── greenhouse_test.go
│   ├── lever.go                   # new
│   ├── lever_test.go
│   ├── ashby.go                   # new
│   ├── ashby_test.go
│   ├── workable.go                # new
│   ├── workable_test.go
│   ├── smartrecruiters.go         # new
│   └── smartrecruiters_test.go
└── roster/                        # new package
    ├── service.go                 # EmployerBoard CRUD, stale flagging
    ├── candidates.go              # candidate discovery from apply URLs, accept/reject
    └── *_test.go

apps/api/internal/db/migrations/
└── 000XX_ats_board_roster.sql     # new: EmployerBoard, BoardCandidate tables (+ SourceRun detail)

apps/api/internal/ingestion/
├── dedupe.go                      # extended: merge-preferring-board-url logic
└── handler.go                     # extended: per-employer run loop, outcome reporting

apps/api/internal/httpapi/
└── roster.go                      # new: roster CRUD + candidate accept/reject endpoints

apps/api/cmd/server/compose.go     # register 5 new adapters + roster service
apps/api/cmd/seed/main.go          # same registration, seed path

packages/shared/
└── (EmployerBoard/BoardCandidate DTOs, extended per Principle III)

apps/dashboard/src/features/sources/
├── SourcesPage.tsx                # extended: 5 vendors appear via existing per-source panel
└── roster/                        # new: RosterPanel, CandidatesPanel, hooks.ts
```

**Structure Decision**: Extends the existing single-monorepo web-application layout
(`apps/api` Go backend + `apps/dashboard` React frontend + `packages/shared` types) already used
by every other source. No new app or service boundary. New work is additive: one `roster`
subpackage under `jobsources`, one migration, one new dashboard feature subdirectory — the
per-vendor adapter pattern, queue, and Sources screen wiring are all reused unchanged.

## Complexity Tracking

*No Constitution Check violations — table not needed.*
