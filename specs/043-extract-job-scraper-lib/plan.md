# Implementation Plan: Extract Job Scraper Library

**Branch**: `043-extract-job-scraper-lib` | **Date**: 2026-08-10 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/043-extract-job-scraper-lib/spec.md`

## Summary

Extract the `apps/api` job-scraping stack — 25 site adapters, the adapter framework, the HTML/JSON helpers, the `platform/scraping` Scraper port, and the multi-rung retrieval engine — into a standalone Go module (`jobscraper`) consumable by any Go project with no DB/queue/config dependency. The app wires its Postgres-backed state store and roster repository into two library-defined ports (`StateStorePort`, `RosterPort`). The 6 board adapters move whole and call through `RosterPort`. `strutil` is vendored into the library. The app's `dto` re-exports the 4 moved model types as aliases so the tygo-generated dashboard contracts are byte-identical.

## Technical Context

**Language/Version**: Go 1.26.5 (matches `apps/api/go.mod`)

**Primary Dependencies (library)**:
- `github.com/PuerkitoBio/goquery` — HTML parsing (htmlutil + adapters)
- `github.com/bogdanfinn/fhttp` + `github.com/bogdanfinn/tls-client` — direct rung TLS client
- `github.com/chromedp/chromedp` — browser rung + HTTPScraper
- `golang.org/x/time/rate` — per-host rate limiting (if vendored; see research.md)

**Primary Dependencies (app, unchanged)**: pgx/v5, asynq, viper, minio, pgvector, goose — none of these enter the library.

**Storage**: N/A for the library. The app keeps Postgres (sqlc) + Redis (asynq). The library talks to storage only through `StateStorePort` and `RosterPort`.

**Testing**: `go test` for both library and app. The library's adapter tests move with the adapters and use the same fixtures. `make test-lint` remains the merge gate.

**Target Platform**: Linux (dev/prod), Go cross-platform.

**Project Type**: library (jobscraper) + existing web-service/API (apps/api as consumer).

**Performance Goals**: No regression vs. pre-extraction (same adapters, same rungs, same pacing). No new perf target.

**Constraints**:
- Library `go.mod` MUST NOT declare `pgx`/`asynq`/`viper`/`minio`/`pgvector`/`goose`/`job-finder/api/internal/*` (FR-002).
- Retrieval engine importable without pulling in `adapters` (FR-016).
- App's `crypto`/`CONFIG_ENCRYPTION_KEY` never crosses into the library (FR-012).
- tygo-generated `packages/shared/src/generated.ts` byte-identical (FR-015).

**Scale/Scope**: 25 adapters (~5.5k LOC in `infrastructure/adapters/`), retrieval engine (~1.2k LOC), helpers (~150 LOC), framework (~300 LOC). One new Go module, one app-side rewiring.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. No Auto-Apply | ✅ Pass | Scraping is discovery only; does not submit applications. Unaffected. |
| II. Grounded Generation | ✅ Pass | No generation in scope. Unaffected. |
| III. Typed Contracts Across Service Boundaries | ✅ Pass | The moved 4 types are re-exported as aliases from `dto`; tygo generation unchanged (FR-015/SC-005). The library↔app boundary uses library-owned port interfaces (StateStorePort/RosterPort), not shared generated types — appropriate for a Go-internal boundary, not a cross-language one. |
| IV. Test Discipline Per Language | ✅ Pass | `make test-lint` is the gate (FR-014/SC-004); adapter tests move with the code and run in `go test`. |
| V. Local-First, Self-Hosted | ✅ Pass | External job sources remain discovery-only. The library has no LLM dependency. The retrieval engine's rungs (direct/browser/flaresolverr) are all self-hostable. |

**Technology & Architecture Constraints**: ✅ Go + sqlc + goose on the app side; the library drops sqlc/goose entirely (ports instead). No migration version added. asynq stays in the app.

**Development Workflow & Quality Gates**: ✅ Plan doc at `specs/043-*/plan.md`; `make test-lint` enforced; feature directory removed on ship per `specs/README.md`.

No violations. Complexity Tracking section not needed.

## Project Structure

### Documentation (this feature)

```text
specs/043-extract-job-scraper-lib/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── state-store-port.md
│   ├── roster-port.md
│   ├── retrieval-service.md
│   └── adapter-framework.md
└── tasks.md             # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
jobscraper/                      # NEW Go module (separate repo or subdir; replace directive during dev)
├── go.mod                       # module github.com/<org>/jobscraper
├── model/
│   └── job.go                   # NormalizedJob, SearchQuery, SourceKind, JobSourceDto (from dto/jobs.go + dto.go)
├── adapter/                     # from jobsources/domain/adapter.go + errors.go
│   ├── adapter.go               # Adapter, PostingReader, Credentialed, DetailNeeder, Registry, EmployerOutcome
│   └── errors.go
├── httpjson/                    # from jobsources/httpjson.go (transport-injectable, no retrieval dep)
├── htmlutil/                    # from jobsources/htmlutil.go
├── strutil/                     # vendored ~30 lines from apps/api/internal/strutil
├── scraping/                    # from platform/scraping/
│   ├── scraper.go               # Scraper port (alias of domain.Scraper)
│   └── httpscraper.go           # chromedp-backed impl
├── retrieval/                   # from retrieval/
│   ├── ladder.go                # RetrievalMethod, rungs
│   ├── challenge.go             # IsChallenged, IsRefused
│   ├── outcome.go               # PageStatus, PageOutcome
│   ├── identity.go              # BrowserIdentity
│   ├── service.go               # Service interface, FetchRequest, FetchResult, HostStatus
│   ├── state_port.go             # StateStorePort interface (library types)
│   ├── direct.go                # directRung (bogdanfinn tls-client)
│   ├── browser.go               # browserRung (chromedp)
│   └── flaresolverr.go          # flareSolverrRung
├── rosterport/                  # NEW: port for board adapters
│   └── roster_port.go           # RosterPort interface + library-owned EmployerBoard/BoardCandidate types
└── adapters/                    # from jobsources/infrastructure/adapters/
    ├── adzuna.go
    ├── arbeitnow.go
    ├── ashby.go
    ├── atsboard.go              # board helpers: runBoardVendor, vendorHealthCheck, boardRunState
    ├── djinni.go
    ├── djinni_session.go
    ├── dou.go
    ├── glassdoor.go
    ├── greenhouse.go
    ├── himalayas.go
    ├── indeed.go
    ├── jobgether.go
    ├── jobleads.go
    ├── jobleads_session.go
    ├── jobspy.go
    ├── jooble.go
    ├── lever.go
    ├── live_smoke_test.go
    ├── manual.go
    ├── remoteok.go
    ├── remotive.go
    ├── robota.go
    ├── smartrecruiters.go
    ├── testdata/
    ├── wellfound.go
    ├── workable.go
    └── workua.go

