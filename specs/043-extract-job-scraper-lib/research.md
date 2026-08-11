# Phase 0: Research

## R-001: Where does `ratelimit` live for the library?

**Decision**: Vendor a minimal copy of `ratelimit.Transport` into the library's `retrieval` package (internal subpackage `retrieval/pacing`), and have `transport.go` (app side) wire the library's transport to the app's state store. The library owns the `golang.org/x/time/rate` dependency; the app no longer imports its own `internal/ratelimit` after the move — the app's `transport.go` becomes a thin wiring layer that calls `jobscraper/retrieval.NewDefaultTransport(stateStorePort, overrides)`.

**Rationale**: The app's `internal/ratelimit` is only used by `retrieval/transport.go` and one test. Moving it into the library keeps the rate-limiting concern co-located with the rungs that need it, removes one more app-internal package the library would otherwise have to duplicate, and matches the spec's Assumptions note ("ratelimit either is vendored similarly or is injected by the app via the retrieval transport wiring"). The ~120-line `Transport` is self-contained (only `golang.org/x/time/rate` + stdlib).

**Alternatives considered**:
- *Extract `libs/ratelimit` as its own module* — rejected: out of scope for this feature; would add a third module to coordinate.
- *App injects the whole `http.RoundTripper`* — rejected: the rate resolver reads from the state store on every request; injecting only the transport loses the per-host dynamic resolution that is the whole point of the pacing layer.

## R-002: Module path and physical location for the library

**Decision**: Module path `github.com/job-finder/jobscraper` (sibling repo, not a subdir of `apps/api`). During development, the app's `go.mod` carries `replace github.com/job-finder/jobscraper => ../jobscraper` (or `../../jobscraper` depending on workspace layout). The library is flat (no `internal/`): `jobscraper/model`, `jobscraper/adapter`, `jobscraper/retrieval`, `jobscraper/adapters`, etc., all importable by consumers.

**Rationale**: The library is intended for reuse by other consumers (SC-001). A sibling repo is the cleanest publication unit and lets the `replace` directive drop without any tree restructuring. A flat (non-`internal/`) layout is required because consumers must reach `retrieval` without pulling `adapters` (FR-016) — `internal/` would make `adapters` unimportable from outside the module, which is the opposite of the goal.

**Alternatives considered**:
- *Subdirectory of this repo (`libs/jobscraper/`)* — viable for development, but the spec's Assumptions say "published as a separate Go module/repo"; a subdir with a `replace` is the dev-time equivalent and the migration to a sibling repo is a path rename. Pick the sibling-repo shape now to avoid a second move.
- *Monorepo sub-module with Go workspace (`go.work`)* — rejected: `go.work` does not play well with the publish flow; `replace` in `go.mod` is simpler and matches the spec.

## R-003: `StateStorePort` shape — how much of `StateStore` does the engine need?

**Decision**: The port exposes exactly the 7 methods the engine calls in `service_impl.go`: `Get`, `Upsert`, `FetchAndSetCrawlDelay`, `RecordBlock`, `RecordSuccess`, `ClearRung`, `ClearCookies`. Plus `LoadCookies`/`SaveCookies` used by the direct rung's cookie jar. The port's `Get` returns a library-owned `HostState` struct (not `sqlcgen.HostRetrievalState`), with fields: `IdentityVersion string`, `CurrentRung string`, `RungLastVerifiedAt *time.Time`, `Cookies []byte`, `ConsecutiveBlocks int32`, `CoolingOffUntil *time.Time`, `LastBlockAt *time.Time`, `LastBlockReason *string`, `CrawlDelaySeconds *int32`. The app's `StateStore` adapts between `sqlcgen.HostRetrievalState` and `jobscraper.HostState` and keeps the crypto encryption/decryption on its side (FR-012).

**Rationale**: This is the exact surface `service_impl.go` touches today (verified by reading every `s.store.*` call). Defining the port over a library-owned struct keeps `pgx`/`pgtype` out of the library. The app's adapter is a thin field-copy; the cookie encryption stays in the app because the key never leaves the app.

**Alternatives considered**:
- *Port takes `any` and the app casts* — rejected: loses type safety, defeats the "typed contracts" constitution principle.
- *Split into two ports (Read + Write)* — rejected: every caller that reads also writes in the same flow; one port is simpler and matches the current cohesion.

## R-004: `RosterPort` shape — what operations do the 6 board adapters need?

**Decision**: The port exposes the 6 methods the board helpers (`runBoardVendor`, `vendorHealthCheck`, `healthCheckEmployer`, `NewBoardAdapters`) and the roster `Service` actually call: `ListForRun(ctx, vendor) []EmployerBoard`, `RecordRunOutcome(ctx, employerID string, postingCount int)`, `GetByVendorAndEmployer(ctx, vendor, identifier) (EmployerBoard, error)`, `InsertEmployerBoard(ctx, params) (EmployerBoard, error)`, `GetBoardCandidate`/`InsertBoardCandidate`/`DecideBoardCandidate` (for discovery), `ListApplyURLsForDiscovery(ctx, limit) []string`. Library-owned types `EmployerBoard` and `BoardCandidate` are plain structs with the fields the adapters read (`ID string`, `Vendor string`, `EmployerIdentifier string`, `DisplayName string`, `ConsecutiveEmptyRuns int`, etc.). The app's `roster.Service` becomes the port implementation; `roster.View` and `roster.candidates` stay app-side because they produce `dto.EmployerBoardDto` (a dashboard contract, not a library concern).

