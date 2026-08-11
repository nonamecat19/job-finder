# Tasks: Extract Job Scraper Library

**Input**: Design documents from `/specs/043-extract-job-scraper-lib/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are NOT generated as separate TDD tasks — the existing adapter unit tests move with the code and serve as the regression gate (FR-013). Quickstart.md scenarios are the validation tasks in the final phase.

**Organization**: Tasks are grouped by user story. US1 (standalone library) is the MVP and carries the bulk of the move; US2 (retrieval engine) is a sub-slice that lands as part of US1's library creation; US3 (zero-regression guardrail) is the app-side rewiring + validation.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Library: `../jobscraper/` (sibling repo; `replace` in `apps/api/go.mod` points here during dev)
- App: `apps/api/` (existing monorepo)
- Paths are relative to repo root unless the task names a file in the sibling library

## Phase 1: Setup (Library Skeleton)

**Purpose**: Create the new Go module with its `go.mod` and the directory structure from plan.md. No code moves yet.

- [X] T001 Create the `../jobscraper/` directory and run `go mod init github.com/job-finder/jobscraper` (sets Go version to 1.26.5 to match the app)
- [X] T002 [P] Create empty package directories in `../jobscraper/`: `model/`, `adapter/`, `httpjson/`, `htmlutil/`, `strutil/`, `scraping/`, `retrieval/`, `rosterport/`, `adapters/`, `adapters/testdata/`
- [X] T003 Add `require github.com/job-finder/jobscraper v0.0.0` and `replace github.com/job-finder/jobscraper => ../jobscraper` to `apps/api/go.mod`

---

## Phase 2: Foundational (Shared Library Types & Ports)

**Purpose**: Move the model types and define the two port interfaces BEFORE any adapter or engine code moves. Everything downstream depends on these.

**⚠️ CRITICAL**: No adapter/engine move can begin until this phase is complete — every moved file imports `model` and most import a port.

- [X] T004 [P] Move `NormalizedJob`, `SearchQuery`, `JobSourceDto` from `apps/api/internal/dto/jobs.go` into `../jobscraper/model/job.go` (verbatim, package `model`)
- [X] T005 [P] Move `SourceKind` type + 4 constants from `apps/api/internal/dto/dto.go` into `../jobscraper/model/job.go` (same file as T004 or a sibling `source_kind.go`)
- [X] T006 [P] Replace the 4 moved types in `apps/api/internal/dto/jobs.go` and `apps/api/internal/dto/dto.go` with type aliases pointing at `github.com/job-finder/jobscraper/model` (e.g. `type NormalizedJob = model.NormalizedJob`); keep the `SourceKind` const block re-declaring the aliases against `model.SourceKind`
- [X] T007 [P] Create `../jobscraper/adapter/adapter.go` from `apps/api/internal/jobsources/domain/adapter.go`: copy `Adapter`, `PostingReader`, `Credentialed`, `DetailNeeder`, `EmployerReporter`, `EmployerOutcome`/constants, `EmployerRunOutcome`, `Registry`, helper fns; replace `dto` imports with `github.com/job-finder/jobscraper/model`
- [X] T008 [P] Create `../jobscraper/adapter/errors.go` from `apps/api/internal/jobsources/domain/errors.go` (`SourceNotFoundError`, `AdapterNotRegisteredError`) — package `adapter`
- [X] T009 [P] Create `../jobscraper/retrieval/state_port.go` per `contracts/state-store-port.md`: define `HostState` struct and `StateStorePort` interface (9 methods, library-owned types, no `pgx`/`pgtype`)
- [X] T010 [P] Create `../jobscraper/rosterport/roster_port.go` per `contracts/roster-port.md`: define `EmployerBoard`, `BoardCandidate`, `EmployerHealthChecker` type, and `RosterPort` interface (11 methods)
- [X] T011 Verify the library compiles with `cd ../jobscraper && go build ./...` (empty packages are fine; `model`, `adapter`, `retrieval`, `rosterport` should have declarations)

**Checkpoint**: Library skeleton + model + framework + ports in place. App still builds because aliases keep `dto` consumers working. Ready to move adapters and engine.

---

## Phase 3: User Story 1 — Scraping capability is consumable as a standalone library (Priority: P1) 🎯 MVP

**Goal**: All 25 adapters + helpers + framework compile in the library; the app imports them from the library; existing tests pass unchanged.

**Independent Test**: Quickstart scenario 1 (`cd ../jobscraper && go build ./... && go test ./...` with clean `go.mod`) and scenario 4 (app tests pass).

### Implementation for User Story 1

#### Helpers (no project coupling)

- [X] T012 [P] [US1] Move `apps/api/internal/jobsources/htmlutil.go` → `../jobscraper/htmlutil/htmlutil.go` (package `htmlutil`, keep `goquery` dep); move `htmlutil_test.go` alongside
- [X] T013 [P] [US1] Move `apps/api/internal/jobsources/util.go` (`Ptr`, `NilIfEmpty`, `StringOr`) → `../jobscraper/strutil/strutil.go` (package `strutil`); merge the vendored `Truncate` + multi-newline normalizer from `apps/api/internal/strutil/strutil.go` into the same file (FR-019); move `util_test.go` alongside
- [X] T014 [P] [US1] Move `apps/api/internal/jobsources/httpjson.go` → `../jobscraper/httpjson/httpjson.go`: change `defaultClient` init to `&http.Client{Timeout: 30*time.Second}` with nil Transport (break the `retrieval.DefaultTransport` hard reference — R-005); keep `SetDefaultClient`/`DefaultClient`/`GetJSON`/`GetJSONStatus`/`PostJSON`; move `httpjson_test.go` alongside and adjust the test that assumes the retrieval transport
- [X] T015 [P] [US1] Move `apps/api/internal/platform/scraping/` → `../jobscraper/scraping/`: `domain/port.go` becomes `scraper.go` (package `scraping`, expose `Scraper`); `infrastructure/httpscraper.go` becomes `httpscraper.go` (package `scraping`, chromedp impl); drop the `scraping.go` facade file; keep `goquery`/chromedp deps

#### Retrieval engine core (US2 sub-slice, lands here because adapters depend on it)

- [X] T016 [P] [US1] Move `apps/api/internal/retrieval/ladder.go` → `../jobscraper/retrieval/ladder.go` (verbatim, stdlib only)
- [X] T017 [P] [US1] Move `apps/api/internal/retrieval/challenge.go` → `../jobscraper/retrieval/challenge.go` (verbatim, stdlib only — includes `IsChallenged` and `IsRefused`)
- [X] T018 [P] [US1] Move `apps/api/internal/retrieval/outcome.go` → `../jobscraper/retrieval/outcome.go` (verbatim, stdlib only)
- [X] T019 [P] [US1] Move `apps/api/internal/retrieval/identity.go` → `../jobscraper/retrieval/identity.go` (verbatim, stdlib only)
- [X] T020 [P] [US1] Create `../jobscraper/retrieval/service.go` from `apps/api/internal/retrieval/service.go`: keep `FetchRequest`, `FetchResult`, `HostStatus`, `HostPacing`, and the `Service` interface; drop the empty `retrieval.go` stub
- [X] T021 [P] [US1] Move `apps/api/internal/retrieval/direct.go` → `../jobscraper/retrieval/direct.go` (package `retrieval`, keep `bogdanfinn/fhttp` + `bogdanfinn/tls-client` deps); rewire cookie jar to call `StateStorePort.LoadCookies`/`SaveCookies` instead of the concrete `*StateStore`
- [X] T022 [P] [US1] Move `apps/api/internal/retrieval/browser.go` → `../jobscraper/retrieval/browser.go` (package `retrieval`, keep `chromedp` dep)
- [X] T023 [P] [US1] Move `apps/api/internal/retrieval/flaresolverr.go` → `../jobscraper/retrieval/flaresolverr.go` (package `retrieval`, stdlib + `net/http`)
- [X] T024 [US1] Create `../jobscraper/retrieval/engine.go`: the `engineImpl` struct (renamed from `ServiceImpl`), `NewEngine(identity *BrowserIdentity, store StateStorePort, opts EngineOpts) Service` constructor, and the `Fetch`/`HostStatus`/`ClearRungPreference`/`ClearCookies`/`OverrideCoolingOff` methods — copy logic from `apps/api/internal/retrieval/service_impl.go` but replace `*StateStore` with `StateStorePort`, `*config.Config` with `EngineOpts` (plain struct with `BrowserEnabled bool`, `FlaresolverrURL string`, `CheapRungRetestInterval time.Duration`, `CoolingOffThreshold int`, `CoolingOffBaseDuration time.Duration`), and `pgtype.Timestamptz` with `*time.Time`
- [X] T025 [P] [US1] Create `../jobscraper/retrieval/pacing.go` from `apps/api/internal/ratelimit/transport.go`: vendor the `Transport` type (package `retrieval`, keep `golang.org/x/time/rate` dep); expose `NewDefaultTransport(store StateStorePort, overrides map[string]float64) *Transport` and the `RateFor(host)` method (R-001)
- [X] T026 [US1] Move `apps/api/internal/retrieval/transport.go` logic into `../jobscraper/retrieval/pacing.go` (the `NewRateResolver` + `ConfigureDefaultTransport` functions), rewired to `StateStorePort` instead of `*StateStore`; delete `apps/api/internal/retrieval/transport.go` and `apps/api/internal/ratelimit/transport.go` + `transport_test.go` after the move (the app re-imports the library's pacing)

#### The 19 non-board adapters

- [X] T027 [P] [US1] Move `apps/api/internal/jobsources/infrastructure/adapters/adzuna.go` + `_test.go` → `../jobscraper/adapters/`; rewire imports: `dto`→`model`, `jobsources`→`httpjson`+`htmlutil`+`strutil`, `platform/scraping`→`scraping`, `jobsources/domain`→`adapter`
- [X] T028 [P] [US1] Move `arbeitnow.go` + `_test.go` (same rewiring pattern as T027)
- [X] T029 [P] [US1] Move `djinni.go` + `djinni_session.go` + `djinni_searchmode.go` + `djinni_test.go` + `djinni_searchmode_test.go` (same pattern; `djinni.go` also imports `strutil` → `jobscraper/strutil`)
- [X] T030 [P] [US1] Move `dou.go` + `_test.go`
- [X] T031 [P] [US1] Move `glassdoor.go` + `_test.go` (imports `retrieval` → `jobscraper/retrieval`)
- [X] T032 [P] [US1] Move `himalayas.go` + `_test.go`
- [X] T033 [P] [US1] Move `indeed.go` + `_test.go`
- [X] T034 [P] [US1] Move `jobgether.go` + `_test.go` (imports `retrieval`)
- [X] T035 [P] [US1] Move `jobleads.go` + `jobleads_session.go` + `_test.go`
- [X] T036 [P] [US1] Move `jobspy.go` + `_test.go`
- [X] T037 [P] [US1] Move `jooble.go` + `_test.go`
- [X] T038 [P] [US1] Move `manual.go`
- [X] T039 [P] [US1] Move `remoteok.go` + `_test.go`
- [X] T040 [P] [US1] Move `remotive.go` + `_test.go`
- [X] T041 [P] [US1] Move `robota.go` + `_test.go` (imports `strutil`)
- [X] T042 [P] [US1] Move `wellfound.go` + `_test.go` (imports `retrieval`)
- [X] T043 [P] [US1] Move `workua.go` + `_test.go` (imports `strutil`)
- [X] T044 [P] [US1] Move `live_smoke_test.go` → `../jobscraper/adapters/` (keep build tag; it references the moved adapters)
- [X] T045 [P] [US1] Move `apps/api/internal/jobsources/infrastructure/adapters/testdata/` → `../jobscraper/adapters/testdata/`

#### The 6 board adapters + shared helpers

- [X] T046 [US1] Move `atsboard.go` + `atsboard_test.go` + `atsboard_integration_test.go` → `../jobscraper/adapters/`: rewire `roster.Service`→`rosterport.RosterPort`, `sqlcgen.EmployerBoard`→`rosterport.EmployerBoard`, `dto`→`model`, `jobsources/domain`→`adapter`, `dbutil`→ (drop; IDs are strings at the port); keep `runBoardVendor`/`vendorHealthCheck`/`healthCheckEmployer`/`classifyOutcome`/`boardRunState`/`NewBoardAdapters`
- [X] T047 [P] [US1] Move `greenhouse.go` + `_test.go` → `../jobscraper/adapters/`: rewire `Roster *roster.Service`→`Roster rosterport.RosterPort`, `sqlcgen`→`rosterport`, `dto`→`model`, `jobsources`→helpers
- [X] T048 [P] [US1] Move `lever.go` + `_test.go` (same pattern as T047)
- [X] T049 [P] [US1] Move `ashby.go` + `_test.go`
- [X] T050 [P] [US1] Move `workable.go` + `_test.go`
- [X] T051 [P] [US1] Move `smartrecruiters.go` + `_test.go`

#### Library validation

- [X] T052 [US1] Run `cd ../jobscraper && go mod tidy && go build ./...` — fix any import-path issues surfaced (the library should now declare `goquery`, `bogdanfinn/fhttp`, `bogdanfinn/tls-client`, `chromedp/cdproto`, `chromedp/chromedp`, `golang.org/x/time/rate`; NO `pgx`/`asynq`/`viper`/`minio`/`pgvector`/`goose`/`job-finder/api`)
- [X] T053 [US1] Run `cd ../jobscraper && go test ./...` — adapter unit tests pass against moved fixtures (FR-013)

**Checkpoint**: Library compiles standalone, tests pass. Adapters are gone from the app. App is now broken (still references old paths) — US3 rewiring comes next, but the library is the MVP deliverable and is independently valid.

---

## Phase 4: User Story 2 — Retrieval engine reusable without app coupling (Priority: P2)

**Goal**: Verify the engine is importable alone (FR-016) and that a no-DB consumer can drive it.

**Independent Test**: Quickstart scenario 3 (throwaway program imports only `retrieval`, `go.sum` has no `chromedp`/`goquery`).

### Implementation for User Story 2

- [X] T054 [US2] Verify `../jobscraper/retrieval/` has no import of `../jobscraper/adapters/` (run `grep -r 'jobscraper/adapters' ../jobscraper/retrieval/` — must be empty; FR-016)
- [X] T055 [US2] Verify `../jobscraper/adapter/` has no import of `jobscraper/retrieval` or `jobscraper/adapters` (same grep; must be empty so the framework is lightweight)
- [X] T056 [P] [US2] Write `../jobscraper/retrieval/example_test.go` — a `TestEngineWithInMemoryStore` that constructs `NewEngine(nil, &memStore{}, EngineOpts{})`, calls `Fetch` on a URL that returns 403, and asserts the outcome escalates or reports challenged (this is the in-memory/no-op store path from SC-001/US2 acceptance scenario 1)

**Checkpoint**: Retrieval engine confirmed standalone-importable. No code moves here — this phase is validation of the US1 move + one example test.

---

## Phase 5: User Story 3 — Job-finder API behavior unchanged after the split (Priority: P3)

**Goal**: Rewire the app to consume the library; implement the two ports; confirm zero regression.

**Independent Test**: Quickstart scenario 4 (`make test-lint` green, tygo byte-identical, adapter tests pass) + scenario 5 (board adapters through port).

### Implementation for User Story 3

#### App-side StateStorePort implementation

- [X] T057 [P] [US3] Update `apps/api/internal/retrieval/state.go`: `StateStore` now implements `jobscraper/retrieval.StateStorePort`; `Get` returns `*retrieval.HostState` (field-copy from `sqlcgen.HostRetrievalState`, decrypt cookies with `internal/crypto`); `Upsert` accepts `*retrieval.HostState` (field-copy to `sqlcgen.UpsertHostRetrievalStateParams`, encrypt cookies); `LoadCookies`/`SaveCookies` unchanged; keep `FetchAndSetCrawlDelay`/`RecordBlock`/`RecordSuccess`/`ClearRung`/`ClearCookies` (already thin)
- [X] T058 [US3] Replace `apps/api/internal/retrieval/service_impl.go` with a thin constructor: drop the `ServiceImpl` struct + `Fetch`/`tryRung`/`tryDirect`/`tryBrowser`/`tryFlareSolverr`/`rungAvailable`/`recordBlock`/`HostStatus`/`ClearRungPreference`/`ClearCookies`/`OverrideCoolingOff` methods (all moved to the library's `engine.go`); keep only a `NewService(identity, store, cfg) jobscraper.Service` that calls `jobscraper.NewEngine(identity, store, EngineOpts{...from cfg...})` and returns the library's `Service`
- [X] T059 [P] [US3] Replace `apps/api/internal/retrieval/transport.go` with a 3-line wiring helper: `jobscraper.NewDefaultTransport(stateStorePort, cfg.HostRPSOverrides)`; delete `apps/api/internal/ratelimit/` entirely (vendored into the library)

#### App-side RosterPort implementation

- [X] T060 [P] [US3] Update `apps/api/internal/jobsources/roster/service.go`: `Service` now implements `jobscraper/rosterport.RosterPort`; `ListForRun` returns `[]rosterport.EmployerBoard` (field-copy from `sqlcgen.EmployerBoard` via `dbutil.UUIDString`); `RecordRunOutcome`/`GetByVendorAndEmployer`/`InsertEmployerBoard`/`DeleteEmployerBoard`/candidate methods thin wrappers with UUID conversion; keep the discovery/candidate orchestration in `candidates.go` (calls port methods on `Service`)
- [X] T061 [P] [US3] Keep `apps/api/internal/jobsources/roster/view.go` and `candidates.go` app-side (they produce `dto.EmployerBoardDto`/`dto.BoardCandidateDto` — dashboard contracts); they call port methods on `Service` unchanged

#### App-side cleanup of moved files

- [X] T062 [US3] Delete from the app (already moved): `apps/api/internal/jobsources/htmlutil.go`(+test), `httpjson.go`(+test), `util.go`(+test), `apps/api/internal/jobsources/domain/adapter.go`(+test), `apps/api/internal/jobsources/domain/errors.go`, `apps/api/internal/retrieval/{ladder,challenge,outcome,identity,service,direct,browser,flaresolverr}.go`(+tests), `apps/api/internal/platform/scraping/` (whole tree)
- [X] T063 [P] [US3] Update `apps/api/internal/jobsources/domain/job_source.go`: `JobSource.Kind` now `model.SourceKind` (alias is fine); `ToDTO` unchanged (produces `dto.JobSourceDto` which is an alias of `model.JobSourceDto`)
- [X] T064 [US3] Update `apps/api/internal/jobsources/domain/repository.go` and `search_repository.go`: no change (they stay sqlcgen-typed, app-internal); if they referenced `domain.Adapter`/`Registry` (moved), re-import from `jobscraper/adapter` via a re-export alias in `domain/` or update call sites directly
- [X] T065 [US3] Update `apps/api/internal/jobsources/application/service.go` + `search_service.go`: replace `jobsources/domain` adapter-registry imports with `jobscraper/adapter`; keep `domain.Repository`/`SearchRepository` references (app-internal); keep `crypto`/`dbutil`/`db/sqlcgen`/`queue` imports
- [X] T066 [US3] Update `apps/api/internal/jobsources/interfaces/worker/handler.go` + `scheduler.go`: replace adapter-framework imports with `jobscraper/adapter`; keep `queue`/`activity`/`db` imports
- [X] T067 [P] [US3] Update `apps/api/internal/jobsources/interfaces/http/` handlers: replace any `jobsources/domain` adapter-type imports with `jobscraper/adapter`; keep `httpx`/`dto`/`apperr` imports
- [X] T068 [US3] Update `apps/api/cmd/server/` wiring: construct `jobscraper.NewEngine(...)` (or the app's `NewService` wrapper), construct `roster.NewService(q, checkers)` with the `checkers` map from `jobscraper/adapters.NewBoardAdapters()`, inject `RosterPort` into the board adapters at construction

#### arch_test and exemption list

- [X] T069 [P] [US3] Verify `apps/api/internal/arch_test.go` exemption list: `testutil` still in the tree (out of scope), `httpapi`/`httpx`/`health` still present and correct; no new exemption added; confirm no moved package imported `chi` (grep `go-chi/chi` in `../jobscraper/` — empty)

#### App validation

- [X] T070 [US3] Run `cd apps/api && go mod tidy && go build ./...` — fix import-path issues; the app should build against the library via the `replace` directive
- [X] T071 [US3] Snapshot tygo output: `cp packages/shared/src/generated.ts /tmp/generated.before.ts` (if not already snapshotted), then `make tygo-generate`, then `diff /tmp/generated.before.ts packages/shared/src/generated.ts` — must be empty (FR-015/SC-005)
- [X] T072 [US3] Run `cd apps/api && go test ./internal/jobsources/... ./internal/retrieval/...` — adapter + retrieval tests pass unchanged (FR-013/SC-003)
- [X] T073 [US3] Run `make test-lint` — `lint-go` + `lint-web` + `test-go` + `test-react` all green (FR-014/SC-004)

**Checkpoint**: App consumes the library; all tests green; tygo byte-identical; merge gate passes.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final validation, throwaway-consumer proof, cleanup.

- [X] T074 [P] Run quickstart scenario 1: `cd ../jobscraper && grep -E 'pgx|asynq|viper|minio|pgvector|goose|job-finder/api' go.mod` returns empty (SC-002)
- [X] T075 Run quickstart scenario 2: create `/tmp/jobscraper-smoke/` throwaway Go project importing `jobscraper`, construct a registry with `AdzunaAdapter` + `RemoteokAdapter`, call `Search`, print results; verify `go.sum` has no `pgx`/`asynq` (SC-001)
- [X] T076 Run quickstart scenario 3: in the throwaway project, import only `jobscraper/retrieval`, construct `NewEngine` with an in-memory `StateStorePort`, `Fetch` a URL; verify `go.sum` has no `goquery` (SC-006/US2). NOTE: `chromedp` IS present and must be — `retrieval/browser.go` is the browser rung, so the engine depends on chromedp by design (plan.md). Only the adapter/HTML stack (`goquery`) must stay out.
- [X] T077 Run quickstart scenario 5: `cd apps/api && go test ./internal/jobsources/infrastructure/adapters/ -run 'Greenhouse|Lever|Ashby|Workable|SmartRecruiters|ATSBoard'` — all 6 board adapter tests green through `RosterPort` (SC-007)
- [X] T078 [P] Remove now-empty app directories: `apps/api/internal/ratelimit/` (deleted in T059), `apps/api/internal/platform/scraping/` (deleted in T062), `apps/api/internal/retrieval/retrieval.go` (empty stub); confirm `git status` shows deletions only for moved files
- [X] T079 [P] Update `apps/api/internal/arch_test.go` if the `exemptDirs` map references any removed directory (it should not — `testutil` stays — but verify)
- [X] T080 Run `make test-lint` one final time on the merged state (library + app wired) — full green gate before the feature ships

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 (module exists) — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Phase 2 (model + framework + ports in place). This is the bulk of the move.
- **US2 (Phase 4)**: Depends on US1's retrieval move (T016–T026) — validation + one example test only
- **US3 (Phase 5)**: Depends on US1 (library exists and compiles) — app rewiring + port implementations + regression gate
- **Polish (Phase 6)**: Depends on US3 (app wired) — end-to-end validation scenarios

### Within-Phase Ordering

- Phase 2: T004–T010 are all `[P]` (different files); T011 is the compile check that depends on all of them
- Phase 3 helpers (T012–T015): all `[P]`
- Phase 3 retrieval core (T016–T023): all `[P]` except T024 (engine) which depends on T016–T023 existing, and T025–T026 (pacing) which depend on T009 (StateStorePort)
- Phase 3 non-board adapters (T027–T045): all `[P]` — independent files, each only depends on Phase 2 + helpers (T012–T015) + retrieval core (T016–T026) being in place
- Phase 3 board adapters (T046–T051): T046 (atsboard helpers) first; T047–T051 `[P]` after T046
- Phase 5: T057–T061 `[P]` (different files); T062 (delete) after T012–T026/T046–T051; T063–T068 sequential (wiring); T069 `[P]`; T070–T073 sequential (validation)

### Parallel Opportunities

- **Phase 2**: T004–T010 — 7 tasks in parallel (model + framework + 2 ports, all different files)
- **Phase 3 helpers + retrieval core**: T012–T023 — 12 tasks in parallel
- **Phase 3 non-board adapters**: T027–T045 — 19 tasks in parallel (each adapter is an independent file with the same rewiring pattern)
- **Phase 3 board adapters**: T047–T051 — 5 tasks in parallel after T046
- **Phase 5 port implementations**: T057, T060, T061 — 3 tasks in parallel (different files)

---

## Parallel Example: Phase 3 Non-Board Adapters

```bash
# Launch 19 adapter moves in parallel (each is an independent file with the same rewiring pattern):
Task: "Move adzuna.go + _test.go to ../jobscraper/adapters/ and rewire imports"
Task: "Move arbeitnow.go + _test.go to ../jobscraper/adapters/ and rewire imports"
Task: "Move djinni.go + djinni_session.go + djinni_searchmode.go + tests to ../jobscraper/adapters/"
# ... (T027–T045)
```

## Parallel Example: Phase 2 Ports

```bash
# Launch the 7 foundational type/port tasks in parallel:
Task: "Move NormalizedJob/SearchQuery/JobSourceDto to ../jobscraper/model/job.go"
Task: "Move SourceKind to ../jobscraper/model/"
Task: "Add type aliases in apps/api/internal/dto/jobs.go + dto.go"
Task: "Create ../jobscraper/adapter/adapter.go from domain/adapter.go"
Task: "Create ../jobscraper/adapter/errors.go from domain/errors.go"
Task: "Create ../jobscraper/retrieval/state_port.go (HostState + StateStorePort)"
Task: "Create ../jobscraper/rosterport/roster_port.go (EmployerBoard + RosterPort)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (library skeleton)
2. Complete Phase 2: Foundational (model + framework + ports) — CRITICAL, blocks everything
3. Complete Phase 3: US1 — move all helpers, retrieval core, and 25 adapters; validate `go build ./...` + `go test ./...` in the library
4. **STOP and VALIDATE**: Quickstart scenario 1 (library standalone) — the library is the MVP deliverable

