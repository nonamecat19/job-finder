# Implementation Plan: Test Database Isolation

**Branch**: `002-test-db-isolation` | **Date**: 2026-07-16 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-test-db-isolation/spec.md`

## Summary

Add a dedicated test database (PostgreSQL) and test Redis (separate DB index) so that all test levels — unit, integration, e2e — run against isolated data that never reaches the development database's persistent volume. The approach is **environment-variable override in the Makefile**, not test-code changes: existing integration tests already read `DATABASE_URL` / `REDIS_URL` from the process environment, so overriding those during `make test-*` targets is sufficient. The test database is created as a separate database name in the same PostgreSQL container; Redis uses DB index 1.

## Technical Context

**Language/Version**: Go 1.26 (`apps/api`), Python 3.12+ (`apps/jobspy-sidecar`), TypeScript/React 19 (`apps/dashboard`)

**Primary Dependencies**: `github.com/jackc/pgx/v5` (Postgres driver), `github.com/pressly/goose/v3` (migrations), `github.com/hibiken/asynq` (Redis-backed task queue)

**Storage**: PostgreSQL 16 + pgvector (development DB `jobfinder`, test DB `jobfinder_test`), Redis 7 (development DB 0, test DB 1). Both in the same containers, isolated by database name / Redis index.

**Testing**: `go test -tags integration ./...` for DB-touching Go tests; `vitest` for dashboard unit tests (jsdom, no DB); `pytest` for sidecar; Playwright for e2e. All Go integration tests read `DATABASE_URL` directly from the process environment — no wrapper, no test config struct.

**Target Platform**: Linux (Docker Compose, single-user self-hosted deployment).

**Project Type**: Web service (Go API + React SPA + Python sidecar, pnpm monorepo).

**Performance Goals**: Test DB creation + migration under 30s (SC-003). No throughput requirements — this is developer tooling.

**Constraints**: 
- The `REDIS_URL` parsing in `internal/queue/queue.go` already supports a path-based DB index (e.g. `redis://localhost:6379/1` → DB 1). No asynq code changes needed.
- Goose migrations are embedded (`//go:embed`) and idempotent — running `Migrate` against the test DB automatically bootstraps it.
- A single PostgreSQL server serves both dev and test via different database names; the `pgdata` volume is shared but database-level isolation guarantees no cross-contamination.

**Scale/Scope**: One new Makefile target, env var additions, Docker Compose profile or setup command, zero Go code changes to test files.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| **I. No Auto-Apply, Ever** | ✅ PASS | Infrastructure change only — no code path touches application/submission logic. |
| **II. Grounded Generation** | ✅ PASS | No LLM or generation pipeline involvement. Test data is ephemeral fixtures, not fabricated credentials. |
| **III. Typed Contracts** | ✅ PASS | No cross-language boundary changes. Database schema is identical between dev and test DB — same migrations, no new types. |
| **IV. Test Discipline** | ✅ PASS | This change *strengthens* test discipline by ensuring integration tests never contaminate real data. Uses the same `go test -tags integration` pattern; `make test-lint` still passes per the constitution. |
| **V. Local-First, Self-Hosted** | ✅ PASS | All test infrastructure runs locally via Docker Compose. No external service dependency. |

**Post-Phase-1 re-check**: ✅ Still passing. The design adds no new code to any app module, no external dependency, and no third-party API. The test database lives in the same local container, respecting the self-hosted constraint.

**No violations. Complexity Tracking section omitted.**

## Project Structure

### Documentation (this feature)

```text
specs/002-test-db-isolation/
├── plan.md              # This file
├── research.md          # Phase 0: technical research on isolation approach
├── data-model.md        # Phase 1: database/Redis naming and env vars
├── quickstart.md        # Phase 1: validation guide
├── contracts/           # Phase 1: no external contracts (internal infra change)
└── tasks.md             # Phase 2 output (/speckit.tasks — NOT created here)
```

### Source Code (repository root)

```text
.
├── Makefile                     # MODIFIED — test-integration, test-e2e use test DB
├── .env.example                 # MODIFIED — add TEST_DATABASE_URL, TEST_REDIS_URL
├── docker-compose.yml           # MODIFIED — add test-db profile or setup service
└── apps/api/internal/
    └── db/
        └── integration_test.go  # N/A — tests stay unchanged (env override approach)
```

**Structure Decision**: This is a developer-tooling/infrastructure feature. All changes are confined to the repository root (Makefile, docker-compose.yml, .env.example). No Go, TypeScript, or Python source code is modified. The test files themselves continue to read `DATABASE_URL` from the environment — only the value changes between dev and test runs.

## Key Design Decisions

Full reasoning in [research.md](./research.md). Summary:

1. **Same container, separate database name**. PostgreSQL already runs in a single container with a persistent `pgdata` volume. Creating a second database (`jobfinder_test`) within the same server provides full table-level isolation with zero additional resource cost. No second container, no volume duplication.

2. **Makefile env override, not test-code changes**. Every integration test reads `os.Getenv("DATABASE_URL")` or uses `db.Migrate(dsn)` where `dsn` comes from that env var. Rather than modifying 10+ test files to add a `TEST_DATABASE_URL` fallback, the Makefile pre-sets `DATABASE_URL` and `REDIS_URL` to the test values when running test targets. This is a single-point change with zero test-code churn.

3. **Redis isolation via DB index 1**. The existing `queue.RedisOpt()` helper already parses `redis://host:port/db` format and sets `asynq.RedisClientOpt.DB`. Using `REDIS_URL=redis://localhost:6379/1` for tests provides full key-space isolation. The asynq `goose_db_version`-equivalent internal keys are namespaced per DB index.

4. **Go test flag for fast failure**. When the test database doesn't exist, `psql` will fail the connection — which already triggers `t.Fatalf(...)` in every integration test's setup path. No additional connectivity check needed; the existing error path is sufficient (SC-005).

5. **E2E test isolation via the same override**. Playwright e2e tests start the Vite dev server and communicate with the Go API at `localhost:3000`. The API must be running independently — if the developer starts it with `make dev` (which reads `.env` directly), it uses the dev DB. E2E tests that require test DB isolation need the API started with test env vars, or the e2e target could start a separate API instance. Documented in quickstart.md as a known gap; the primary scope is integration-test isolation.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Developer accidentally runs `make test-integration` while the dev DB is the only one up, causing truncation of real data | Low | The Makefile override is explicit: `DATABASE_URL=postgresql://...jobfinder_test` targets a different database name. Even if the test DB doesn't exist, `go test` fails fast — it won't silently fall back to the dev DB. |
| Test database out of sync with migration schema | Low | `db.Migrate()` is idempotent and called at the top of every integration test setup. A new migration is automatically applied on first test run after the migration is added. |
| Redis DB 1 already in use by another process | Low | If the user runs other software on Redis DB 1, they can override `TEST_REDIS_URL` in their `.env`. The design documents this flexibility. |
| Goose migration fails on test DB creation because `pgvector` extension not available | Low | The same `pgvector/pgvector:pg16` image powers both databases. The extension is available cluster-wide, not per-database. |
| E2E tests still hit dev DB when API is started separately | Medium | Documented in quickstart.md. For full isolation, the developer must start the API with `DATABASE_URL=postgresql://...jobfinder_test` before running e2e tests. A follow-up task could automate this. |

## Phase Status

- [ ] Phase 0: research.md — resolve unknowns, document approach
- [x] Phase 1: data-model.md, quickstart.md (contracts skipped — internal infra change, no external interfaces)
- [x] Constitution re-check post-design — passing
- [ ] Phase 2: tasks.md — run `/speckit.tasks`
