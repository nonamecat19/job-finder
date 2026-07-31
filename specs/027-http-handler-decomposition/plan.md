# Implementation Plan: HTTP Handler Decomposition into Feature Modules

**Branch**: `027-http-handler-decomposition` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/027-http-handler-decomposition/spec.md`

## Summary

`internal/httpapi` holds 26 non-test source files (2300 LOC) plus 19 test files, covering 23 feature areas, and imports 24 other internal packages. Every feature meets there.

The move is mechanical because the groundwork already exists: `NewRouter(mounts ...func(chi.Router))` accepts route contributions positionally, so physical package location is already irrelevant to wiring; each handler already declares its own consumer interface (`JobsProvider`, `DocumentLister`, …) rather than depending on a concrete service; and five modules already have an `interfaces/` layer holding their worker adapter, with the HTTP adapter as the obvious missing sibling.

Four workstreams:

1. **Extract shared helpers** into a package features may depend on (`internal/httpapi/httpx` or similar), so the move does not duplicate `writeJSON`/`writeError`.
2. **Move handlers**, one feature per commit, into `internal/<feature>/interfaces/http/`.
3. **Fix the one layering violation in transit** — `roster.go` imports `db/sqlcgen` and `dbutil` directly.
4. **Lock it** with `depguard` rules in the existing `apps/api/.golangci.yml`, **plus one placement test** — `depguard` matches import paths, not file locations, so it cannot alone satisfy FR-011.

## Technical Context

**Language/Version**: Go 1.26 (`apps/api` only)

**Primary Dependencies**: existing — `go-chi/chi/v5` v5.3.1, `go-chi/cors`. Linting: golangci-lint 2.12.2, already pinned in `apps/api/.golangci-version` and wired into `make lint-go`. **No new dependencies.**

**Storage**: none. No migration, no query change, no sqlc regeneration.

**Testing**: `go test ./...`. Handler tests move with their handlers. The dashboard's Playwright suite (`make test-e2e`) is the end-to-end guard that no route changed.

**Target Platform**: Linux.

**Project Type**: Backend restructuring. Zero behaviour change by design.

**Performance Goals**: none. Success is structural (SC-001: zero feature dependencies from the shared package, down from 24).

**Constraints**:

- **`apps/api` does not currently compile** — seven packages broken by the DDD restructure (see tasks.md Phase 0). A refactor whose entire safety argument is "the compiler and the tests agree nothing changed" cannot begin on a red tree. This is a harder blocker here than for features 025/026.
- Route registration is positional in `buildServers` (`cmd/server/servers.go:70-79`); moving a handler changes its import path there but not the mount mechanism.
- `NewRouter` mounts every contribution **twice** — `r.Route("/api", mountAll)` and `r.Route("/api/v1", mountAll)` — from a single registration. This must survive (FR-007).
- Shared helpers live in `helpers.go` (`writeJSON`, `writeError`) and `middleware.go` (`requestLogger`). They are lowercase/unexported today; extracting them means exporting them, which is the only source-level change to code that is otherwise moved verbatim.
- **One handler violates layering already**: `roster.go` imports `db/sqlcgen` and `dbutil` directly. Six others (`activity`, `contacts`, `hosts`, `notifications`, `postage`, `sources`) import only `dto` and move trivially.
- `internal/dto` is imported by 22 of 23 handlers and is **not** decomposed by this feature — it is a cross-cutting contract package also consumed by workers and by tygo. Splitting it is entangled with the `packages/shared` duplication problem (feature 024) and is explicitly out of scope.
- The five modules with an existing `interfaces/` layer use `interfaces/worker`. The HTTP sibling must be named consistently — `interfaces/http` — rather than introducing a new term.
- `depguard` is a standard golangci-lint linter; the config currently sets `linters.default: standard` with `errcheck` disabled. Adding `depguard` means adding it to an `enable` list, since it is not in the standard set.

**Scale/Scope**: 23 feature areas, 26 source files, 19 test files, ~2300 LOC moved. Zero net LOC change expected beyond import lines and the helper export.

## Constitution Check

| Principle | Assessment |
|---|---|
| I. No Auto-Apply, Ever | **N/A** — no behaviour change at all. |
| II. Grounded Generation | **N/A**. |
| III. Typed Contracts | **Respected.** No DTO change, no sqlc or tygo regeneration. `internal/dto` is untouched, so `packages/shared` is untouched. |
| IV. Test Discipline | **Respected.** Handler tests move with handlers and must pass unmodified — a test that needs editing signals the move changed behaviour, which is a defect. `make test-e2e` is the cross-boundary guard. |
| V. Local-First | **N/A**. |

**Deviation to justify**: none. The `depguard` addition strengthens the constitution's architecture constraints rather than deviating from them.

## Project Structure

### Documentation (this feature)

```text
specs/027-http-handler-decomposition/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── inventory.md   # The 23 handlers, their dependencies, destinations, order
│   └── depguard.md    # The arrangement rules and their messages
├── checklists/requirements.md
└── tasks.md
```

### Source Code (target shape)

```text
apps/api/internal/
├── httpapi/                       # SHRINKS to routing only
│   ├── router.go                  # NewRouter, mounts, 404
│   ├── middleware.go              # requestLogger
│   └── router_test.go
├── httpx/                         # NEW: shared response helpers, depended on by features
│   ├── json.go                    # WriteJSON, WriteError (exported)
│   └── json_test.go
├── jobs/
│   └── interfaces/http/           # NEW, sibling to interfaces/worker where one exists
│       ├── handler.go
│       └── handler_test.go
├── applications/interfaces/http/
├── profile/interfaces/http/
└── …                              # one per feature area
```

**Structure Decision**: shared helpers move to a **new sibling package `internal/httpx`**, not to a subpackage of `httpapi`. Features must depend on the helpers; if the helpers stayed under `httpapi`, every feature would import `httpapi`, and SC-001 ("the shared routing package depends on zero feature modules") would be satisfied only in letter — the coupling would simply invert. A separate leaf package that depends on nothing keeps the dependency graph acyclic and the rule expressible in `depguard`.

## Phase 0: Research

See [research.md](./research.md). Five questions resolved: what actually blocks the move (R1), where shared helpers go (R2), naming (R3), the enforcement mechanism (R4), and sequencing for reviewability (R5).

## Phase 1: Design

- [data-model.md](./data-model.md) — the target package graph and the allowed dependency directions.
- [contracts/inventory.md](./contracts/inventory.md) — all 23 handlers with measured dependencies, destination, and migration wave.
- [contracts/depguard.md](./contracts/depguard.md) — the rules, their messages, and what each prevents.
- [quickstart.md](./quickstart.md) — route-parity verification.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|--------------------------------------|
| A new `internal/httpx` package for two functions | Features must depend on the helpers without depending on the router. A leaf package is the only shape that keeps the graph acyclic and the `depguard` rule expressible. | Leaving helpers in `httpapi` was rejected: every feature would then import `httpapi`, inverting the coupling rather than removing it. Duplicating the helpers per feature was rejected by FR-008. |
| Fixing `roster.go`'s data-access violation inside this refactor | Moving it unchanged installs a layering violation *inside* the new adapter layer — the exact thing the feature exists to prevent — and `depguard` would then fail on freshly-moved code. | Deferring the fix was rejected because it would require an exemption in the very rule being introduced, and exemptions added at birth never get removed. |
| 23 separate commits rather than one | FR-014 and SC-007 require incremental delivery; a single 45-file move is unreviewable. | A single change was rejected explicitly in the spec's Assumptions. |
