# Implementation Plan: CI Test Gate

**Branch**: `007-ci-test-gate` | **Date**: 2026-07-24 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/007-ci-test-gate/spec.md`

## Summary

CI (`.github/workflows/api-ci.yml`) currently runs only two generated-code drift checks
(sqlc, tygo). `go test`, `go vet`, and the frontend test/typecheck suite never run in CI,
so regressions caught by `make test` locally can still merge. Added CI jobs (`go-vet`,
`go-test`, `frontend-test`, `frontend-typecheck`) that run `go vet ./...`, `go test ./...`,
`pnpm --filter @job-finder/dashboard test`, and `pnpm typecheck`. No Postgres/Redis
services needed for `go-test`: all DB/Redis-touching tests are gated behind a
`//go:build integration` tag, which plain `go test ./...` never compiles in (see
research.md revision). Do not add `golangci-lint` or ESLint gating — no config exists for
either yet, and authoring one is out of scope for this fix (per spec Assumptions/FR-008).

## Technical Context

**Language/Version**: Go 1.26 (apps/api), TypeScript/Node >=20 + pnpm workspaces (apps/dashboard, packages/shared)

**Primary Dependencies**: GitHub Actions (`actions/checkout@v4`, `actions/setup-go@v5`, `pnpm/action-setup`, `actions/setup-node@v4`), existing Makefile targets (`test-go`, `test-react`), `go vet`, `pnpm typecheck`

**Storage**: N/A for CI — plain `go test ./...` never compiles the `//go:build integration`-tagged tests that touch Postgres/Redis, so no service containers are needed (revised from initial plan; see research.md)

**Testing**: `go test ./...` (apps/api), `vitest run` (apps/dashboard) — both already wired via `make test-go` / `make test-react`

**Target Platform**: GitHub Actions `ubuntu-latest` runners

**Project Type**: Web application (Go backend `apps/api` + React frontend `apps/dashboard` + shared TS package `packages/shared`) — CI workflow change only, no application code changes

**Performance Goals**: N/A (CI reliability/gating feature, not a runtime performance feature)

**Constraints**: Must not modify or remove the existing `sqlc-drift` / `tygo-drift` jobs (FR-007); must not introduce lint config authoring (FR-008)

**Scale/Scope**: Single workflow file (`.github/workflows/api-ci.yml`); no new files needed on the application side; runs on every push to `master` and every pull request, same triggers as existing jobs

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Principle IV (Test Discipline Per Language, Enforced at the Boundary)**: This feature
  directly implements the constitution's own requirement — "`go test` for apps/api,
  `vitest` for the dashboard" — as an enforced CI gate rather than a local-only
  convention. PASS, no violation; this feature closes an existing compliance gap rather
  than creating one.
- **Principle III (Typed Contracts Across Service Boundaries)**: Existing `sqlc-drift` /
  `tygo-drift` jobs already enforce this; left untouched (FR-007). PASS.
- **Principles I, II, V**: Not implicated — no application behavior, LLM generation, or
  auto-apply logic changes. N/A.
- No Complexity Tracking entries required — no violations.

## Project Structure

### Documentation (this feature)

```text
specs/007-ci-test-gate/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── quickstart.md         # Phase 1 output (local validation guide)
└── checklists/
    └── requirements.md
```

No `data-model.md` or `contracts/` — this feature has no data entities and exposes no
external interface; it modifies an internal CI workflow only.

### Source Code (repository root)

```text
.github/
└── workflows/
    └── api-ci.yml        # MODIFIED: add go-test, go-vet, frontend-test, frontend-typecheck jobs

apps/api/                 # unchanged — existing `make test-go`, `go vet ./...` targets reused
apps/dashboard/           # unchanged — existing `make test-react`, `pnpm typecheck` targets reused
Makefile                  # unchanged — targets already exist (test-go, test-react, typecheck)
```

**Structure Decision**: Single-file change to `.github/workflows/api-ci.yml`, adding jobs
alongside the existing `sqlc-drift` and `tygo-drift` jobs. No new directories, no
application source changes — this is a CI-configuration-only feature reusing tooling that
already exists per the Makefile and root `package.json`.

## Complexity Tracking

*No violations — table not needed.*