apps/api/
├── go.mod                        # adds require github.com/<org>/jobscraper + replace -> ../../jobscraper (dev)
├── internal/
│   ├── dto/
│   │   ├── jobs.go               # 4 types replaced with: type NormalizedJob = jobscraper.NormalizedJob  (aliases)
│   │   └── dto.go                # SourceKind removed (now alias of jobscraper.SourceKind)
│   ├── jobsources/
│   │   ├── domain/
│   │   │   ├── adapter.go        # REMOVED (moved to library); re-export aliases if app code still references
│   │   │   ├── errors.go         # REMOVED (moved)
│   │   │   ├── job_source.go     # stays (app-specific ToDTO wiring)
│   │   │   ├── repository.go     # stays (sqlcgen-typed)
│   │   │   └── search_repository.go  # stays
│   │   ├── application/          # stays (Service, SearchService, ingest/) — imports library adapter framework
│   │   ├── roster/               # stays — implements library RosterPort
│   │   │   ├── ports.go          # stays (sqlcgen Repository interface — app-internal)
│   │   │   ├── service.go        # becomes RosterPort impl adapter
│   │   │   └── ...
│   │   ├── interfaces/           # stays (HTTP + worker)
│   │   ├── httpjson.go           # REMOVED (moved)
│   │   ├── htmlutil.go           # REMOVED (moved)
│   │   └── util.go               # REMOVED (moved or merged into library strutil)
│   ├── retrieval/
│   │   ├── service_impl.go       # stays — implements library Service using app StateStore + config
│   │   ├── state.go              # stays — implements library StateStorePort, uses app crypto
│   │   ├── transport.go          # stays — wires library ratelimit/app state store
│   │   ├── ladder.go             # REMOVED (moved)
│   │   ├── challenge.go          # REMOVED (moved)
│   │   ├── outcome.go            # REMOVED (moved)
│   │   ├── identity.go           # REMOVED (moved)
│   │   ├── service.go             # REMOVED (interface moved; app keeps ServiceImpl)
│   │   ├── direct.go             # REMOVED (moved)
│   │   ├── browser.go            # REMOVED (moved)
│   │   └── flaresolverr.go       # REMOVED (moved)
│   ├── platform/scraping/       # REMOVED (moved to library; app imports jobscraper/scraping)
│   └── arch_test.go              # exemption list updated if testutil leaves (out of scope here)
└── ...
```

**Structure Decision**: Single new Go module (`jobscraper`) consumed by the existing `apps/api` monorepo app via a `replace` directive. The library is flat-package (no `internal/`) so consumers can reach `retrieval` without pulling `adapters`. The app retains all DB/queue/HTTP-coupled code and implements the two ports.