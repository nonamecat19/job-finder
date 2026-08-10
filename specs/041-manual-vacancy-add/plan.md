# Implementation Plan: Manual Vacancy Add by URL

**Branch**: `041-manual-vacancy-add` | **Date**: 2026-08-10 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/041-manual-vacancy-add/spec.md`

## Summary

Let the operator paste a single posting URL into the feed and get an ordinary vacancy back,
synchronously, within 30 seconds. The URL is resolved to a known source, the posting page is
read through the existing retrieval ladder, and the result is persisted through the existing
ingest path so every downstream capability works with no special case.

The one genuine gap is that **no adapter can currently read a standalone posting page into a
complete vacancy**. `FetchDetail` exists on nine adapters, but it returns a *patch*
(description, salary, location, postedAt) applied to a job whose title and company already
came from a search card. Manual add starts from nothing, so it needs a new port —
`domain.PostingReader` — implemented per source. Djinni ships first; every other host degrades
to the P3 fill-in form until its reader is added. That degradation is already sanctioned by
the spec's Assumptions.

Everything else is assembly of parts that exist: `Subscription` gains a `kind`, `SourceRun`
gains a subscription link and a trigger, the shared persist/dedupe code moves out of the
asynq worker so a synchronous caller can use it, and the feed gains a Manual filter and a
24-hour surfacing term in its ordering.

## Technical Context

**Language/Version**: Go 1.25 (`apps/api`), TypeScript 5.x + React 19 (`apps/dashboard`)

**Primary Dependencies**: chi (HTTP), sqlc (typed DB access), goose (migrations), pgx v5,
hibiken/asynq (async work), goquery (HTML parsing), TanStack Query + HeroUI + Tailwind
(dashboard), tygo (Go→TS type generation)

**Storage**: PostgreSQL with pgvector; Redis for asynq queues

**Testing**: `go test` (apps/api), vitest (dashboard), Docker-backed
`make test-integration` for real Postgres/Redis paths

**Target Platform**: Self-hosted Linux via Docker Compose

**Project Type**: Web application — Go API + React dashboard in a pnpm monorepo

**Performance Goals**: A manual add returns a definite outcome within 30 s wall-clock
(FR-003b), including any per-host pacing wait. Post-ingest work (match, ghost, enrich) is
asynchronous and unbounded, exactly as for a crawl.

**Constraints**: Synchronous HTTP request — the add cannot be an asynq task (FR-003a).
Per-host pacing must not be bypassed (FR-003c). Failed manual adds must not count toward the
3-consecutive-failure source-health threshold (FR-017g).

**Scale/Scope**: Single operator, a handful of adds per day. No bulk paste, no batching.
Roughly: 1 migration, 1 new Go feature module, 1 new adapter port with one implementation,
2 modified SQL query files, ~4 dashboard components.

## Constitution Check

*GATE: checked before Phase 0, re-checked after Phase 1 design.*

| Principle | Verdict | Reasoning |
|---|---|---|
| **I. No Auto-Apply, Ever** | ✅ Pass | Manual add creates a vacancy in the feed. It submits nothing, contacts no employer, and touches no application state. It is strictly more human-in-the-loop than a crawl. |
| **II. Grounded Generation** | ✅ Pass | No LLM generation is introduced. Extracted fields come from the posting page; hand-filled fields come from the operator. Downstream tailoring reads the stored posting exactly as it does for a crawled one, so its grounding is unchanged. |
| **III. Typed Contracts Across Boundaries** | ✅ Pass, with an obligation | New DTOs are defined once in `apps/api/internal/dto` and regenerated into `packages/shared/src/generated.ts` via tygo. No hand-written TS mirror. `make tygo-check` (scripts/tygo-check.sh) must be green. |
| **IV. Test Discipline Per Language** | ✅ Pass, with an obligation | Adapter reader tests use fixture HTML; persistence and dedupe get integration tests against real Postgres following `persist_integration_test.go`. The feature spans `apps/api`, `apps/dashboard` and `packages/shared`, so `make test-lint` is required before done, plus `make sqlc-check` and `make tygo-check`. |
| **V. Local-First, Self-Hosted** | ✅ Pass | Reuses the existing retrieval ladder against public job boards — discovery, not inference. No new external dependency and no AI call in the add path. |

**Gate result: PASS.** No violations, so Complexity Tracking is empty.

One design choice deserves recording even though it violates nothing: moving persist/dedupe
out of `jobsources/interfaces/worker` (see research.md D5) is a refactor of working code. It
is justified by Principle III's sibling rule in `domains/codebase-structure.md` — one copy of
each behaviour — and the smaller alternative is documented and rejected there.

## Project Structure

### Documentation (this feature)

```text
specs/041-manual-vacancy-add/
├── plan.md              # This file
├── research.md          # Phase 0 — the eight decisions this design rests on
├── data-model.md        # Phase 1 — schema deltas and entity rules
├── quickstart.md        # Phase 1 — how to run and verify it
├── contracts/
│   └── manual-add.md    # Phase 1 — HTTP contract, error taxonomy, adapter port
├── checklists/
│   └── requirements.md  # From /speckit.specify + /speckit.clarify
└── tasks.md             # Phase 2 — NOT created by /speckit.plan
```

### Source Code (repository root)

```text
apps/api/internal/
├── db/
│   ├── migrations/
│   │   └── 00041_manual_add.sql          # NEW — Subscription.kind, SourceRun link/trigger
│   ├── queries/
│   │   ├── joblist.sql                   # MOD — only_manual filter, 24h surfacing term
│   │   ├── subscription.sql              # MOD — kind-aware create/list, manual lookups
│   │   └── sourcerun.sql                 # MOD — health query excludes manual runs
│   └── sqlcgen/                          # REGEN — sqlc generate
├── manualadd/                            # NEW feature module
│   ├── domain/
│   │   ├── errors.go                     # the six FR-018 failure kinds
│   │   ├── ports.go                      # repository + reader ports
│   │   └── outcome.go                    # created | duplicate | needs_fill_in | failed
│   ├── application/
│   │   ├── service.go                    # resolve → read → persist → enqueue, 30s bound
│   │   ├── fillin.go                     # P3 hand-filled save path
│   │   └── *_test.go
│   └── interfaces/http/
│       └── manualadd.go                  # POST /jobs/manual, POST /jobs/manual/fill-in
├── jobsources/
│   ├── domain/
│   │   └── adapter.go                    # MOD — PostingReader port + helper
│   ├── application/ingest/               # NEW — persist.go + dedupe.go moved here
│   ├── infrastructure/adapters/
│   │   └── djinni.go                     # MOD — ReadPosting + MatchesPostingURL
│   └── interfaces/worker/
│       ├── handler.go                    # MOD — trigger + subscription on SourceRun
│       └── scheduler.go                  # MOD — never schedule kind='manual'
├── subscriptions/
│   ├── application/service.go            # MOD — manual kind, ensure-on-first-use, guards
│   └── interfaces/http/                  # MOD — reject writes against manual subs
└── dto/jobs.go                           # MOD — ManualAdd DTOs, SubscriptionDto.kind

