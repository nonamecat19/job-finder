# Feature Specification: Extract Job Scraper Library

**Feature Branch**: `043-extract-job-scraper-lib`

**Created**: 2026-08-10

**Status**: Draft

**Input**: User description: "Split the project (api). Create reusable libraries, not have it all in project. Only jobs scrape — the huge chunk of functionality can be moved to another repo as library."

## Clarifications

### Session 2026-08-10

- Q: Board adapters that both scrape and persist — fallback if the split is hard? → A: C — Move all 6 board adapters whole (including roster concerns) into the library; the library defines a `RosterPort` interface the app implements. The library stays DB-free but owns the full adapter surface including roster wiring.
- Q: Where does `strutil` live for the library? → A: B — The library vendors its own copy of the ~30 lines of string helpers it needs. Zero coupling to the app; trivial duplication of stable functions.

## User Scenarios & Testing *(mandatory)*

<!--
   The extraction is an internal-architecture refactor, not an end-user-facing
   feature. "Users" here are: (a) the maintainer who develops the scraper lib and
   the API side-by-side, and (b) a hypothetical second consumer of the library
   (another job product, a one-off research script) that wants the scraping
   capability without adopting this repo's DB, queue, or config stack.
-->

### User Story 1 - Scraping capability is consumable as a standalone library (Priority: P1)

A developer wants to scrape job postings from the supported sources (Adzuna, Djinni, Greenhouse, Indeed, Lever, Work.ua, etc.) from a brand-new Go project, without cloning this repository, without a Postgres/Redis dependency, and without a job-finder-specific configuration file. They add the scraper library as a normal module dependency, construct an adapter registry, call `Search` with a query, and receive normalized job postings. Nothing about job-finder's database schema, queue, encryption keys, or observability stack is required for this to work.

**Why this priority**: This is the entire point of the extraction. If the library cannot be consumed standalone, the split has not actually happened — it has only moved code around inside the monolith. P1 because it defines the success boundary: the rest of the work is in service of this.

**Independent Test**: Create a throwaway Go program outside this repo that imports the library, scrapes one API-based source (e.g. Adzuna) and one HTML-based source (e.g. Djinni) with no database and no Redis, and prints normalized jobs. If it compiles and returns jobs, the story is met.

**Acceptance Scenarios**:

1. **Given** a new empty Go module with only the scraper library as a dependency, **When** the developer constructs a registry with two adapters and calls Search on each, **Then** both return `[]NormalizedJob` without any reference to job-finder's DB types, queue types, or config.
2. **Given** the library's `go.mod`, **When** inspected, **Then** it declares no dependency on `pgx`, `asynq`, `viper`, `minio`, `pgvector`, `goose`, or job-finder's own packages.
3. **Given** the existing job-finder API, **When** built against the extracted library via a local `replace` directive, **Then** all existing adapter behavior (search, posting-read, health check, board scraping) works unchanged against the same fixtures and live tests.

---

### User Story 2 - Scraping engine (retrieval ladder) is reusable without app coupling (Priority: P2)

A developer wants the multi-strategy fetch engine — direct TLS-client request, escalating to headless browser, escalating to FlareSolverr, with challenge detection and per-host pacing — available as a library primitive, decoupled from job-finder's persistence of host state. They can supply their own state store (or none) and their own rate-limit overrides, and the engine still climbs the ladder and reports which rung succeeded.

**Why this priority**: The retrieval engine is the highest-value reusable subcomponent and is currently tangled into job-finder's DB-backed state store and config. Extracting it separately from the adapters keeps the library cohesive (adapters depend on the engine, not the other way around) and makes the engine independently useful for non-job scraping.

**Independent Test**: From a separate Go program, construct the retrieval `Service` with an in-memory or no-op state store, fetch a URL that triggers a 403, and observe the engine escalate from `direct` to `browser` (or the next configured rung) and report the outcome — with no Postgres connection anywhere in the call path.

**Acceptance Scenarios**:

