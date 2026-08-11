# Domain: Codebase Structure

Consolidates **027** HTTP handler decomposition, **024** agent-context and shared-type
consolidation, **043** the scraper-library extraction.

Implementation: `apps/api/internal/*/interfaces/http/`, `apps/api/internal/httpapi/`,
`apps/api/internal/httpx/`, `apps/api/.golangci.yml`, `apps/api/internal/arch_test.go`,
`packages/shared/src/`, `apps/api/go.mod`. How it works:
[`docs/architecture/component-map.md`](../../docs/docs/architecture/component-map.md),
[`docs/backend/http-api.md`](../../docs/docs/backend/http-api.md),
[`docs/frontend/shared-types.md`](../../docs/docs/frontend/shared-types.md).

These rules are **mechanically enforced**. They are not style preferences, and a change that
breaks one fails CI rather than review.

---

## 1. Feature-module layout (027)

Each feature owns its own vertical slice: `domain/`, `application/`, `infrastructure/`,
`interfaces/http/`, `interfaces/worker/`.

| # | Rule |
|---|---|
| 027-FR-001 | A feature's request handling lives in that feature's own module, under the same adapter layer that already holds its background workers. |
| 027-FR-002 | A feature without an adapter layer gains one in the same arrangement — the layout is uniform, with no feature left flat (027-SC-006). |
| 027-FR-003 | `internal/httpapi` retains **only** route assembly, cross-cutting request behaviour, and shared response handling. |
| 027-FR-004 | `internal/httpapi` depends on **zero** feature modules (027-SC-001: down from twenty-four). |
| 027-FR-005 | Each feature contributes routes through the existing variadic registration on `httpapi.NewRouter(...)`, wired in `apps/api/cmd/server/`. Adding a route never edits `router.go`. |
| 027-FR-007 | Versioned and unversioned mounts are produced from a **single** registration per feature. |
| 027-FR-008 | Response and error writing goes through the shared helpers in `internal/httpx` — `WriteJSON`, `WriteError`, `WriteAppError`, `DecodeJSON` — never duplicated per feature. |
| 027-FR-012 | A feature's HTTP adapter never depends on another feature's internals. Cross-feature needs go through that feature's own public surface. |

**Enforcement** (027-FR-010, 027-FR-011): the `depguard` rules in `apps/api/.golangci.yml`
and `apps/api/internal/arch_test.go` fail a change that adds a dependency from the shared
routing package to a feature, or that places feature request handling outside the feature's
adapter layer. 027-SC-005: a deliberately introduced violation is rejected 100% of the time.

**Non-negotiable during the move** (027-FR-006, 027-FR-009, 027-FR-014): routes, methods,
request/response shapes, status codes and error format were unchanged; endpoint tests moved
with their feature and kept running; the work landed one feature at a time with the system
fully working after each (027-SC-003 verified by automated comparison, 027-SC-004 by the
unmodified e2e suite).

**Bar** (027-SC-002): adding an endpoint to an existing feature edits files in exactly one
feature directory, plus at most one registration line.

027-FR-013: contributor and agent documentation must describe this arrangement and must
match what the automated check enforces. `AGENTS.md` § Conventions is that description.

### 1.1 The two enforcement mechanisms

One is not enough, and the reason is precise: **`depguard` matches import paths, not file
locations.** A handler at `internal/jobs/handler.go` — inside its feature module but outside
`interfaces/http` — violates 027-FR-011 and passes every import rule. The placement test
closes that gap.

**`depguard`** lives in the existing `apps/api/.golangci.yml`, so `make lint-go` and the
`lint-go` CI job already run it — no new tooling, no new job. It is **not** in the `standard`
linter set, so it must sit in an explicit `enable` list alongside the existing
`default: standard` / `disable: [errcheck]`.

