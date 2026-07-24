# Feature Specification: CI Test Gate

**Feature Branch**: `007-ci-test-gate`

**Created**: 2026-07-24

**Status**: Draft

**Input**: User description: "CI is two drift checks only. No go test, no go vet, no golangci-lint (no config file exists), no frontend test/typecheck job, no ESLint config found. make test exists but CI never runs it. Fix first — cheap."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Backend regressions caught before merge (Priority: P1)

A contributor opens a pull request that breaks a Go test or fails `go vet`. Today CI (sqlc-drift, tygo-drift only) is green, so the PR looks mergeable. After this feature, CI runs the Go test suite and `go vet`, and fails the PR before a reviewer or merge sees it.

**Why this priority**: This is the actual gap named in the report — broken backend code currently merges silently. Highest value, cheapest fix (the make targets already exist).

**Independent Test**: Push a branch with a failing Go test (or a `go vet`-flagged issue, e.g. unreachable code) and confirm the new CI job fails on that branch and passes once fixed.

**Acceptance Scenarios**:

1. **Given** a pull request with a failing Go unit test, **When** CI runs, **Then** the backend test job fails and blocks merge.
2. **Given** a pull request with code that fails `go vet`, **When** CI runs, **Then** the job fails with the vet output visible in the log.
3. **Given** a pull request where all Go tests and vet pass, **When** CI runs, **Then** the job succeeds.

---

### User Story 2 - Frontend regressions caught before merge (Priority: P2)

A contributor breaks a dashboard component or introduces a TypeScript type error. Today nothing in CI runs frontend tests or typechecking. After this feature, CI runs the frontend test suite and typecheck, failing the PR on regressions.

**Why this priority**: Same class of gap as backend, second priority because the report flags it as an equally missing but separate check.

**Independent Test**: Push a branch with a failing Vitest test (or a TypeScript type error) and confirm the new CI job fails; fix it and confirm the job passes.

**Acceptance Scenarios**:

1. **Given** a pull request with a failing frontend test, **When** CI runs, **Then** the frontend test job fails and blocks merge.
2. **Given** a pull request with a TypeScript type error, **When** CI runs, **Then** the typecheck job fails with the error visible in the log.
3. **Given** a pull request where frontend tests and typecheck pass, **When** CI runs, **Then** the job succeeds.

---

### User Story 3 - Existing drift checks remain intact (Priority: P3)

The two existing CI jobs (sqlc-drift, tygo-drift) must keep running exactly as before — this feature adds gates, it doesn't replace or weaken existing ones.

**Why this priority**: Lowest priority because it's a non-regression guarantee, not new value, but still must be verified.

**Independent Test**: Confirm both existing drift jobs are still present and unmodified in the CI workflow after the change, and still fail on injected drift as before.

**Acceptance Scenarios**:

1. **Given** the updated CI workflow, **When** a generated-code drift is introduced (sqlc or tygo), **Then** the corresponding existing drift job still fails as it did before this change.

---

### Edge Cases

- What happens when the backend test job needs Postgres/Redis (per `make test-go`'s `DATABASE_URL`/`REDIS_URL` dependency)? CI must provision these services for the job to run at all.
- What happens if a required service (Postgres/Redis) fails to start in CI? The job must fail clearly rather than hang or silently skip tests.
- How does the system handle a contributor's PR that only touches docs or config with no code changes — do the new jobs still run? Yes; simplicity and predictability are preferred over path-based conditional gating for this cheap fix.
- Lint (`golangci-lint`, ESLint) is explicitly out of scope: no config exists for either, and authoring lint rule sets is a separate, non-cheap decision (see Assumptions).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: CI MUST run the Go test suite (`make test-go` or equivalent) on every push to `master` and every pull request.
- **FR-002**: CI MUST run `go vet` across the Go module on every push to `master` and every pull request.
- **FR-003**: CI MUST run the frontend test suite (`make test-react` or equivalent) on every push to `master` and every pull request.
- **FR-004**: CI MUST run the frontend typecheck (`make typecheck` or equivalent) on every push to `master` and every pull request.
- **FR-005**: CI MUST provision any datastore dependencies (Postgres, Redis) required by the Go test suite so tests execute the same way they do via `make test-db-setup` / `make test-go` locally.
- **FR-006**: A failure in any new job (Go tests, go vet, frontend tests, frontend typecheck) MUST cause the overall CI run to report failure.
- **FR-007**: The two existing drift-check jobs (sqlc-drift, tygo-drift) MUST continue to run unchanged alongside the new jobs.
- **FR-008**: This feature MUST NOT introduce `golangci-lint` or ESLint gating, since no configuration exists for either yet and authoring one is out of scope for this fix.

### Key Entities

- **CI Workflow**: The GitHub Actions definition (currently `API CI` with two jobs) that gates merges; gains new jobs without losing existing ones.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of pull requests with a failing Go test or `go vet` violation are blocked by CI before merge.
- **SC-002**: 100% of pull requests with a failing frontend test or TypeScript type error are blocked by CI before merge.
- **SC-003**: The two pre-existing drift checks continue to pass/fail identically to their pre-change behavior (zero regressions in existing gates).
- **SC-004**: A contributor can identify which check failed (Go test, go vet, frontend test, or typecheck) from the CI job name alone, without opening logs.

## Assumptions

- "Cheap" means wiring CI to run checks and tooling that already exist locally (`make test-go`, `make test-react`, `make typecheck`, `go vet`) — not authoring new lint configs from scratch.
- `golangci-lint` and ESLint gating are deliberately deferred: no config file exists for either, and picking a rule set is a separate scoping decision, not a "cheap" one.
- The Go test suite requires a live Postgres and Redis instance (per existing `Makefile` targets); CI must stand these up as services rather than mocking them, matching current local test setup.
- Frontend tests run via Vitest and typecheck via the existing `pnpm typecheck` / `make typecheck` target; no new frontend tooling is introduced.
- New jobs run on every push/PR (same triggers as existing jobs), not gated by changed file paths, to keep the change simple.
