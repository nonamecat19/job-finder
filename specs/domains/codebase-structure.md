# Domain: Codebase Structure

Consolidates **027** HTTP handler decomposition, **024** agent-context and shared-type
consolidation.

Implementation: `apps/api/internal/*/interfaces/http/`, `apps/api/internal/httpapi/`,
`apps/api/internal/httpx/`, `apps/api/.golangci.yml`, `apps/api/internal/arch_test.go`,
`packages/shared/src/`. How it works:
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