| Rule | Denies | Prevents |
|---|---|---|
| `router-is-not-a-library` | Importing `internal/httpapi` from anywhere except `cmd/server/**` and `httpapi` itself | A new handler being added to `httpapi`, **and** a feature importing the router — 027-FR-004 in both directions |
| `httpx-stays-a-leaf` | `internal/httpx` importing anything under `internal/` | The helper package accreting dependencies and reintroducing cycle risk. It is imported by every feature's adapter |
| `no-cross-feature-internals` | Files under `internal/*/interfaces/**` importing another feature's `infrastructure` | An adapter reaching past another feature's boundary (027-FR-012) |

**The placement test** is `apps/api/internal/arch_test.go` (package `internal_test`), running
under `go test ./...` and therefore under `make test-lint`. It walks `internal/`, parses
imports with `go/parser` in `ImportsOnly` mode, and asserts that any file importing
`github.com/go-chi/chi/v5` lives inside an `interfaces/` package. `internal/httpapi` is exempt
because it *is* the router; `internal/httpx` is exempt only so the two mechanisms do not report
the same violation twice.

A test rather than a linter plugin because golangci-lint has no built-in "file location implies
allowed imports" check, and a custom plugin needs building, pinning and distributing. The test
is ~20 lines and debuggable with `go test -run`.

> **A rule never observed failing has not been tested.** All three `depguard` rules and the
> placement test must be seen *rejecting* a deliberate violation before being trusted — a
> `depguard` rule that silently matches nothing is indistinguishable from a passing build.
>
> ```sh
> printf 'package http\nimport _ "github.com/job-finder/api/internal/httpapi"\n' \
>   > internal/jobs/interfaces/http/violation.go
> make lint-go   # must fail, naming internal/httpapi and the depguard rule
> rm internal/jobs/interfaces/http/violation.go
> ```

**Two deviations forced by what `depguard` v2.12.2 actually does**, recorded because both look
like mistakes otherwise:

1. **`no-cross-feature-internals` enumerates the feature `infrastructure` packages instead of
   globbing them.** `depguard` matches `deny[].pkg` by **prefix, not glob**, so the intended
   `…/internal/*/infrastructure` matched nothing and the rule would have shipped silently
   dead. Confirmed empirically: with the glob in place, a deliberate cross-feature
   `infrastructure` import produced `0 issues`. `platform/*` is deliberately excluded — it is
   shared infrastructure, not a feature.
2. **`httpx-stays-a-leaf` allows `internal/apperr`.** `WriteAppError` renders an
   `*apperr.Error` and cannot do so without naming the type. `apperr` imports only `fmt` and
   `net/http`, so a leaf importing a leaf carries no cycle risk, and the alternative — moving
   HTTP rendering into `apperr` — would drag an HTTP package into every domain layer that
   reports errors. The rule keeps its deny on all of `internal/`; `apperr` is a single named
   `allow`.

**Limitations, stated rather than discovered later:**

- `no-cross-feature-internals` cannot distinguish a legitimate cross-feature port from an
  illegitimate reach-in. A feature importing another feature's `application` package still
  passes — **that is deliberate**, since `application` is the exported boundary 027-FR-012
  permits.
- Rule 3 matches by path glob, so a feature not following the `internal/<feature>/interfaces/`
  layout is not covered at all. **027-FR-002's "every module adopts the layout" is therefore
  load-bearing for enforcement, not cosmetic.**
- The placement test is keyed on the chi import, so a handler written against `net/http` alone
  would not be caught. Accepted: every handler mounts via chi, and one that never touches the
  router cannot be registered.
- These rules are **additive only.** They prevent regression; they do not verify the original
  move was complete. 027-SC-006 was checked by an inventory audit, not by the linter.
- **Verify glob syntax against the pinned golangci-lint version** before relying on it.
  `depguard`'s `files` patterns and `list-mode` semantics have changed across major versions.

### 1.2 What the move actually was