packages/shared/src/generated.ts          # REGEN — tygo

apps/dashboard/src/
├── features/feed/
│   ├── AddVacancyForm.tsx                # NEW — URL input + outcome handling
│   ├── ManualFillInDialog.tsx            # NEW — P3 form
│   ├── FeedPage.tsx                      # MOD — mount form, Manual filter chip
│   └── hooks.ts                          # MOD — useAddVacancyByUrl, useSaveManualVacancy
├── features/sources/SourcesPage.tsx      # MOD — render manual subs read-only
└── lib/api.ts                            # MOD — endpoints, JobFilters.onlyManual
```

**Structure Decision**: Follows the feature-module layout that
`specs/domains/codebase-structure.md` binds (024, 027) — a new `manualadd` module with
`domain` / `application` / `interfaces` layers, matching `subscriptions/` and `jobsources/`.
Manual add is its own module rather than a handler bolted onto `jobsources` because it is a
distinct, synchronous entry point with its own failure taxonomy; it *depends* on jobsources'
adapter registry and the extracted ingest package rather than living inside them.

## Phase 0 — Research

See [research.md](research.md). Eight decisions, in dependency order:

- **D1** — a new `PostingReader` port, because `FetchDetail` cannot produce title or company.
- **D2** — URL→source resolution and posting-vs-search discrimination live on the adapter.
- **D3** — `Subscription.kind` discriminates manual from crawling; one manual sub per source.
- **D4** — a registered no-op `manual` source backs fill-in vacancies on unknown hosts.
- **D5** — persist/dedupe move to `jobsources/application/ingest` so both callers share them.
- **D6** — the 30 s bound is one `context.WithTimeout` covering pacing, fetch and parse.
- **D7** — `SourceRun` gains `subscriptionId` + `trigger`; health counts exclude manual runs.
- **D8** — feed surfacing is an ordering term, not a stored flag; the posted date stays true.

## Phase 1 — Design

- [data-model.md](data-model.md) — the migration, entity rules, and every invariant that has
  to hold after it.
- [contracts/manual-add.md](contracts/manual-add.md) — the two endpoints, the outcome
  envelope, the six-way error taxonomy mapped to HTTP, and the Go adapter port.
- [quickstart.md](quickstart.md) — running it, adding a Djinni URL end to end, and the
  verification checklist tied to the spec's success criteria.

**Post-design Constitution re-check: PASS.** The design adds no third-party inference, no
hand-written cross-language types, no auto-apply path, and no untested cross-service
behaviour. The obligations recorded in the gate table (tygo regeneration, `make test-lint`)
are carried into Phase 2 as tasks rather than assumed.

## Known deltas from the spec

Two, both already sanctioned by the spec's own Assumptions but worth stating plainly so
Phase 2 does not treat them as bugs:

1. **FR-022 lands partially in this feature.** Automatic extraction requires a `PostingReader`
   per source; only Djinni gets one here. Every other known host behaves as FR-023 describes
   (told there is no reader, offered the fill-in form). Adding readers is per-source follow-up
   work, and the port is designed so each is ~80 lines plus a fixture test.
2. **FR-023's fill-in fallback depends on User Story 3 (P3).** Until P3 ships, an unreadable
   or unknown host is a clear rejection (FR-018), as the spec's P3 rationale anticipates.
   Sequencing P1 → P2 → P3 keeps each slice independently shippable.
