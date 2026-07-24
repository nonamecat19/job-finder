---

description: "Task list for feature implementation"
---

# Tasks: Health/Readiness Endpoints + Compose Healthchecks

**Input**: Design documents from `/specs/008-health-readiness-checks/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/health-api.md, quickstart.md

**Tests**: Included — plan.md's Testing section explicitly calls for `go test ./internal/httpapi/...` coverage of the new handler.

**Organization**: Tasks are grouped by user story (US1: compose visibility, US2: readiness endpoint, US3: ollama/minio compose wiring) to enable independent implementation and testing.

**Scope note**: `apps/jobspy-sidecar` is excluded — it is being deleted from the repo in the current working tree (see spec.md Assumptions). FR-006 (api compose healthcheck) applies to `docker-compose.prod.yml` only — `docker-compose.yml` (dev) has no `api` service block today (dev runs the api via `make dev` outside compose); adding one is out of scope for this feature.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)

## Path Conventions

Single Go backend project (`apps/api`) + two root-level Docker Compose files — see plan.md's Project Structure.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: One-time prep shared by the api compose healthcheck (US1)

- [X] T001 [P] Add `curl` to the `apt-get install` line in `apps/api/Dockerfile` (needed by US1's api `healthcheck.test`, per research.md Decision 3)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Client plumbing the readiness handler (US2) needs before it can be written. Nothing in US1 or US3 depends on this phase directly — US1 depends on it only transitively, through US2's endpoint existing.

**⚠️ CRITICAL**: Must complete before US2 implementation tasks (T005+)

- [X] T002 Add a `RedisClient redis.UniversalClient` field to `Platform` in `apps/api/cmd/server/platform.go`, built in `buildPlatform` via `p.RedisOpt.MakeRedisClient().(redis.UniversalClient)` (promotes `github.com/redis/go-redis/v9` from indirect to direct import — see research.md Decision 2)
- [X] T003 Add a `MinioReady *minio.Client` field to `Platform` in `apps/api/cmd/server/platform.go`, built in `buildPlatform` from `cfg.MinioEndpoint/MinioAccessKey/MinioSecretKey/MinioUseSSL` (nil when `cfg.MinioEndpoint == ""`, matching the existing "MinIO disabled" convention in `apps/api/internal/storage/minio.go`) — depends on T002 (same file)

**Checkpoint**: `Platform` exposes `DB.Pool` (existing), `RedisClient`, and `MinioReady` — everything the readiness handler needs to ping all three dependencies.

---

## Phase 3: User Story 2 - API exposes a readiness endpoint distinct from liveness (Priority: P1)

**Goal**: `GET /api/health/ready` checks Postgres/Redis/MinIO with a bounded timeout and reports per-dependency status; `GET /api/health` stays pure liveness.

**Independent Test**: Per quickstart.md step 3 — `curl http://localhost:3000/api/health/ready` returns `{"ok":true,"checks":{...}}`; stopping Postgres (quickstart.md step 4) flips it to `503` with `postgres.status:"error"` while `/api/health` still returns `200`.

### Tests for User Story 2