1. **Given** the library's retrieval package, **When** a caller provides a state-store implementation satisfying a library-defined interface (Get/Put host state, no sqlcgen types), **Then** the engine reads and writes rung preference, crawl delay, and block timestamps through that interface.
2. **Given** the retrieval package's core types (`RetrievalMethod`, `PageOutcome`, `BrowserIdentity`, `IsChallenged`), **When** compiled, **Then** they carry no import of job-finder's `config`, `db`, `crypto`, or `ratelimit` packages — those dependencies live behind app-supplied implementations.
3. **Given** a host that returns a challenge page on the direct rung, **When** `Fetch` is called, **Then** the engine escalates to the next available rung and returns a `FetchResult` whose `Outcome` records which rung succeeded or that all rungs were exhausted.

---

### User Story 3 - Job-finder API's scraping behavior is unchanged after the split (Priority: P3)

After the extraction, the job-finder API continues to scrape every source it scraped before, with identical observable behavior: the same adapters are registered, the same per-host pacing applies, the same cookies are stored encrypted, the same saved-search and subscription ingestion pipeline runs, and the existing HTTP endpoints return the same shapes. The only change is that the adapter and retrieval code now lives in an imported library, and the app wires its DB/config/crypto into library-defined ports.

**Why this priority**: A refactor that regresses behavior is worse than no refactor. P3 because it is a guardrail, not the goal — but it must hold or the extraction is not shippable.

**Independent Test**: Run the existing adapter unit tests and the `live_smoke_test` against the library-backed app and confirm no new failures. Run `make test-lint` and confirm both `lint-go` and `test-go` pass.

**Acceptance Scenarios**:

1. **Given** the extracted library wired into the API via `replace`, **When** the existing `apps/api` adapter tests run, **Then** they pass without modification to test fixtures or assertions.
2. **Given** the app's `retrieval/service_impl.go` and `state.go` (which stay in the app), **When** the retrieval state store reads/writes encrypted cookies, **Then** encryption still uses the app's `crypto` package and `CONFIG_ENCRYPTION_KEY` exactly as before — the library never sees the key.
3. **Given** the app's DTO types re-exported as aliases of the library's model types, **When** the dashboard's tygo-generated `packages/shared` types are regenerated, **Then** the generated TypeScript is byte-identical to pre-extraction (the JSON shape did not change).

---

### Edge Cases

