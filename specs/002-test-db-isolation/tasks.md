---
description: "Task list for test-db-isolation feature implementation"
---

# Tasks: Test Database Isolation

**Input**: Design documents from `/specs/002-test-db-isolation/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md

**Tests**: Local validation commands are included. No automated test tasks — this is infrastructure whose correctness is verified by running the existing test suites after the changes.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add test database configuration variables so the environment is prepared before any Makefile changes.

- [x] T001 Add `TEST_DATABASE_URL` and `TEST_REDIS_URL` to `.env.example` with documentation comment

  - Add after the existing `DATABASE_URL` line:
    ```env
    # Test database (separate DB name in same Postgres container, separate Redis DB index)
    TEST_DATABASE_URL=postgresql://jobfinder:${DB_PASSWORD}@localhost:5432/jobfinder_test
    TEST_REDIS_URL=redis://localhost:6379/1
    ```

  - **File**: `.env.example`
  - **FR**: FR-008
  - **Story**: US1

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before user stories can be verified.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T002 Add `test-db-setup` Makefile target that creates the test database
- [x] T003 Update `test-integration` Makefile target to use test database and Redis
- [x] T004 [P] Update `test-e2e` Makefile target to use test database env vars
- [x] T005 [P] Update `test-go` target to use test database env vars for any DB-touching tests

**Checkpoint**: Foundation ready — all Makefile changes are in place. Run `make test-db-setup` then `make test-integration` to validate the core isolation mechanism.

---

## Phase 3: User Story 1 — Integration Test Isolation (Priority: P1) 🎯 MVP

**Goal**: Integration tests run against a dedicated PostgreSQL database, leaving the development database untouched.

**Independent Test**: Run `make test-integration`, then query both databases — dev DB should be unchanged, test DB should have been used.

- [x] T006 Verify `make test-db-setup` creates the test database correctly

  - Run: `make up && make test-db-setup`
  - Verify:
    ```sh
    docker compose exec -T postgres psql -U jobfinder -l | grep jobfinder_test
    ```
  - If the DB already exists, the command should be a no-op (idempotent).

- [x] T007 Run integration tests end-to-end and verify isolation

  - Run: `make test-integration`
  - Verify all existing integration tests pass (SC-001).
  - Verify dev DB is untouched (SC-002):
    ```sh
    docker compose exec -T postgres psql -U jobfinder -d jobfinder -c "SELECT count(*) FROM \"Job\";"
    ```
  - The count should match the count before the test run.

**Checkpoint**: US1 is verified — integration tests run against jobfinder_test, dev DB is unchanged.

---

## Phase 4: User Story 2 — Redis Queue Isolation (Priority: P1)

**Goal**: Test Redis operations use a separate DB index, preventing test queue entries from polluting development queue processing.

**Independent Test**: Run integration tests, then inspect Redis DB 0 (dev) — it should show no test-related entries.

- [x] T008 Verify Redis DB index isolation after test run

  - After `make test-integration` completes:
    ```sh
    # Check dev Redis (DB 0) — should contain only dev queue entries
    docker compose exec -T redis redis-cli -n 0 KEYS '*' | wc -l

    # Check test Redis (DB 1) — may contain leftover test queue entries
    docker compose exec -T redis redis-cli -n 1 KEYS '*' | wc -l
    ```
  - FR-002 requires that DB 0 has no test-injected keys. DB 1 may have test data (that's expected and isolated).

**Checkpoint**: US2 is verified — Redis queue isolation works.

---

## Phase 5: User Story 3 — Go Unit Test DB Safety (Priority: P2)

**Goal**: Go unit tests (`go test ./...`) that opt-in to database access target the test database, not the dev database.

**Independent Test**: Run `make test-go` and confirm no changes to the development database.

- [x] T009 Run `make test-go` and verify no DB contamination

  - Run: `make test-go`
  - Since most Go tests use mocks, the env override is a safety net.
  - Verify: Check that `make test-go` exit code is 0 and produces no unexpected output.
  - If any test currently connects to the dev DB via `DATABASE_URL`, it will now connect to `jobfinder_test`. Verify the test database has been migrated (goose will auto-migrate on first connection).

**Checkpoint**: US3 is verified — unit tests with DB access target the test database.

---

## Phase 6: User Story 4 — Clean Test Data Between Runs (Priority: P2)

**Goal**: The test database resets between runs, preventing stale data from affecting test determinism.

**Independent Test**: Run `make test-integration` twice — both runs should produce identical results.

- [x] T010 Verify test run idempotency

  - Run `make test-integration`, capture output hash.
  - Run `make test-integration` again, capture output hash.
  - Compare: both output hashes should match (modulo timestamps). Focus on pass/fail counts being identical.

**Checkpoint**: US4 is verified — test runs are idempotent, stale data doesn't accumulate.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Remaining tasks to complete the feature.

- [x] T011 Update `test-lint` target to also use test database env vars (for consistency)

  - The `test-lint` target currently runs `test-go test-react test-python`. Since `test-go` now has env overrides, `test-lint` inherits that. Ensure `test-lint` works correctly:
    ```makefile
    test-lint: test-go test-react test-python
    ```

  - **File**: `Makefile` (verify, likely no change needed)

- [x] T012 Run quickstart validation scenarios end-to-end

  - Execute each scenario in [quickstart.md](./quickstart.md):
    1. Integration tests target isolated database
    2. Test database is clean on each run
    3. Test database failure is loud (drop DB, run tests — should fail with connection error)
    4. Dev data is unchanged after test run
    5. Redis queue isolation

- [x] T013 Update `.env` (user's active file) with test database variables

  - Add `TEST_DATABASE_URL` and `TEST_REDIS_URL` to `.env` so the developer has the canonical values documented locally. Use the same values as `.env.example`.
  - **File**: `.env`
  - Note: `.env` is user-specific and gitignored — this task is optional but recommended for consistency.

- [x] T014 Document e2e test isolation gap

  - Update [quickstart.md](./quickstart.md) "Known Gaps" section if not already documented:
    - Full e2e isolation requires starting the Go API separately with test env vars
    - This is a manual step; automating it is deferred

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — single `.env.example` edit
- **Foundational (Phase 2)**: Depends on Setup — all Makefile changes are additive
- **User Stories (Phases 3-6)**: All depend on Foundational phase completion. Can be run in any order since they are verification tasks, not code changes.
- **Polish (Phase 7)**: Depends on all user stories being verifiable

### Execution Order

1. Complete Phase 1: `.env.example` changes
2. Complete Phase 2: All 4 Makefile changes (T002-T005, in order)
3. Complete Phase 3: Verify US1 (T006-T007)
4. Complete Phase 4: Verify US2 (T008)
5. Complete Phase 5: Verify US3 (T009)
6. Complete Phase 6: Verify US4 (T010)
7. Complete Phase 7: Polish tasks (T011-T014)

### Parallel Opportunities

- T004 and T005 (test-e2e and test-go Makefile updates) can run in parallel
- T006-T010 (verification tasks) run sequentially by nature
- T011-T014 (polish) can run in parallel

## Notes

- [P] tasks = different files, no dependencies
- No Go, TypeScript, or Python source code is modified in this feature
- All changes are to repository-root config files: `Makefile`, `.env.example`
- Verification relies on running the existing test suites against the new test database
- Commit after each logical group of tasks