23 handler files in `internal/httpapi`, measured at `ede4b90` (2026-07-30), moved in three
waves: six `dto`-only handlers (verbatim moves), fifteen with one or two feature dependencies,
and one — `roster` — that needed real work first, because it reached into `db/sqlcgen` and
`dbutil` directly and had to go behind `jobsources/roster`'s boundary before it could move.
`depguard` was enabled only after wave 3.

| | Count |
|---|---|
| Handlers moved | 22 |
| Handlers staying in `httpapi` | 1 (`health`) |
| Distinct destination packages | 19 — **four `jobsources` handlers share one** (`hosts`, `sources`, `searches`, `roster`) |
| Modules gaining an `interfaces/` layer | 16 |
| Test files moved | 19 |
| Files with changes beyond imports | 2 (`roster`, and `helpers` → `httpx`) |

What stayed: `router.go` (the router), `middleware.go` (`requestLogger`, applied once), and
`health` — cross-cutting, owned by no feature. `helpers.go` moved to `internal/httpx` and was
exported.

> **`health` moved to its own `internal/health` package unconditionally**, not conditionally on
> whether 026 had landed. 026 adds a `PoolStatter` referencing `internal/db` to
> `HealthHandler`; leaving `health` in `httpapi` would have made the router package import
> `db`. That passes 027-SC-001 literally — `db` is not a *feature* — but weakens the invariant.
> A conditional resolution would have made the outcome depend on merge order, which nobody
> remembers later. Whichever feature landed second adjusted one wiring line in `compose.go`.

**The 19 test files moved unmodified.** A test requiring edits is a signal the move changed
behaviour — stop and investigate rather than adjusting the test. `router_test.go` stayed; its
route-parity assertions are the primary guard for 027-FR-006 and should be extended, never
replaced.

## 2. Shared types — single source (024)

`packages/shared` is generated from Go DTOs via tygo. Before 024 it also carried 56
hand-written duplicates of generated shapes.

| # | Rule |
|---|---|
| 024-FR-001 | Each shared type with a backend counterpart has **exactly one** definition (024-SC-001: duplicates 56 → 0). |
| 024-FR-002 | Hand-maintained duplicates are **removed**, not kept in sync. Syncing by hand is the failure mode, not the fix. |
| 024-FR-003 | Where a consumer needs a narrower form than generation can express, the narrowing is derived from or layered on the generated type — `packages/shared/src/index.ts` re-exports and narrows; it never restates a shape. |
| 024-FR-004 | Types with no backend counterpart are retained, explicitly labelled consumer-only, and kept separate — `packages/shared/src/consumer-only.ts` (024-SC-003: no type is ambiguous about which it is). |
| 024-FR-005 | The public import surface is unchanged, so no consumer file needs an import change (024-SC-004: zero of 47 importers touched). |
| 024-FR-008 | Reintroducing a hand-maintained duplicate is caught by an automated check — the `shared-types-no-duplicates` CI job. |

024-FR-007 required each already-diverged pair to be enumerated with its resolution recorded,
rather than silently resolved in generation's favour. 024-FR-009 required in-flight
uncommitted edits to be reconciled into the result, not discarded.

**Workflow** (024-SC-002): adding a field is one file edit — the Go DTO in
`apps/api/internal/dto/` — plus `make tygo-generate`. Down from two hand edits.

### 2.1 The public surface of `@job-finder/shared`

47 dashboard files import this package, and **the surface is frozen**: same entry point, same
export names, same or stricter types. Anything else is a breaking change.

`packages/shared/src/index.ts` is the only entry point. **Deep imports into `generated.ts`,
`nullable.ts` or `consumer-only.ts` are not part of the contract and must not appear in
consumers.**

| File | Role | Hand-edited |
|---|---|---|
| `generated.ts` | tygo output from `apps/api/internal/dto` | **never** |
| `nullable.ts` | the `Nullable<T, K>` generic, ~6 lines | rarely |
| `consumer-only.ts` | the 14 types with no backend counterpart | yes, deliberately |
| `index.ts` | re-exports and narrowings | field names only |

