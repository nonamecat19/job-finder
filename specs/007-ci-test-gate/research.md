# Research: CI Test Gate

No unresolved `NEEDS CLARIFICATION` markers remained after `/speckit-specify`. This
research documents the concrete choices for the Technical Context / plan.

## Decision (revised during implementation): plain `go test ./...`, no Postgres/Redis service containers

**Rationale**: The original research pass assumed `go test ./...` needs a live
Postgres/Redis because `make test-go` sets `DATABASE_URL`/`REDIS_URL`. Implementation
inspection showed every test file that actually opens a DB/Redis connection (
`internal/db/integration_test.go`, `internal/applications/integration_test.go`,
`internal/httpapi/activity_test.go`, `internal/dbtest/lock.go`, and the other
`*_integration_test.go` / `live_test.go` files) carries a `//go:build integration` tag.
Plain `go test ./...` (no `-tags integration`, matching `make test-go`'s actual Makefile
recipe) never compiles those files in, so no datastore is touched. The `go-test` CI job
therefore runs `go test ./...` directly with no services — simpler and cheaper, and still
faithful to what `make test-go` runs today.

**Alternatives considered**:
- Standing up Postgres/Redis as GitHub Actions `services:` anyway (original plan) —
  rejected after inspection: adds CI minutes and complexity for zero coverage benefit,
  since no code path in the plain unit-test build reaches a datastore.
- Running `-tags integration` in this same job — explicitly out of scope: that's
  `make test-integration`/`test-e2e` territory, a separate, heavier gate not requested by
  the "cheap fix" framing of this feature. Left for a future feature if desired.
- Mocking Postgres/Redis — moot, not needed since the unit build never touches them.

## Decision: `go vet ./...` as its own job step (or combined with the test job)

**Rationale**: `go vet` is fast (seconds) and has no external dependencies, so it can run
as a separate lightweight job (fails fast, clear job name per SC-004) or as an early step
in the same job as `go test` before spinning up services. Separate job chosen for clarity
of failure attribution (SC-004: contributor identifies failing check from job name alone).

**Alternatives considered**: Folding vet into the `go-test` job as a step — works
functionally but the failure would show as "go-test job step 3 failed" rather than a
dedicated `go-vet` job name; separate job is marginally more CI minutes but clearer signal.

## Decision: Frontend jobs use pnpm workspace install (`pnpm install --frozen-lockfile`), matching root `package.json`

**Rationale**: `apps/dashboard`'s `test`/`typecheck` depend on `packages/shared` being
built first (per constitution Development Workflow: "`pnpm --filter @job-finder/shared
build` before other workspace packages"). CI must run that build step before `test-react`
and `typecheck`, exactly as local setup requires.

**Alternatives considered**: Running `pnpm --filter dashboard test` without building
`shared` first — rejected, would fail on missing built output/types that tygo/shared
generate, same failure mode the constitution's workflow note exists to prevent.
