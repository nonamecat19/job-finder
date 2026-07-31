# Phase 0 Research: HTTP Handler Decomposition

All Technical Context unknowns resolved. Measured against this repository.

---

## R1 — What actually blocks moving a handler out of `httpapi`?

**Decision**: nothing structural. The blockers are two shared helpers and one handler's direct data-access import.

**Evidence**:

- **Route wiring is already location-agnostic.** `NewRouter(mounts ...func(chi.Router))` takes route contributions positionally (`internal/httpapi/router.go`), and `buildServers` passes 23 `X.Mount` values (`cmd/server/servers.go:70-79`). Moving a handler changes its import path in `servers.go` and nothing else about wiring. AGENTS.md already documents this as the intended extension point: "New HTTP handlers are wired in `cmd/server/main.go` via `httpapi.NewRouter(...)`'s variadic mounts, not by editing `router.go` directly."
- **Handlers already own their dependency interfaces.** `jobs.go` declares `JobsProvider` and `DocumentLister` locally rather than importing a concrete service type. This is the pattern throughout, so a moved handler carries its own port with it.
- **Measured dependency profile** (`grep` over each non-test file, excluding `router.go`, `helpers.go`, `middleware.go`):

  | Dependency shape | Count | Handlers |
  |---|---|---|
  | `dto` only | 6 | `activity`, `contacts`, `hosts`, `notifications`, `postage`, `sources` |
  | `dto` + one feature package | 14 | `applications`, `coach`, `companies`, `documents`, `ghostjob`, `interviewprep`, `jobs`, `keyword`, `outreach`, `profiles`, `referral`, `searches`, `subscriptions`, `aifeature` |
  | `dto` + two feature packages | 1 | `llm_settings` (`platform/llm`, `llmsettings`) |
  | **direct data access** | 1 | **`roster`** (`db/sqlcgen`, `dbutil`) |
  | none | 1 | `health` |

**Consequence**: 22 of 23 are verbatim moves plus an import-path edit. One (`roster`) needs a real fix. The feature is low-risk in aggregate and the risk is concentrated in a single known file.

---

## R2 — Where do the shared response helpers go?

**Decision**: a new leaf package `internal/httpx`, with `writeJSON`/`writeError` exported as `WriteJSON`/`WriteError`.

**Rationale**: features must be able to write responses. Three placements were considered:

| Placement | Result |
|---|---|
| Leave in `httpapi` | Every feature imports `httpapi`. SC-001 is satisfied literally (`httpapi` imports no feature) but the coupling has merely inverted, and the `depguard` rule becomes unstateable — you cannot forbid a dependency you also require. **Rejected.** |
| Duplicate per feature | Violates FR-008 explicitly, and a bug in error formatting would need 23 fixes. **Rejected.** |
| New leaf package `httpx` | Depends on nothing but `net/http` and `encoding/json`. Graph stays acyclic; rule is expressible as "features may import `httpx`, nobody may import `httpapi`". **Chosen.** |

**Consequence**: the only source-level change to otherwise-verbatim-moved code is `writeJSON(` → `httpx.WriteJSON(`. Mechanical, and greppable for review.

**Open detail for implementation**: `requestLogger` (in `middleware.go`) stays in `httpapi` — it is applied once by `NewRouter` and no feature calls it. Cross-cutting request behaviour stays centralised (FR-003, spec Assumptions).

---

## R3 — What is the adapter layer called?

**Decision**: `internal/<feature>/interfaces/http/`.

**Evidence**: five modules already have an `interfaces/` layer, and in all five it contains exactly one subpackage, `worker`:

```
matching/interfaces:  worker
generation/interfaces: worker
ghostjob/interfaces:   worker
salary/interfaces:     worker
jobsources/interfaces: worker
```

**Rationale**: the HTTP adapter is the same kind of thing as the worker adapter — a translation layer between an external protocol and the feature's core. `interfaces/http` is the sibling the existing convention implies. Introducing a new term (`api/`, `rest/`, `transport/`) would create two names for one concept, which is how the current inconsistency started.

**Package-name collision — decided, not deferred**: the package is named `http`, and `net/http` is imported normally inside it. This is legal Go and unambiguous: import names are file-scoped, package-level declarations are package-scoped, and a package never refers to itself by name — so `http.ResponseWriter` inside `package http` resolves to the standard library. The decision is made here rather than at implementation time because it applies to all 19 destination packages and reversing it mid-migration renames every package moved so far. **Fallback**, if the pinned linter objects: `interfaces/httpapi`, applied uniformly to every feature in one change, decided once.