Six rules:

1. **No shape is defined twice.** A type with a Go counterpart is generated; `index.ts` may
   narrow it but never restate its shape.
2. **A narrowing touches only what it narrows.** Naming a field is always fine; naming a
   field's *type* is fine only for the field being narrowed, and only where generation cannot
   express the constraint.
   ```ts
   // allowed — names fields, no types at all
   export type ActivityRunDto = Nullable<Gen.ActivityRunDto, 'error' | 'jobId'>;

   // allowed — restates one field's type, because generation cannot express it
   export type JobDto = Omit<Gen.JobDto, 'status'> & { status: ApplicationStatus | 'hidden' };

   // forbidden — restates the whole shape
   export interface ActivityRunDto { id: string; op: string; error: string | null; /* … */ }

   // forbidden — restates a field the narrowing does not change
   export type JobDto = Omit<Gen.JobDto, 'status' | 'title'>
     & { status: ApplicationStatus | 'hidden'; title: string };
   ```
3. **Adding a Go DTO field requires zero edits here.** Only a change in *nullability* touches
   `index.ts`.
4. **A consumer-only type carries a reason** — a comment explaining why it has no backend
   counterpart.
5. **Strictness may increase, never decrease.** `T | null` must not become `T?`; a literal
   union must not become `string`.
6. Go DTO structs are the source of truth; tygo (`apps/api/tygo.yaml`) emits `generated.ts`.

Enforcement: `scripts/tygo-check.sh` keeps `generated.ts` matching the Go DTOs; `tsc --noEmit`
plus `vitest run` keep the public surface unchanged; and the `shared-types-no-duplicates`
baseline-comparison script catches both a reintroduced duplicate shape **and** any Go pointer
field lacking `omitempty` that has no `Nullable` entry.

> **That last check is what makes 024-FR-008 real.** Without it, nothing stops the next author
> pasting an interface back into `index.ts`, and the feature decays into a one-time cleanup.

The frozen inventory is 91 exports: 56 previously-duplicated shapes now generated (some
wrapped in a narrowing), the 14 consumer-only types, and the remaining const arrays and derived
union types (`SOURCE_KINDS`, `APPLICATION_STATUSES`, `DOCUMENT_TYPES`, `ENTRY_TYPES` and their
`typeof …[number]` aliases). With `enum_style: union`, prefer the generated union and keep the
const array only where a consumer iterates it at runtime.

**Explicit non-goals**: renaming any type, changing any field name or wire representation,
adding new types or Go DTOs, or introducing a runtime validator (zod or similar) — that last
is a larger change with its own trade-offs.

## 3. Documentation ownership (024-FR-010..019)

- 024-FR-010: for each topic, **exactly one** document states the operative rule. Others
  refer to it rather than restating it (024-SC-006: zero conflicting pairs; 024-SC-011: a
  reader can determine which document owns a rule in under a minute).
- 024-FR-011: the constitution and `AGENTS.md` must not contradict each other on shared type
  definitions.
- 024-FR-015: **every rule stated in a context document must be true of the repository when
  the change lands** — including claims about directories, file counts and command coverage
  (024-SC-007: zero false statements, verified statement by statement).
- 024-FR-016: amending the constitution follows its own procedure — version bump, Sync Impact
  Report update, and a re-check of `.specify/templates/*.md` and the installed `speckit-*`
  skills in the same change.

**The ownership table.** A rule has exactly one home; other documents may point at it, never
restate it in their own words. Restatements are how the original contradictions arose.

