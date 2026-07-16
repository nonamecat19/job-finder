# Quickstart: Test Database Isolation

**Date**: 2026-07-16 | **Feature**: test-db-isolation | **Plan**: [plan.md](./plan.md)

## Prerequisites

- Docker Compose stack running (`make up`)
- `.env` file with `DB_PASSWORD` set (same as dev)
- Go 1.26+ toolchain

## Validation Scenarios

### Scenario 1: Integration tests target isolated database

**Setup** (one-time):
```sh
make test-db-setup
```

**Run**:
```sh
make test-integration
```

**Expected**: Tests pass. The development database (`jobfinder`) is completely untouched.

**Verify isolation**:
```sh
# Check dev DB — should have no test data
docker compose exec -T postgres psql -U jobfinder -d jobfinder -c "SELECT count(*) FROM \"Job\";"
# Should show only your real/seed jobs (e.g. 150)

# Check test DB — should have been used by tests
docker compose exec -T postgres psql -U jobfinder -d jobfinder_test -c "SELECT count(*) FROM \"Job\";"
# Should show whatever test data was created (or empty if tests cleaned up)
```

### Scenario 2: Test database is clean on each run

**Run** twice:
```sh
make test-integration
make test-integration
```

**Expected**: Both runs produce identical results. The second run starts from a clean state regardless of what the first run left behind.

**Verify**:
```sh
# The test-idempotency check — compare output hashes
make test-integration 2>&1 | sha256sum > /tmp/test-run1.txt
make test-integration 2>&1 | sha256sum > /tmp/test-run2.txt
diff /tmp/test-run1.txt /tmp/test-run2.txt
# Should produce no output (identical)
```

### Scenario 3: Test database failure is loud

**Run with test DB not created**:
```sh
# Drop the test database first
docker compose exec -T postgres psql -U jobfinder -c "DROP DATABASE IF EXISTS jobfinder_test;"
make test-integration
```

**Expected**: Tests fail with a connection error (e.g. `database "jobfinder_test" does not exist`). No silent fallback to dev database.

### Scenario 4: Dev data is unchanged after test run

**Run and verify dev DB integrity**:
```sh
# Snapshot dev DB row counts before
docker compose exec -T postgres psql -U jobfinder -d jobfinder -c "
  SELECT schemaname, tablename, n_live_tup FROM pg_stat_user_tables ORDER BY tablename;
" > /tmp/dev-before.txt

make test-integration

# Snapshot dev DB row counts after
docker compose exec -T postgres psql -U jobfinder -d jobfinder -c "
  SELECT schemaname, tablename, n_live_tup FROM pg_stat_user_tables ORDER BY tablename;
" > /tmp/dev-after.txt

diff /tmp/dev-before.txt /tmp/dev-after.txt
# Should produce no output — dev DB is untouched
```

### Scenario 5: Redis queue isolation

**Run and check Redis**:
```sh
make test-integration

# Check dev Redis (DB 0) — should have no test noise
docker compose exec -T redis redis-cli -n 0 DBSIZE
# Should be your normal queue size (e.g. 15-20 keys)

# Check test Redis (DB 1) — may have leftover test keys
docker compose exec -T redis redis-cli -n 1 DBSIZE
# Should be 0 or small (depends on test cleanup)
```

## Commands Reference

| Command | Description |
|---|---|
| `make test-db-setup` | Create test database (idempotent) |
| `make test-integration` | Run integration tests against test database |
| `make test-e2e` | Run e2e tests (requires API started with test DB — see note below) |
| `make test-go` | Run Go unit tests (no database dependency) |
| `make test-lint` | Run all test suites |

## Pre-existing Test Bugs Fixed

During implementation, the clean isolated test database exposed several pre-existing bugs that had been masked by parallel-test data races and dev-DB noise:

| Bug | File | Fix |
|-----|------|-----|
| 14 tests used `t.Parallel()` with shared global DB — concurrent truncate/insert/assert leaked data | `apps/api/internal/db/integration_test.go` | Removed `t.Parallel()` — tests now run sequentially |
| `TestStatsQueries` used `NowTimestamp()` directly — Postgres `now()` (transaction time) was always before Go wall clock, so `ingestedAt >= NowTimestamp` was always false | `apps/api/internal/db/integration_test.go:1069` | Offset timestamp by −5 seconds before passing to query |
| `TestJobListQueries` passed bare `"Go"` to ILIKE (no `%` wildcards) — exact match fails for `"Backend Go Dev"` | `apps/api/internal/db/integration_test.go:1162` | Changed query to `"%Go%"` |
| `TestActivityList` omitted required `Meta` field — sqlc INSERT included `meta` column but test passed `nil` | `apps/api/internal/httpapi/activity_test.go:39` | Added `Meta: []byte("{}")` |

## Known Gaps

- **E2E test isolation**: The `make test-e2e` target overrides `DATABASE_URL` and `REDIS_URL`, but the Playwright runner talks to a Go API process that must be started separately. To run e2e tests against the test database, start the API explicitly:
  ```sh
  # Terminal 1: Start API with test DB
  cd apps/api && DATABASE_URL=postgresql://jobfinder:change-me@localhost:5432/jobfinder_test \
    REDIS_URL=redis://localhost:6379/1 \
    go run ./cmd/server

  # Terminal 2: Run e2e tests
  make test-e2e
  ```
  This is a manual step until automated in a follow-up.
- **React tests**: `make test-react` has pre-existing failures (duplicate nav-link roles, text-spacing in `App.test.tsx`, `JobDetailPage.test.tsx`) unrelated to DB isolation. These existed before this feature.
