# Research: Test Database Isolation

**Date**: 2026-07-16 | **Feature**: test-db-isolation | **Plan**: [plan.md](./plan.md)

## Decisions

### D1: PostgreSQL isolation — separate database name, same container

- **Decision**: Use `CREATE DATABASE jobfinder_test` in the existing `pgvector/pgvector:pg16` container.
- **Rationale**: The project already runs a single PostgreSQL container with a persistent `pgdata` volume. Creating a second database within the same server provides complete table-level isolation (different namespace, same server). No second container means no additional memory/CPU overhead, no extra health check, and no port conflict. The `pgvector` extension is available cluster-wide, so the test DB can use embeddings without extra setup.
- **Alternatives considered**:
  - **Second container** (`test-postgres`): Adds complexity, extra health check, duplicate resource usage. No benefit since database-level isolation is equivalent to server-level isolation for this use case.
  - **Schema-level isolation** (same database, different schema): More complex to manage, goose doesn't natively support per-schema migrations without configuration changes.
  - **In-memory SQLite**: Would require rewriting all sqlc queries and losing pgvector support. Not viable.

### D2: Redis isolation — separate DB index

- **Decision**: Use `REDIS_URL=redis://localhost:6379/1` for tests (DB index 1 vs default 0).
- **Rationale**: The existing `queue.RedisOpt()` parser in `apps/api/internal/queue/queue.go` (line 85-89) already supports path-based DB indexing: `u.Path[1:]` is parsed as the DB number. Setting `REDIS_URL` to `redis://localhost:6379/1` routes all asynq client/server connections to a separate key-space. No code changes needed — this works today.
- **Alternatives considered**:
  - **Second Redis container**: Overkill for a key-space isolation need that Redis natively provides via DB index.
  - **Key prefixing**: Would require modifying the asynq client configuration. DB index is cleaner.

### D3: Env override in Makefile, not test-code changes

- **Decision**: Override `DATABASE_URL` and `REDIS_URL` environment variables in the `make test-integration` and `make test-e2e` targets, rather than modifying Go test code to read a `TEST_DATABASE_URL` variable.
- **Rationale**: Every integration test reads `os.Getenv("DATABASE_URL")` directly — either via `config.Load()` (which parses env) or via explicit `os.Getenv("DATABASE_URL")` with a fallback default. By setting these vars in the Makefile's shell command, all existing tests automatically target the test database without any Go code changes. This is a single-point change that affects all test targets consistently.
- **Alternatives considered**:
  - **Add `TEST_DATABASE_URL` env var + fallback in Go code**: Would require touching every integration test and `config.Load()`. More invasive, less maintainable.
  - **Dedicated test config file**: Over-engineered for this project's scale.
  - **Go build tag to switch URLs**: Adds complexity with no benefit over env override.

### D4: Test database creation — `make test-db-setup` target

- **Decision**: Add a `test-db-setup` Makefile target that creates the test database if it doesn't exist, using `docker compose exec -T postgres psql -U jobfinder -c "CREATE DATABASE jobfinder_test;"`. The `test-integration` target depends on this.
- **Rationale**: `createdb` fails silently if the DB already exists (via `2>/dev/null || true`), making it idempotent. The test DB creation is fast (<1s) and uses the same running PostgreSQL container.
- **Alternatives considered**:
  - **Manual creation**: Error-prone, undocumented step.
  - **Docker Compose init script**: Would require modifying the postgres image entrypoint. Over-engineered.

### D5: Docker Compose — no new service, add documentation

- **Decision**: Do not add a separate test service to `docker-compose.yml`. The existing `postgres` and `redis` services serve both dev and test roles.
- **Rationale**: A separate service would duplicate resources and add complexity. The isolation is database-name-level, which is sufficient. Document the approach in the Makefile and `.env.example` instead.
- **Alternatives considered**:
  - **Profile-based test service**: Add a `test` profile to docker-compose.yml that starts a second postgres. Adds resource overhead but provides full filesystem isolation. Not needed per spec assumption ("A single PostgreSQL server is sufficient").
  - **Test-specific docker-compose.test.yml**: Clean separation but adds a compose file. Overkill for the current need.

### D6: E2E test isolation

- **Decision**: Document that e2e tests require the Go API to be started with `DATABASE_URL` pointing to the test database. The `make test-e2e` target overrides the env var, but since the Playwright runner talks to an external API process, the developer must start that process with test env vars.
- **Rationale**: The Playwright e2e tests interact with the application through the browser. The React frontend communicates with the Go API at `localhost:3000`. If the API is running with dev DB credentials (e.g. via `make dev`), e2e test data goes to the dev DB. Full isolation requires the API to be started with test credentials, or the e2e target to start an isolated API instance. This is a known scope edge documented in the spec's assumptions.
- **Alternatives considered**:
  - **e2e target starts its own API**: Would require building and running the Go binary in the test target, adding significant complexity. Deferred as a follow-up.

## Dependencies Confirmed

| Dependency | Version | Used For |
|---|---|---|
| `github.com/pressly/goose/v3` | v3.27.2 | Migrations against test DB — already idempotent |
| `github.com/jackc/pgx/v5` | v5.10.0 | PostgreSQL driver — URL-based, no code change needed |
| `github.com/hibiken/asynq` | (via go.mod) | Redis client/server — `RedisOpt` already supports DB index in URL path |
| `pgvector/pgvector:pg16` | latest | Docker image — extension available cluster-wide |
| `redis:7-alpine` | latest | Docker image — multi-database support built in |

## Existing Patterns Confirmed

- **TestMain pattern** (`internal/db/integration_test.go`): Reads `DATABASE_URL` from env, calls `db.Migrate(dsn)`, sets up a `testDB` package variable, provides `truncateAll(t)` helper. Will automatically target the test DB when env is overridden.
- **Per-function pattern** (`internal/matching/`, `internal/generation/`, `internal/httpapi/`): Each test reads `os.Getenv("DATABASE_URL")`, calls `db.Migrate(dsn)`, creates a local `testDB`, truncates relevant tables. Same env-override approach applies.
- **Goose migration embedding**: `db.Migrate()` uses `//go:embed migrations/*.sql`. The same embedded files apply to any database — test or dev. No migration duplication.