- What happens when a board adapter (Greenhouse/Lever/Ashby/Workable/SmartRecruiters/ATSBoard) scrapes *and* persists employer boards? The whole adapter moves to the library; the library defines a `RosterPort` interface (library-owned types) and the board adapters call through it. The app implements `RosterPort` against its existing `roster.Repository`. No board adapter is split — they all move whole, with their persistence calls retargeted from `sqlcgen` types to the port.
- What happens to `jobsources/httpjson.go`'s hard reference to `retrieval.DefaultTransport`? This coupling must be broken before `httpjson` can move: the default client must be injectable from the app, not imported from the retrieval package's package-level variable.
- What happens to the `arch_test.go` exemption list (`httpx`, `httpapi`, `health`, `testutil`) when `testutil` leaves the tree? The exemption list shrinks; no new exemption is needed for the library because the library lives outside `internal/`.
- What happens to in-flight feature work that touches `jobsources/` adapters while the extraction is mid-flight? The extraction should land on `master` in a state where the adapters are imported from the library; concurrent feature branches rebase onto the post-extraction `master` and resolve the import-path changes mechanically.
- What happens to the `dto` package after `NormalizedJob`, `SearchQuery`, `SourceKind`, and `JobSourceDto` move to the library? They are re-exported from `dto` as type aliases so every existing importer in the app and the tygo generator sees no change.
- What happens if a second consumer wants only the retrieval engine and none of the adapters? The library's internal structure must make `retrieval` importable without pulling in the `adapters` package (no import cycle, no heavyweight `adapters` transitive deps from `retrieval`).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The scraping capability (adapter framework + the 25 site adapters) MUST be consumable as a standalone Go module importable by any Go project, with no dependency on job-finder's `apps/api` packages.
- **FR-002**: The library's `go.mod` MUST NOT declare dependencies on `pgx`, `asynq`, `viper`, `minio`, `pgvector`, `goose`, or any `github.com/job-finder/api/internal/*` path.
- **FR-003**: The library MUST expose the job model types (`NormalizedJob`, `SearchQuery`, `SourceKind`, `JobSourceDto`) in its own package; the app's `dto` package MUST re-export them as type aliases so existing app code and the tygo generator are source-compatible.
- **FR-004**: The library MUST expose the adapter framework (`Adapter`, `PostingReader`, `Credentialed`, `DetailNeeder`, `Registry`, adapter errors) decoupled from job-finder's `dto` and `sqlcgen` — the framework references only the library's own model types.
- **FR-005**: The library MUST include the HTML helpers (`HTMLToText`, `SelectionText`) and the JSON HTTP client (`GetJSON`/`PostJSON`) as internal packages, with no dependency on job-finder's `retrieval` package; the default HTTP client MUST be injectable from the consuming app rather than hard-wired to a package-level transport.
- **FR-006**: The library MUST include the retrieval engine: the retrieval-ladder abstraction (`RetrievalMethod`/rung ordering), challenge detection (`IsChallenged`), page outcome types, browser identity, the three rung implementations (direct TLS-client, headless browser, FlareSolverr), and the `Service` interface.
- **FR-007**: The retrieval engine's `Service` interface MUST define a state-store port using library-owned types (not `sqlcgen`/`pgtype`), so a consumer can implement the port against any backend or with an in-memory/no-op store.
- **FR-008**: The retrieval engine MUST NOT import job-finder's `config`, `db`, `crypto`, or `ratelimit` packages; per-host rate limiting MUST be either a library-internal concern or injectable by the consumer.
- **FR-009**: The library MUST include the `platform/scraping` Scraper port (`Scraper` interface + chromedp-backed `HTTPScraper`), with no project-internal imports.
- **FR-010**: The 6 board adapters (Greenhouse, Lever, Ashby, Workable, SmartRecruiters, ATSBoard) MUST move whole into the library, including their roster/board-candidate concerns. The library MUST define a `RosterPort` interface (library-owned types, not `sqlcgen`/`pgtype`) covering the operations the board adapters need (list/insert/delete employer boards, list/insert/decide board candidates, record run outcomes, list apply URLs for discovery). The app MUST implement `RosterPort` against its `roster.Repository` and inject it at construction time. The library stays free of `pgx`/`sqlcgen`/`pgtype` dependencies — all persistence goes through the port.
- **FR-011**: The job-finder API MUST retain the application services (`Service`, `SearchService`, `ingest`), the repository interfaces (sqlcgen-typed), the `roster` package (as the `RosterPort` implementation), the HTTP interfaces, the worker handler and scheduler, and the retrieval state implementation (`service_impl.go`, `state.go`, `transport.go`) — these are app-specific and stay. The `roster` package no longer owns adapter logic; it implements the library's `RosterPort` against the DB.
- **FR-012**: The app's retrieval state implementation MUST continue to encrypt stored cookies with the app's `crypto` package and `CONFIG_ENCRYPTION_KEY`; the library MUST NOT see or import the encryption key.
- **FR-013**: Existing adapter unit tests and the `live_smoke_test` MUST pass unchanged against the library-backed app; no test fixture or assertion may be modified to accommodate the move.
- **FR-014**: `make test-lint` (`lint-go` + `lint-web` + `test-go` + `test-react`) MUST pass after the extraction.
- **FR-015**: The tygo-generated `packages/shared/src/generated.ts` MUST be byte-identical before and after the extraction (the JSON shapes exposed to the dashboard did not change, because the moved types are re-exported as aliases, not reshaped).
- **FR-016**: The library's internal package structure MUST allow importing the retrieval engine without transitively importing the adapters package, so a consumer can use only the engine.
- **FR-017**: The library MUST NOT depend on the `dto` package; references that previously pointed at `dto.NormalizedJob`/`dto.SearchQuery`/`dto.SourceKind` MUST point at the library's own model package instead.
- **FR-018**: The `arch_test.go` exemption list in `apps/api/internal/` MUST be updated if any exempted package leaves the tree (`testutil` is a candidate if extracted separately); no new exemption may be added to accommodate the library.
- **FR-019**: The library MUST vendor its own copy of the string helpers it needs (`Truncate`, multi-newline normalization, any others referenced by moved adapters) in an internal package; it MUST NOT import the app's `internal/strutil`. The app's `strutil` stays in the app unchanged.