- [X] T004 [P] [US2] Write httptest coverage in `apps/api/internal/httpapi/health_test.go` for: all dependencies ok (`200`, `ok:true`), one dependency erroring (`503`, `ok:false`, named error), and MinIO disabled (`status:"disabled"`, doesn't affect overall `ok`) — per data-model.md ReadinessReport examples

### Implementation for User Story 2

- [X] T005 [US2] Implement `HealthHandler` with `Mount(chi.Router)` in `apps/api/internal/httpapi/health.go`, registering `GET /health/ready`: pings `Platform.DB.Pool.Ping(ctx)`, `Platform.RedisClient.Ping(ctx)`, and `Platform.MinioReady.BucketExists(ctx, bucket)` (skipped/`"disabled"` if `MinioReady == nil`), each bounded by a 2s `context.WithTimeout` (FR-004); returns the `ReadinessReport` JSON shape from data-model.md, `200` if all non-disabled checks ok else `503` (FR-002, FR-003)
- [X] T006 [US2] Wire `httpapi.HealthHandler` into `App`/`composeApp` in `apps/api/cmd/server/compose.go`
- [X] T007 [US2] Add `app.Health.Mount` to the `httpapi.NewRouter(...)` call in `apps/api/cmd/server/servers.go`

**Checkpoint**: User Story 2 is fully functional and testable independently — `/api/health/ready` works with real or locally-stopped dependencies, no compose changes required yet.

---

## Phase 4: User Story 1 - Operator sees accurate container health in `docker compose ps` (Priority: P1)

**Goal**: `docker compose -f docker-compose.prod.yml up` shows the `api` service's real health (not just "running") in `docker compose ps`, driven by the US2 readiness endpoint.

**Independent Test**: Per quickstart.md steps 2 & 4 — `docker compose ps` shows `api` as `healthy`; stopping `postgres` flips `api` to `unhealthy` within one interval, and restarting `postgres` flips it back to `healthy` without a manual container restart (SC-002, SC-004).

**Depends on**: T001 (curl in image) and Phase 3 (readiness endpoint must exist to be curled)

### Implementation for User Story 1

- [X] T008 [US1] Add a `healthcheck:` block to the `api` service in `docker-compose.prod.yml`: `test: ["CMD", "curl", "-f", "http://localhost:3000/api/health/ready"]`, `interval: 5s`, `timeout: 3s`, `retries: 10` (matching the existing Postgres healthcheck convention, per FR-010)
- [X] T009 [US1] Change the `api` service's `depends_on` in `docker-compose.prod.yml` to `condition: service_healthy` for `postgres` and `minio` (leave `redis` at `condition: service_started` — redis has no healthcheck in scope per spec.md Assumptions) (FR-011)

**Checkpoint**: `api`'s compose health status in `docker-compose.prod.yml` accurately reflects readiness; startup ordering honors `postgres`/`minio` health.

---

## Phase 5: User Story 3 - Third-party services (ollama, minio) get standardized health signals (Priority: P2)

**Goal**: `ollama` and `minio` report real compose health using their own existing signals — no new api-side code, independent of US1/US2.

**Independent Test**: Per quickstart.md step 5 — `docker compose exec ollama ollama list` exits 0; the minio `/dev/tcp` probe against `/minio/health/live` returns a line containing `200`; `docker compose ps` shows both `healthy`.

### Implementation for User Story 3

- [X] T010 [P] [US3] Add a `healthcheck:` block to the `ollama` service in `docker-compose.yml`: `test: ["CMD", "ollama", "list"]`, `interval: 5s`, `timeout: 3s`, `retries: 10` (or a longer `start_period` if model loading needs more time — per research.md Decision 3 / FR-010)
- [X] T011 [P] [US3] Add a `healthcheck:` block to the `minio` service in `docker-compose.yml` using the bash `/dev/tcp` probe from research.md Decision 3 / contracts/health-api.md against `/minio/health/live`, `interval: 5s`, `timeout: 3s`, `retries: 10`
- [X] T012 [P] [US3] Add the same `healthcheck:` block as T011 to the `minio` service in `docker-compose.prod.yml`

**Checkpoint**: All user stories independently functional — `docker compose ps` (both files) shows every in-scope service (`postgres`, `api` in prod, `ollama` in dev, `minio` in both) with a real health status (SC-001, SC-003).

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Verify the full feature end-to-end across all three stories

- [X] T013 [P] Run `go test ./internal/httpapi/...` in `apps/api` to confirm `health_test.go` (T004) passes with no regressions
- [X] T014 Run the full quickstart.md validation (steps 1–5) against both `docker-compose.yml` and `docker-compose.prod.yml`, confirming SC-001 through SC-004

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: No dependencies — can start immediately, in parallel with Phase 1
- **User Story 2 (Phase 3)**: Depends on Phase 2 (needs `Platform.RedisClient`/`MinioReady`)
- **User Story 1 (Phase 4)**: Depends on Phase 1 (T001, curl in image) AND Phase 3 (readiness endpoint must exist)
- **User Story 3 (Phase 5)**: No dependency on Phase 2/3/4 — can run any time after project setup, fully in parallel with US1/US2
- **Polish (Phase 6)**: Depends on all desired stories being complete

### User Story Dependencies

- **US2 (P1)**: Independently testable once Phase 2 is done; no dependency on US1 or US3
- **US1 (P1)**: Requires US2's endpoint to exist (curls it) — build US2 first despite equal priority
- **US3 (P2)**: Fully independent of US1 and US2 — touches only `ollama`/`minio` compose blocks

### Parallel Opportunities

- T001 (Setup) and T002/T003 (Foundational) can run in parallel — different files
- T002 and T003 touch the same file (`platform.go`) — not parallel-safe, run sequentially
- T004 (US2 tests) can be written in parallel with nothing else in US2 (it's the first task in that phase, precedes T005)
- T010, T011, T012 (US3) are all `[P]` — different service blocks/files, no shared dependency
- US3 (Phase 5) can be worked entirely in parallel with US1 (Phase 4) and US2 (Phase 3) by a different contributor, since it shares no files

---

## Parallel Example: User Story 3

```bash
# All three US3 tasks touch different files/blocks and have no cross-dependency:
Task: "Add ollama healthcheck to docker-compose.yml"
Task: "Add minio healthcheck to docker-compose.yml"
Task: "Add minio healthcheck to docker-compose.prod.yml"
```

---

## Implementation Strategy

### MVP First (User Story 2 Only)

1. Complete Phase 1 (T001) + Phase 2 (T002–T003)
2. Complete Phase 3: User Story 2 (readiness endpoint)
3. **STOP and VALIDATE**: `curl /api/health/ready` locally, with and without Postgres running
4. This alone delivers the endpoint on-call/tooling can already poll manually, even before any compose wiring

### Incremental Delivery

1. Setup + Foundational → clients ready
2. US2 → readiness endpoint works standalone → validate via curl
3. US1 → api's compose healthcheck goes live in `docker-compose.prod.yml` → validate via `docker compose ps`
4. US3 → ollama/minio compose healthchecks go live → validate via `docker compose ps` + direct probes
5. Polish → full quickstart.md pass across both compose files

### Suggested Team Split

- Developer A: Phase 2 → Phase 3 (US2) → Phase 4 (US1) — these three are a dependency chain
- Developer B: Phase 5 (US3) — fully independent, can start immediately alongside Developer A's Phase 1/2

---

## Notes

- [P] tasks = different files, no dependencies
- Constitution IV (Test Discipline) is satisfied by T004/T013 (`go test` in apps/api's native suite); no cross-service Docker-backed integration test was added since compose YAML changes have no native test harness in this repo (verified manually via T014/quickstart.md instead)
- Commit after each task or logical group
- Stop at any checkpoint to validate a story independently