| Topic | Owner | May be referenced by |
|---|---|---|
| Product trust boundary (no auto-apply) | `constitution.md` I | README |
| Grounded generation | `constitution.md` II | — |
| Type-sharing **principle** | `constitution.md` III | `AGENTS.md` |
| Type-sharing **procedure** (how to regenerate) | `AGENTS.md` | — |
| Test-discipline **principle** | `constitution.md` IV | `AGENTS.md` |
| What the quality command covers | `AGENTS.md`, matching the `Makefile` recipe | — |
| Branch and pull-request rule | `AGENTS.md` | enforced by 023 |
| Worktree lifecycle | `AGENTS.md` | — |
| Migration numbering | `constitution.md` Tech Constraints | — |
| Local-first inference | `constitution.md` V | README |
| Commit conventions | `AGENTS.md` | — |

> **The split rule**: the constitution says *what must be true and why*; `AGENTS.md` says
> *what to run*. When both must mention a topic, the constitution states the principle and
> `AGENTS.md` states the procedure implementing it. **Two statements of the same rule in
> different words is the defect** — not a redundancy to tolerate.

**Principle III was not amended, and that was the point.** It was correct; the *practice*
violated it. Amending the principle to bless the duplication would have written the bug into
law. The constitution was still amended (1.0.0 → 1.0.1, PATCH) for a different reason
entirely — its line claiming design documents live under a `plan/` directory was false, and
024-FR-015 requires every stated rule to be true. A corrected constitution carrying a stale
version line and an unchanged Sync Impact Report would itself have been a new false statement.

Verification is per-statement, not a skim: for every row of the ownership table, search every
context document for statements on that topic; two documents stating the same rule in different
words fails 024-SC-006, and any statement untrue of the repository fails 024-SC-007.

Three specific corrections 024 made, recorded because the same errors are easy to reintroduce:

| # | Was | Is |
|---|---|---|
| 024-FR-017 | The constitution said design documents live under a `plan/` directory. | That directory never existed. Plans live at `specs/<nnn>-<slug>/plan.md`. Constitution 1.0.1 corrected it. |
| 024-FR-018 | `AGENTS.md` described a project directory for implementation plans. | It did not exist; the claim was removed. |
| 024-FR-019 | `AGENTS.md` said the backend DTOs live in a **single file**, in two places. | They are spread across ten files under `apps/api/internal/dto/`. |

024-FR-012/013/014 fixed the same class of drift in `AGENTS.md`: it must describe the quality
command's coverage accurately, state the branch-and-PR rule, and state which working copy is
authoritative and how worktrees are created and retired.

024-SC-010 is the point of all of it: an agent given the same task in two sessions follows
the same rules, because only one version of each rule exists to find.

## 4. One agent stack (024-FR-020..025)

- Exactly one supported agent stack is declared (024-FR-020), and the declared configuration
  names the stack actually installed (024-FR-022).
- Exactly one copy of each speckit command definition exists (024-FR-021, 024-SC-008: down
  from two).
- The removed stack's directory and manifests were deleted, and no configuration, script or
  document references them (024-FR-023, 024-SC-009: zero references remain).
- All speckit commands still resolve their helper scripts and templates
  (`.specify/scripts/bash/`, `.specify/templates/`) after consolidation (024-FR-024).
- 024-FR-025: a future speckit upgrade targets the single declared stack by default and must
  not reinstall the removed one. **Check this after any `specify` tooling upgrade.**

## 5. The scraper library boundary (043)

The scraping stack is **not in this repository**. The adapter framework, the 25 site
adapters, the retrieval ladder and the scraping helpers live in
`github.com/nonamecat19/job-scraper`, a separate Go module consumed by `apps/api` as an
ordinary tagged dependency (`v0.1.0` at extraction). There is **no `replace` directive** in
`apps/api/go.mod` — local-path wiring was a development convenience during the move and was
dropped to publish.

**The boundary rule, stated once**: the library scrapes; the app persists, schedules and
serves. Everything the library needs from a database, a queue, a config file or an encryption
key arrives through a port the app implements.