### Key Entities *(include if feature involves data)*

- **Library model package**: the canonical home for `NormalizedJob`, `SearchQuery`, `SourceKind`, `JobSourceDto` after extraction. Owned by the library, re-exported as aliases by the app's `dto`.
- **Adapter framework (library)**: `Adapter`, `PostingReader`, `Registry` and supporting interfaces, referencing only library model types.
- **Retrieval Service (library)**: the `Fetch`/`HostStatus` interface and the ladder/challenge/identity/rungs machinery, behind a consumer-supplied state-store port.
- **State-store port (library, implemented by app)**: the interface the retrieval engine reads/writes host state through; the app's `StateStore` (DB-backed, crypto-encrypting) satisfies it.
- **RosterPort (library, implemented by app)**: the interface the 6 board adapters call through for employer-board and board-candidate persistence; the app's `roster.Repository` satisfies it. Library-owned types, no `sqlcgen`/`pgtype`.
- **App retention boundary**: the application services, repository interfaces, roster, HTTP interfaces, worker handler/scheduler, and `service_impl`/`state`/`transport` that stay in the app and wire the library into job-finder's DB, queue, and config.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A new empty Go project importing only the scraper library can scrape at least one API-based source and one HTML-based source and print `NormalizedJob` results, with zero references to job-finder's `apps/api` packages, in under one hour of setup.
- **SC-002**: The library's `go.mod` declares no dependency on `pgx`, `asynq`, `viper`, `minio`, `pgvector`, `goose`, or any `github.com/job-finder/api/internal/*` path — verifiable by reading the file.
- **SC-003**: Every existing `apps/api` adapter unit test passes against the library-backed app with no change to test fixtures or assertions.
- **SC-004**: `make test-lint` passes (both `lint-go` and `lint-web` green, both `test-go` and `test-react` green) after the extraction.
- **SC-005**: The tygo-generated `packages/shared/src/generated.ts` is byte-identical before and after extraction, confirming no JSON contract drift reached the dashboard.
- **SC-006**: A consumer can import and use the retrieval engine without the library transitively pulling in the `adapters` package or any site-specific scraping code.
- **SC-007**: All 25 site adapters are present in the library with no behavioral regression against their pre-extraction unit tests; the 6 board adapters move whole into the library and call through a library-defined `RosterPort` that the app implements against its `roster.Repository`.

## Assumptions

- The extraction targets the `apps/api` Go scraping stack only; the React dashboard, `packages/shared`, and all non-scraping API features (matching, generation, outreach, salary, coach, etc.) are out of scope and must not be modified beyond mechanical import-path changes.
- The library is published as a separate Go module/repo; during development it is wired into the app via a `go.mod` `replace` directive pointing at a local path, and the replaces are dropped to publish.
- The library is Go-only; no TypeScript types are generated from the library itself. Cross-language contracts continue to flow through the app's `dto` → tygo → `packages/shared` chain, which is unchanged because the moved types are re-exported as aliases.
- The existing 25 adapters' scraping behavior is the baseline; this extraction does not add, remove, or rewrite adapters, only relocates them and adjusts their imports to point at library packages.
- The `strutil`, `crypto`, `ratelimit`, `apperr`, `httpx`, `dbutil`, and `testutil` packages are out of scope for extraction as shared modules. `strutil` is vendored into the library as a small internal copy (the ~30 lines the adapters use); the app keeps its own `strutil` unchanged. `ratelimit` either is vendored similarly or is injected by the app via the retrieval transport wiring — decided during planning. The app's `crypto` stays in the app and the library never imports it (FR-012).
- The app's existing config loading (`internal/config`) stays in the app; the library does not read `viper` config. Any retrieval configuration the library needs (rung enablement, identity version) is passed in as plain values by the app at construction time.
- Concurrent in-flight features that touch `jobsources/` adapters rebase onto post-extraction `master` and resolve import-path changes mechanically; the extraction is not sequenced around in-flight work.
- The `arch_test.go` "no chi outside `interfaces/http/`" rule is unaffected by the extraction because the library lives outside `apps/api/internal/`; only the exemption list may shrink if `testutil` is later extracted (not part of this feature).