**Consequence**: the 22 moved handlers land in **19 distinct destination packages** (the four `jobsources` handlers — `sources`, `hosts`, `searches`, `roster` — share one). Three of those modules already have an `interfaces/` directory holding their worker adapter (`generation`, `ghostjob`, `jobsources`), so **16 modules gain an `interfaces/` layer for the first time** (FR-002). `matching` and `salary` have existing `interfaces/worker` but no HTTP handler, so they are untouched by this feature. Package name is `http`, which shadows the stdlib `net/http` import inside those files — resolved by the conventional alias (`nethttp "net/http"`) or by the fact that the package's own name is not referenced from within itself. Verify at the first move; if it proves noisy, `interfaces/httpapi` is the fallback, decided once and applied uniformly.

---

## R4 — How is the arrangement enforced?

**Decision**: `depguard` rules in the existing `apps/api/.golangci.yml`.

**Evidence**: golangci-lint is pinned (`apps/api/.golangci-version`), configured (`apps/api/.golangci.yml`, `linters.default: standard`, `errcheck` disabled with a measured rationale), wired into `make lint-go`, and gates CI via the `lint-go` job. `depguard` is a standard golangci-lint linter but is **not** in the `standard` set, so it must be added to an explicit `enable` list.

**Rules** (detailed in `contracts/depguard.md`):

1. Nothing may import `internal/httpapi` except `cmd/server`. Enforces SC-001/FR-004 in both directions — it also stops a feature from importing the router.
2. `internal/httpx` may not import any `internal/` package — keeps the leaf a leaf.
3. `internal/*/interfaces/**` may not import another feature's infrastructure (FR-012).

**`depguard` alone does not satisfy FR-011.** FR-011 has two halves. Half one — handling placed in the shared routing package — is rule 1. Half two — handling placed inside a feature module but outside its `interfaces/http` package — is a **file-location** property, and `depguard` matches import paths, not locations. A handler at `internal/jobs/handler.go` violates FR-011 and passes all three rules.

**Decision**: cover half two with a placement test (`internal/arch_test.go`), walking the module tree and failing on any file that imports the router library from outside an `interfaces/` package. Roughly twenty lines, runs in the existing `go test ./...`, needs no new tooling and no new CI job.

**Alternative rejected**: narrowing FR-011 to only what `depguard` can do. That would leave the requirement technically satisfied and practically unenforced — the failure mode being guarded against is precisely someone taking the convenient path, and "inside the feature but not in the adapter layer" is the most convenient wrong path available once `httpapi` is closed off.

**Rationale for reusing golangci-lint**: zero new tooling, zero new CI job, and violations are reported with file, line and rule in the same format as every other lint failure — which is what FR-010/FR-011 require ("identify the offending dependency").

**Remaining limitation**: rule 3 is a prefix rule and cannot tell a legitimate cross-feature port from an illegitimate one — a feature importing another feature's `application` package still passes. That is deliberate: `application` is the exported boundary, which FR-012 permits. What is *not* accepted as a limitation is FR-011's second half, which the placement test now covers.

---

## R5 — What order minimises review risk?

**Decision**: three waves, easiest first, each feature its own commit.

| Wave | Contents | Rationale |
|---|---|---|
| 0 | `httpx` extraction; no handler moves | Isolates the only source-level edit (helper export) from all the moves, so wave 1+ diffs are pure renames |
| 1 | The 6 `dto`-only handlers | Zero feature coupling; proves the pattern end-to-end at minimum risk |
| 2 | The 15 single/double-feature handlers | Mechanical repetition of the proven pattern |
| 3 | `roster` (data-access fix), then `depguard` rules | The one real change, then the lock |

**Rationale**: `depguard` lands **last**. Enabling it before the moves are complete would fail the build on every not-yet-moved handler, forcing a growing exemption list that then has to be unwound — turning a clean sequence into a noisy one.

**Verification between waves**: route parity (quickstart.md) after each wave, not only at the end. A route lost in wave 1 and discovered in wave 3 is a bisection problem across 20 commits.

---

## Summary of decisions

| ID | Decision |
|---|---|
| R1 | Nothing structural blocks the move; 22/23 are verbatim, `roster` needs a real fix |
| R2 | Shared helpers → new leaf package `internal/httpx`, exported |
| R3 | `internal/<feature>/interfaces/http/`, matching the existing `interfaces/worker` convention |
| R4 | `depguard` in the existing `.golangci.yml` (three rules, must be added to `enable`) **plus a placement test** — `depguard` alone cannot satisfy FR-011's second half |
| R5 | Four waves, helpers first, `depguard` last, route parity checked between waves |