### Incremental Delivery

1. Setup + Foundational → skeleton + ports ready
2. US1 → library compiles + tests pass standalone (MVP — the library exists and is usable by a new consumer)
3. US2 → confirm retrieval-imports-without-adapters (validation only, 1 example test)
4. US3 → app rewired to consume the library; full regression gate green
5. Polish → throwaway-consumer proof, cleanup, final `make test-lint`

### Parallel Team Strategy

With multiple developers after Foundational completes:
- Developer A: retrieval core (T016–T026) — the one sequential chain
- Developer B: 19 non-board adapters (T027–T045) — mechanical, parallel
- Developer C: 6 board adapters (T046–T051) — after T046, the 5 are parallel
- Then one developer takes US3 rewiring while others do US2 validation + Polish

---

## Notes

- `[P]` tasks = different files, no dependencies on incomplete tasks
- `[Story]` label maps task to specific user story for traceability
- US1 is the MVP — the library must compile and test green standalone before US3 rewiring begins
- US3 is the regression guardrail — it cannot be skipped, or the app is broken
- The 19 non-board adapter moves (T027–T045) are the most parallelizable work: same pattern, independent files
- Board adapters (T046–T051) carry the highest risk — they touch the `RosterPort` boundary; validate with scenario 5
- No new migrations, no DB schema changes, no new goose version (constitution: migration versions unique/sequential)
- Commit after each task or logical group; stop at any checkpoint to validate independently