**Rationale**: Verified by reading `atsboard.go` (the shared board helpers) + `roster/service.go` + `roster/candidates.go`. The board adapters only call `rosterSvc.ListForRun` and `rosterSvc.RecordRunOutcome` directly; the discovery/candidate flow is used by the HTTP layer (`roster.View`) and the scheduler, not by the adapters themselves — but those callers also want to go through the port for consistency, so the port covers the full roster surface. The `EmployerHealthChecker` function type moves to the library (it's referenced by `NewBoardAdapters`'s return signature).

**Alternatives considered**:
- *Two ports: one for adapters (2 methods), one for the app's discovery flow (the rest)* — rejected: the app's `roster.Service` implements both anyway; one port is simpler and the discovery callers are in the same app.
- *Move `roster.View` into the library* — rejected: `roster.View` produces `dto.EmployerBoardDto` (a dashboard contract). It stays app-side.

## R-005: `httpjson` transport injection — how does the library avoid importing retrieval?

**Decision**: `httpjson`'s `defaultClient` starts as a plain `&http.Client{Timeout: 30*time.Second}` with a nil `Transport`. The app, at startup, calls `httpjson.SetDefaultClient(&http.Client{Transport: jobscraper.RetrievalTransport, ...})` — wiring the rate-limited transport from the outside. The library never imports `retrieval` from `httpjson`. Adapters that need the retrieval-aware client use `DefaultClient()` which returns the wired one.

**Rationale**: Today `httpjson.go:18` hard-wires `Transport: retrieval.DefaultTransport`, which is the one line that blocks extraction (spec Edge Cases). Making the default injectable preserves behavior (the app still sets the same transport) while cutting the import. `SetDefaultClient` already exists in the current code — this decision just removes the initialization-time coupling to the `retrieval` package's package-level variable.

**Alternatives considered**:
- *Pass the client to every `GetJSON` call* — rejected: every adapter call site would change; the spec requires no adapter call-site changes (FR-013).
- *Keep a package-level var but init it from an `init()` in the library* — rejected: `init()` order is fragile and the library has no way to know the right transport at init time.

## R-006: `dto` re-export mechanism — aliases vs. wrapper types

**Decision**: Use Go type aliases (`type NormalizedJob = jobscraper.NormalizedJob`) in `apps/api/internal/dto/jobs.go`. Aliases are identity-equal to the library types, so the tygo generator sees the same Go types with the same JSON tags and produces byte-identical TypeScript (FR-015/SC-005). `SourceKind` becomes `type SourceKind = jobscraper.SourceKind` and its `const` block re-declares the aliases against the library's underlying string type.

**Rationale**: Type aliases are the Go-native way to re-export without duplication. tygo reads the Go type and its struct tags; an alias resolves to the same struct, so the generated TS is unchanged. Wrapper types (`type NormalizedJob struct { jobscraper.NormalizedJob }`) would change the Go type identity and break the `Adapter.Search` signature compatibility.

**Alternatives considered**:
- *Move tygo generation to the library* — rejected: the dashboard's shared types include many non-scraping DTOs (`ApplicationDto`, `MatchResultDto`, …); tygo stays in the app over the app's `dto` package.

## R-007: `arch_test.go` exemption list impact

**Decision**: The `arch_test.go` exemption list (`httpapi`, `httpx`, `health`, `testutil`) is unchanged by this feature. The moved packages (`jobsources/httpjson`, `jobsources/htmlutil`, `jobsources/util`, `platform/scraping`, `retrieval`, `jobsources/domain/adapter.go`) are all outside `internal/httpapi`/`httpx`/`health`/`testutil`, and the library lives outside `apps/api/internal/` entirely. No new exemption is added. The note in spec FR-018 about `testutil` leaving the tree is out of scope for this feature.

**Rationale**: The `arch_test.go` rule is "no `chi` imports outside `interfaces/http/`". The library does not import `chi` (verified: no adapter or retrieval file imports `go-chi/chi`). The exemption list only needs updating if an exempted package leaves, which this feature does not trigger.

**Alternatives considered**: none — the rule is unaffected.

## R-008: Board adapter helpers — do `runBoardVendor`/`vendorHealthCheck`/`boardRunState`/`classifyOutcome` move too?

**Decision**: Yes — `atsboard.go` (the shared board-adapter helpers) moves whole into `jobscraper/adapters/`. It defines `runBoardVendor`, `vendorHealthCheck`, `healthCheckEmployer`, `classifyOutcome`, `boardRunState`, and `NewBoardAdapters` — all of which reference `roster.Service` and `domain.EmployerRunOutcome`. After the move, they reference `rosterport.RosterPort` and `adapter.EmployerRunOutcome` (both library-owned). The `employerFetcher` type alias moves too.

**Rationale**: These helpers are the glue the 5 board adapters (Greenhouse/Lever/Ashby/Workable/SmartRecruiters) call into. They are part of the board-adapter surface and must move with them (FR-010). `NewBoardAdapters` returns the adapters plus a `map[string]roster.EmployerHealthChecker` — that map is consumed by the app's `roster.NewService(q, checkers)` at construction time, so the app still wires it.

**Alternatives considered**:
- *Leave the helpers in the app* — rejected: would force the 5 board adapters to import back into the app for the helper, breaking FR-001 (standalone library).