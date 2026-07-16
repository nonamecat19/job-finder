# Feature Specification: Test Database Isolation

**Feature Branch**: `002-test-db-isolation`

**Created**: 2026-07-16

**Status**: Draft

**Input**: User description: "lets divide testing environment from real. all test should not affect real data in db container"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run integration tests without data contamination (Priority: P1)

As a developer, I want to run integration tests against a dedicated test database so that my test runs never modify or pollute the real development data that I interact with through the dashboard.

**Why this priority**: Data contamination is the core problem being solved — without this, every test run risks corrupting real job records, applications, profiles, and embeddings that are costly to regenerate.

**Independent Test**: Can be verified by running the existing integration test suite (`make test-integration`) and confirming that:
1. The development database in the `postgres` container shows no changes after test execution
2. All test data resides in a completely separate database or container

**Acceptance Scenarios**:

1. **Given** a running development PostgreSQL container with real job data, **When** the integration test suite runs, **Then** no test-created records appear in the development database.
2. **Given** the developer has real data in the development database, **When** they query it after running tests, **Then** all records (jobs, applications, profiles, match results, subscriptions, generated documents) are unchanged.

---

### User Story 2 - Test Redis queue isolation (Priority: P1)

As a developer, I want test runs to use a separate Redis database or instance so that test jobs don't leak into or interfere with development queue processing.

**Why this priority**: The asynq queue processes jobs asynchronously — test enqueued jobs could trigger real side effects (enrichment, LLM calls, document generation) if they share a Redis namespace with development.

**Independent Test**: Can be verified by checking that after running integration tests, the development Redis queue shows no test-related entries and no unexpected processing occurred.

**Acceptance Scenarios**:

1. **Given** the development Redis is running with active queue entries, **When** integration tests enqueue and process test jobs, **Then** the development queue entries remain untouched.
2. **Given** the asynq queue is configured for a test run, **When** tests complete, **Then** no test-injected jobs appear in the development queue's inspection views.

---

### User Story 3 - Run Go unit tests that touch DB without side effects (Priority: P2)

As a developer, I want even unit tests that optionally touch the database to target the test database, so I can run all test levels (unit, integration, e2e) without worrying about the environment.

**Why this priority**: Lower priority because most Go unit tests use mocks (`apps/api/internal/...` tests don't touch the DB), and the primary concern is the integration suite. However, if any test uses the DATABASE_URL, it should point to the test DB by default.

**Independent Test**: Can be verified by running `make test-go` with database-involved tests and confirming no change to the development database.

**Acceptance Scenarios**:

1. **Given** the `DATABASE_URL` environment variable points to the test database, **When** any test connects to it, **Then** it connects to the isolated test database, not the development database.

---

### User Story 4 - Clean test data between runs (Priority: P2)

As a developer, I want the test database to be recreated or reset between test runs so that test results are deterministic and not influenced by stale data from previous runs.

**Why this priority**: Without clean state, tests become order-dependent and flaky — a test that passes in isolation may fail when run after another test that left data behind.

**Independent Test**: Can be verified by running the integration suite twice and observing identical results.

**Acceptance Scenarios**:

1. **Given** the test database already contains data from a previous run, **When** a new test run starts, **Then** the database is reset to a clean state (all tables empty or freshly migrated).
2. **Given** migrations are applied to the test database, **When** a migration is added or changed, **Then** the test environment runs migrations automatically before tests.

---

### Edge Cases

- What happens when both the development and test databases are in the same PostgreSQL container? — They must use different database names so DROP/CREATE operations on the test database never affect the development database.
- What happens when the test database connection fails? — Tests should fail fast with a clear error message indicating the test database is unavailable, rather than silently falling back to the development database.
- What happens when the developer runs both development and tests simultaneously? — Both must operate independently without conflicts, even when both are performing write operations.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide a dedicated test PostgreSQL database that is entirely separate from the development database, either via a second container or a distinct database within the same container.
- **FR-002**: System MUST provide a dedicated test Redis database (separate DB index or separate container) that does not share queue/state with development Redis.
- **FR-003**: The test environment MUST apply database migrations from scratch before each test run to ensure a known starting state.
- **FR-004**: The test environment MUST reset or drop/recreate the test database between runs to prevent cross-run data contamination.
- **FR-005**: The `make test-integration` target MUST automatically target the test database and fail if the test database cannot be reached.
- **FR-006**: The `make test` / `make test-go` targets MUST either default to the test database for DB-touching tests or use mocks that never reach any database.
- **FR-007**: The `docker-compose.yml` file MUST include a test database service definition (or document how to start one) so developers can bring up the test environment with a single command.
- **FR-008**: Configuration for the test database (connection string, credentials, port) MUST NOT be the same as the development database configuration, and both MUST coexist in `.env` and `.env.example`.
- **FR-009**: Test data MUST be ephemeral — no test data persists on disk volumes used by the development database.
- **FR-010**: The test environment MUST support running migrations (goose) against the test database independently of the development database.

- **FR-011**: The e2e test suite (Playwright) MUST also target the test database infrastructure when run against a real backend, so all tests — regardless of level — are isolated from development data.

### Key Entities *(include if feature involves data)*

- **Test PostgreSQL Database**: A dedicated database (separate name or container) containing all tables identical in schema to the development database but populated only with test fixtures and ephemeral test data.
- **Test Redis Database**: A separate Redis database index (e.g., DB 1 vs DB 0) or a separate Redis container for test queue isolation.
- **Environment Configuration**: Variables (`DATABASE_URL`, `REDIS_URL`, `TEST_DATABASE_URL`, `TEST_REDIS_URL`) that control which database each environment targets.
- **Docker Compose Service**: A service definition for the test database infrastructure that can be started alongside or independently of the development services.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All existing integration tests pass with no changes when run against the test database instead of the development database.
- **SC-002**: After running the full integration test suite, the development database shows zero changes in any table compared to its state before the test run.
- **SC-003**: The time to set up a clean test database (create + migrate) is under 30 seconds.
- **SC-004**: A developer can run `make test-integration` with zero configuration beyond what is documented in `.env.example`, and it safely targets the test database.
- **SC-005**: Running `make test-integration` while the development database is offline produces a clear error message within 5 seconds, not a silent fallback.

## Assumptions

- The development database contains real or representative data the user has collected (jobs from Adzuna, Djinni, JobSpy) that would be costly or time-consuming to re-acquire if corrupted.
- Integration tests are run locally (not in CI) and target a Docker Compose-managed Postgres/Redis, consistent with the existing `make test-integration` approach.
- The existing test suite uses the `DATABASE_URL` environment variable to connect; changing how this variable is resolved for tests is acceptable.
- A single PostgreSQL server is sufficient — test isolation can be achieved through a separate database name rather than a separate container, reducing resource usage.
- The test database does not need pgvector extension or seed data beyond what migrations create, since integration tests set up their own data.
- The Playwright e2e tests run against a full production-like stack and may need their own database consideration — scoped to the test infrastructure definition, not the e2e test behavior itself.
