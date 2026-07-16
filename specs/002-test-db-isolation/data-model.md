# Data Model: Test Database Isolation

**Date**: 2026-07-16 | **Feature**: test-db-isolation | **Plan**: [plan.md](./plan.md)

## Environment Configuration

The test environment is configured through environment variables defined in `.env` / `.env.example`. The system uses **Makefile-level overrides** to switch between dev and test databases, rather than changing test code.

### Variables

| Variable | Dev Value | Test Value | Purpose |
|---|---|---|---|
| `DATABASE_URL` | `postgresql://jobfinder:${DB_PASSWORD}@localhost:5432/jobfinder` | `postgresql://jobfinder:${DB_PASSWORD}@localhost:5432/jobfinder_test` | PostgreSQL connection string |
| `REDIS_URL` | `redis://localhost:6379` | `redis://localhost:6379/1` | Redis connection string (DB index 1 for test isolation) |
| `TEST_DATABASE_URL` | — | `postgresql://jobfinder:${DB_PASSWORD}@localhost:5432/jobfinder_test` | Documented in `.env.example` as the canonical test DB URL |
| `TEST_REDIS_URL` | — | `redis://localhost:6379/1` | Documented in `.env.example` as the canonical test Redis URL |

### Runtime Binding

The `DATABASE_URL` and `REDIS_URL` variables are consumed at these points:

| Consumer | File | How it reads |
|---|---|---|
| Go server startup | `apps/api/cmd/server/main.go` | `config.Load()` → `cfg.DatabaseURL`, `cfg.RedisURL` |
| Integration tests (TestMain) | `apps/api/internal/db/integration_test.go` | `os.Getenv("DATABASE_URL")` |
| Integration tests (per-function) | `apps/api/internal/matching/integration_test.go` and others | `os.Getenv("DATABASE_URL")` |
| asynq client/server | `apps/api/internal/queue/queue.go` → `cmd/server/main.go` | `config.Load()` → `cfg.RedisURL` → `queue.RedisOpt()` |
| Docker Compose services | `docker-compose.yml`, `docker-compose.prod.yml` | Environment section in service definitions |

### Isolation Boundaries

```
┌────────────────────────────────────────────┐
│           Docker Host                       │
│                                             │
│  ┌─────────────────────────────┐            │
│  │  PostgreSQL (port 5432)     │            │
│  │  ├─ jobfinder (dev DB)      │  persistent │
│  │  └─ jobfinder_test (test DB)│  ephemeral  │
│  └─────────────────────────────┘            │
│                                             │
│  ┌─────────────────────────────┐            │
│  │  Redis (port 6379)          │            │
│  │  ├─ DB 0 (dev queue/data)   │            │
│  │  └─ DB 1 (test queue/data)  │            │
│  └─────────────────────────────┘            │
│                                             │
│  ┌─────────────────────────────┐            │
│  │  Makefile override          │            │
│  │  test-integration:          │            │
│  │    DATABASE_URL=...test     │            │
│  │    REDIS_URL=.../1          │            │
│  │    go test -tags integration│            │
│  └─────────────────────────────┘            │
└────────────────────────────────────────────┘
```

### Schema

The test database shares the exact same schema as the development database — both use the same goose migration files embedded in the Go binary (`apps/api/internal/db/migrations/*.sql`). There is no separate schema definition for the test environment.

### Test Data Lifecycle

1. **Creation**: `make test-db-setup` creates `jobfinder_test` via `CREATE DATABASE` (idempotent)
2. **Migration**: `db.Migrate()` runs at test startup (idempotent — goose tracks applied migrations)
3. **Per-test cleanup**: Each integration test calls `truncateAll()` or table-specific `TRUNCATE ... CASCADE`
4. **Destruction**: `make clean` → `docker compose down -v` removes the entire `pgdata` volume, including the test database
