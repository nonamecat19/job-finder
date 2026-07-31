# Phase 1 Data Model: HTTP Handler Decomposition

No database schema. No data structures. The "model" here is the target package graph and the dependency rules over it.

---

## 1. Target package graph

```
cmd/server
    │  imports everything (composition root — the one place that may)
    ├──> internal/httpapi          (router, cross-cutting middleware)
    ├──> internal/<feature>/interfaces/http
    ├──> internal/<feature>/interfaces/worker
    └──> internal/<feature>/application, /domain, /infrastructure

internal/<feature>/interfaces/http
    ├──> internal/httpx            (response helpers)
    ├──> internal/dto              (shared wire types)
    ├──> internal/apperr           (error classification)
    ├──> its own feature's application/domain
    └──> github.com/go-chi/chi/v5

internal/httpapi
    ├──> github.com/go-chi/chi/v5
    ├──> github.com/go-chi/cors
    └──> (nothing under internal/ except httpx)

internal/httpx
    └──> net/http, encoding/json only
```

**The single invariant**: `internal/httpapi` sits *above* features in the composition order but must import *none* of them. `cmd/server` is the only package that knows both the router and the features, which is exactly what a composition root is for.

---

## 2. Allowed dependency directions

| From | May import | May not import |
|---|---|---|
| `cmd/server` | anything | — |
| `internal/httpapi` | `httpx`, chi, cors, stdlib | any feature package |
| `internal/<f>/interfaces/http` | `httpx`, `dto`, `apperr`, own feature, chi, stdlib | `httpapi`; another feature's `application`/`domain`/`infrastructure` |
| `internal/<f>/interfaces/worker` | as above, plus asynq | as above |
| `internal/httpx` | stdlib only | everything under `internal/` |
| `internal/dto` | stdlib, domain value types | `httpapi`, any `interfaces/` package |

**Cross-feature rule (FR-012)**: a feature's HTTP adapter needing another feature must go through that feature's own boundary — its exported application service or a locally-declared port satisfied by it — never its `domain` or `infrastructure` internals. In practice this is already how handlers work: each declares a local consumer interface (`JobsProvider`, `DocumentLister`) rather than importing a concrete type, so the rule mostly documents existing behaviour.

---

## 3. Per-feature adapter shape

Uniform across all 23, including single-endpoint features (spec Clarifications — consistency wins):

```
internal/<feature>/interfaces/http/
├── handler.go       # the Handler struct, its ports, and Mount(chi.Router)
└── handler_test.go  # moved verbatim from internal/httpapi/<feature>_test.go
```

`Mount(r chi.Router)` keeps its current signature. `buildServers` continues to pass `X.Mount` positionally, so both the `/api` and `/api/v1` mounts continue to come from one registration (FR-007) with no change to `NewRouter`.

---

## 4. What stays in `internal/httpapi`

| File | Fate |
|---|---|
| `router.go` | stays — `NewRouter`, the dual mount, the 404 handler, CORS |
| `middleware.go` | stays — `requestLogger` is applied once by the router; no feature calls it |
| `router_test.go` | stays |
| `helpers.go` | **moves** to `internal/httpx`, exported |
| `health.go` / `health_test.go` | **moves** to `internal/httpapi`… see below |
| all 22 other handler files + tests | **move** to their feature |

**`health` moves to its own `internal/health` package** (T039), unconditionally. It has no feature package — it pings Postgres, Redis and MinIO through a local `Pinger` — so leaving it in `httpapi` would not violate the letter of the invariant today. But feature 026 adds a `PoolStatter` to it referencing `internal/db`, which would leave `httpapi` importing `db`: SC-001 (feature packages only) still passes literally while the invariant weakens. Making the move conditional on merge order was rejected — the outcome would depend on which feature landed first, which nobody will remember later. `internal/health` is the answer in both orderings.

---

## 5. Migration waves

| Wave | Handlers | Why grouped |
|---|---|---|
| 0 | — (`httpx` extraction only) | Isolates the sole source-level edit from the pure renames |
| 1 | `activity`, `contacts`, `hosts`, `notifications`, `postage`, `sources` | `dto`-only; zero feature coupling; proves the pattern |
| 2 | `applications`, `coach`, `companies`, `documents`, `ghostjob`, `interviewprep`, `jobs`, `keyword`, `outreach`, `profiles`, `referral`, `searches`, `subscriptions`, `aifeature`, `llm_settings` | Single or double feature dependency; mechanical repetition |
| 3 | `roster`, then `depguard` | The one real fix, then the lock |

`depguard` is last by necessity: enabled earlier it fails on every unmoved handler, producing an exemption list that must then be unwound (research.md R5).

---

## 6. The one real fix: `roster`

`roster.go` imports `db/sqlcgen` and `dbutil` directly — the only handler that reaches past its feature into data access.

Moving it unchanged would install that violation *inside* the new adapter layer and would require a `depguard` exemption in the very rule being introduced. It is fixed in transit instead: the data access it performs moves behind `internal/jobsources/roster`'s own boundary, and the handler depends on a locally-declared port like every other handler.

Scope check before starting: `roster.go` is 1 of 26 files. If the fix turns out to be materially larger than a handler move, it is split into its own preceding change rather than allowed to swell this one.

---

## 7. Success measurement

| Criterion | How measured |
|---|---|
| SC-001 | `go list -deps ./internal/httpapi \| grep job-finder/api/internal \| grep -v httpx \| wc -l` → 0 (from 24) |
| SC-002 | Adding an endpoint touches one feature directory + at most one line in `servers.go` |
| SC-003 | Route inventory diff before/after → empty (quickstart.md) |
| SC-004 | `make test-e2e` passes unmodified |
| SC-005 | Deliberate violations → `make lint-go` fails naming the import, **and** the placement test fails naming the file |
| SC-006 | Every feature has `interfaces/http`; no handler left in `httpapi` but `health` |
| SC-007 | ≥20 commits, each building and passing tests |