| # | Rule |
|---|---|
| 043-FR-001 | The scraping capability (adapter framework + the site adapters) is consumable as a standalone Go module by any Go project, with no dependency on `apps/api`. |
| 043-FR-002 | The library's `go.mod` declares **no** dependency on `pgx`, `asynq`, `viper`, `minio`, `pgvector`, `goose`, or any `github.com/job-finder/api/internal/*` path. Verifiable by reading the file (043-SC-002). |
| 043-FR-003 | The job model types (`NormalizedJob`, `SearchQuery`, `SourceKind`, `JobSourceDto`) are owned by the library's `model` package; `apps/api/internal/dto` re-exports them as **type aliases** (`internal/dto/scraper_aliases.go`) so every app importer and the tygo generator are source-compatible. |
| 043-FR-004 | The adapter framework (`Adapter`, `PostingReader`, `Credentialed`, `DetailNeeder`, `Registry`, adapter errors) references only library model types — never `dto`, never `sqlcgen`. |
| 043-FR-005 | HTML helpers and the JSON HTTP client are library packages with no dependency on the retrieval package; the default HTTP client is **injectable** rather than hard-wired to a package-level transport. |
| 043-FR-006 | The retrieval engine — ladder, challenge detection, page outcomes, browser identity, the three rungs, the `Service` interface — is library-owned. See [`retrieval-and-ingestion.md`](retrieval-and-ingestion.md) § 2.1. |
| 043-FR-007 | The engine reads and writes host state through a **library-owned** state store port (`ports.StateStore` since the library's ports/adapters restructure), so a consumer can back it with an in-memory map and no DB. |
| 043-FR-008 | The engine imports none of the app's `config`, `db`, `crypto` or `ratelimit`; per-host pacing is library-internal (`ratelimit` was vendored into `retrieval/pacing.go` and deleted from the app). |
| 043-FR-009 | The `Scraper` port is library-owned (`ports.Scraper`). Its chromedp-backed implementation moved to the library's `internal/`, so the app brings its own (`apps/api/internal/scraping`) and hands it to the sources — the same value also serves company-intel and PDF rendering, which touch no job source at all. |
| 043-FR-010 | The 6 board adapters moved **whole**, roster concerns included, and persist through a library-owned roster port (`ports.Roster`). See [`job-sources.md`](job-sources.md) § 3. |
| 043-FR-011 | The app retains the application services, the sqlcgen-typed repository interfaces, the `roster` package (now a port implementation), the HTTP interfaces, the worker handler and scheduler, and the retrieval state/wiring files. |
| 043-FR-012 | Stored cookies are still encrypted app-side with `internal/crypto` and `CONFIG_ENCRYPTION_KEY`. **The library never sees the key**; cookie bytes cross the port as plaintext JSON. |
| 043-FR-016 | The library's package graph allows importing the engine **without** transitively importing the adapters — a consumer who wants only the fetch ladder pays for nothing else (043-SC-006). |
| 043-FR-017 | The library does not depend on `dto`; former `dto.NormalizedJob`/`dto.SearchQuery`/`dto.SourceKind` references point at `model`. |
| 043-FR-018 | The `arch_test.go` exemption list may shrink when an exempted package leaves the tree. **No new exemption may be added to accommodate the library** — it lives outside `internal/`, so neither `depguard` nor the placement test needs to know about it. |
| 043-FR-019 | The library vendors the ~30 lines of string helpers its adapters use (`strutil/`) rather than importing the app's `internal/strutil`, which stays in the app unchanged. |

**Package graph.** The import direction is the whole contract; a cycle here would put site
code on the engine's dependency path and break 043-FR-016.

```
model        ← everything (types only, stdlib)
ports        → model                     (every interface; nothing else)
adapter      → ports, model              (registry, Provider/Catalog, capability helpers)
  middleware → ports                     (recover, observe, timeout, retry, log)
retrieval    → ports                     (the engine; NEVER adapter, NEVER adapters)
session      → ports                     (one login implementation for every site)
store/*      → ports                     (in-memory reference implementations)
internal/*   → (helpers with no public contract: htmlutil, httpjson, strutil, scraping)
adapters/*   → adapter, ports, model, internal/*   (one package per site)
adapters/all → every vendor package      (side-effect registration only)
jobscraper   → all of the above          (the Client facade)
```

`adapters/*` are the heavyweight packages and the only ones that know about specific sites;
importing a single vendor package pulls in only that site. `adapter` (singular, the
framework) and `retrieval` (the engine) are each usable alone, and `ports` is the only
package everything shares.

**The two ports, and who implements them.** Both are declared by the library in library-owned
types and satisfied by an existing app struct — no new adapter layer was introduced.

| Port | Declared in | Implemented by | Carries |
|---|---|---|---|
| `ports.StateStore` (9 methods) | `ports/state.go` | `apps/api/internal/retrieval.StateStore` | Per-host rung preference, crawl delay, block/cooling-off timestamps, cookies. `HostState` is a plain struct — no `pgtype`, no `sqlcgen`. |
| `ports.Roster` (11 methods) | `ports/roster.go` | `apps/api/internal/jobsources/roster.Service` | Employer boards and board candidates. IDs are `string` (UUID form) at the boundary; the app converts via `dbutil`. |
| `ports.SourceConfigStore` (2 methods) | `ports/session.go` | `apps/api/internal/jobsources/application.Service` | Per-source settings, and where a credentialed session persists its cookie. |

All three implementations assert conformance at compile time (`var _ ports.StateStore =
(*StateStore)(nil)`) — the port breaking is a build failure, not a runtime surprise.

`ports.Roster` includes the candidate/discovery methods even though **no library adapter calls
them**. That is deliberate: it keeps `roster.Service` the single implementer instead of
forcing a second struct for the app-only half of the surface. `roster/view.go` and
`roster/candidates.go` stay app-side — they produce dashboard DTOs.

**What the app kept, and why it is small.** After the move, `apps/api/internal/retrieval/`
holds only the wiring: `state.go` (the port implementation, with the encryption), plus two
thin files where a whole engine used to be —

- `service_impl.go` — one `NewService` that maps `config.Config` onto the engine's
  functional options (`WithIdentity`, `WithBrowser`, `WithFlareSolverr`,
  `WithCheapRungRetest`, `WithCoolingOff`) and returns `jsretrieval.NewEngine(store, ...)`.
  The ladder, rungs and cooling-off logic are gone from the app entirely. Options rather
  than an opts struct means a knob the app leaves alone keeps the library's default instead
  of being zeroed.
- `transport.go` — `ConfigureDefaultTransport`, which points the library's paced transport
  at the app's host state. It is called from `composeRetrieval` and is load-bearing rather
  than boilerplate: dropping it ignores every crawl delay, and the omission does not break
  the build — see [`retrieval-and-ingestion.md`](retrieval-and-ingestion.md) § 3.1 for the
  release-long outage that followed the last time it went missing.

  `UsePacedHTTPJSONClient` is **gone** — and so is the need for it. `httpjson` moved into
  the library's `internal/`, putting its default client out of the app's reach, so the
  library now installs the paced client itself (`retrieval/jsonclient.go`). Importing
  `retrieval` is what paces the JSON path; the app contributes only the resolver, through
  `ConfigureDefaultTransport`.

**Regression guardrails** (043-FR-013/014/015, 043-SC-003/004/005): every adapter unit test
and the `live_smoke_test` moved with their code and passed **unmodified** — no fixture or
assertion was adjusted to accommodate the move; `make test-lint` passes; and
`packages/shared/src/generated.ts` is byte-identical before and after, because the moved
types are re-exported as aliases rather than reshaped. The dashboard never learned that the
scraping stack left the repo.

**Working across the two repos.** A change to adapter or engine behaviour is now a library
change, tagged and then bumped in `apps/api/go.mod` — not an edit under `apps/api/internal/`.
The app-side files that may still change for a scraping reason are the two ports'
implementations and the `compose.go`/`platform.go` wiring.
