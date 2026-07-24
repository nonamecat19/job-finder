# Feature Specification: Health/Readiness Endpoints + Compose Healthchecks

**Feature Branch**: `008-health-readiness-checks`

**Created**: 2026-07-24

**Status**: Draft

**Input**: User description: "Health/readiness endpoints + compose healthchecks on api, ollama, minio, sidecar. Only Postgres has one"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator sees accurate container health in `docker compose ps` (Priority: P1)

An operator running the stack locally or in production wants `docker compose ps` / orchestrator tooling to show whether the api, ollama, and minio containers are actually up and usable, not just that the process started.

**Why this priority**: Postgres is the only service with a healthcheck today, so `depends_on: service_healthy` cannot be used for the other services and operators must guess whether a container that "started" is actually ready to serve traffic.

**Independent Test**: Run `docker compose up`, then `docker compose ps` — every listed service shows a `healthy`/`unhealthy` status (not just `running`), and status flips to `healthy` only once each service can serve requests.

**Acceptance Scenarios**:

1. **Given** the stack is starting up, **When** the api container's dependencies (Postgres, Redis, MinIO) are not yet ready, **Then** the api container's compose health status is `starting`/`unhealthy` until it can serve traffic.
2. **Given** all containers are running normally, **When** an operator runs `docker compose ps`, **Then** api, ollama, and minio all report `healthy`.
3. **Given** the ollama container's model runtime is unresponsive, **When** compose runs its healthcheck, **Then** the container is reported `unhealthy` within one healthcheck interval.

---

### User Story 2 - API exposes a readiness endpoint distinct from liveness (Priority: P1)

A caller (compose healthcheck, load balancer, or on-call engineer) wants to distinguish "the api process is alive" from "the api can actually serve requests" (i.e. its database, cache, and object storage dependencies are reachable).

**Why this priority**: The current `/health` endpoint only proves the HTTP server is up; it does not catch the common failure mode of the api container running while Postgres, Redis, or MinIO are unreachable, which silently breaks the app.

**Independent Test**: Stop the Postgres container while the api container keeps running. Call the readiness endpoint directly — it returns a non-2xx status and identifies Postgres as the failing dependency, while the liveness endpoint still returns 2xx.

**Acceptance Scenarios**:

1. **Given** all api dependencies (Postgres, Redis, MinIO) are reachable, **When** a caller requests the readiness endpoint, **Then** it returns success with per-dependency status.
2. **Given** one dependency (e.g. Redis) is unreachable, **When** a caller requests the readiness endpoint, **Then** it returns a failure status and names the failing dependency, while the liveness endpoint still returns success (process itself is alive).
3. **Given** the api process is running but has not finished startup initialization, **When** a caller requests readiness, **Then** it returns not-ready until initialization completes.

---

### User Story 3 - Third-party services (ollama, minio) get standardized health signals (Priority: P2)

An operator wants ollama's own status signal and minio's existing built-in health endpoint wired into compose.

**Why this priority**: ollama and minio ship their own runtimes; wiring their existing/native health signals into compose is lower effort and lower risk than instrumenting the api, but still closes the "only Postgres has a healthcheck" gap.

**Independent Test**: Query each service's health signal directly (`curl` to minio's health path, ollama's status endpoint) and confirm each responds successfully when the service is up, and compose reflects the same status.

**Acceptance Scenarios**:

1. **Given** minio is running, **When** compose executes minio's healthcheck, **Then** it uses minio's built-in health endpoint and reports `healthy`.
2. **Given** ollama is running and has finished loading, **When** compose executes ollama's healthcheck, **Then** it reports `healthy`.

---

### Edge Cases

- What happens when a dependency is slow to respond rather than fully down (e.g. Postgres under heavy load)? Readiness checks must apply a bounded timeout and report not-ready rather than hanging.
- How does the system handle a dependency that is intermittently flapping? Compose retries/interval settings must avoid marking a service unhealthy on a single transient blip (matches existing Postgres healthcheck pattern of retries).
- What happens during the normal startup race (e.g. api starts before minio has created its bucket)? Readiness must report not-ready rather than crash-looping, and compose `depends_on: condition: service_healthy` should be used where startup ordering matters.
- What happens if a healthcheck command/tool isn't available inside a given image (e.g. minimal distroless-style images lacking `curl`)? The chosen healthcheck mechanism must use a tool already present in that image.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The api service MUST expose a liveness endpoint that returns success whenever the process is running, independent of dependency state.
- **FR-002**: The api service MUST expose a readiness endpoint that checks connectivity to Postgres, Redis, and MinIO, and reports per-dependency status.
- **FR-003**: The readiness endpoint MUST return a non-success HTTP status when any checked dependency is unreachable.
- **FR-004**: The readiness endpoint MUST bound each dependency check with a timeout so a hung dependency cannot hang the endpoint indefinitely.
- **FR-006**: `docker-compose.prod.yml` MUST define a `healthcheck` for the api service, using the readiness endpoint. (`docker-compose.yml`, the dev compose file, has no `api` service block today — dev runs the api via `make dev` outside compose — so this requirement applies only where the api service actually exists.)
- **FR-007**: The compose files MUST define a `healthcheck` for the ollama service, using ollama's own status signal.
- **FR-008**: The compose files MUST define a `healthcheck` for the minio service, using minio's built-in health endpoint.
- **FR-010**: Each new healthcheck MUST follow the existing Postgres healthcheck's interval/timeout/retries convention (5s interval, 3s timeout, 10 retries) unless a service's own startup time requires a longer allowance (e.g. ollama model loading).
- **FR-011**: Services with startup-order dependencies (e.g. api needing Postgres/MinIO ready) MUST use `depends_on` with `condition: service_healthy` on the relevant upstream services, mirroring the existing Postgres `service_healthy` usage.

### Key Entities

- **Liveness check**: Confirms a service process is running and its HTTP server accepts connections; no dependency checks.
- **Readiness check**: Confirms a service can serve real requests, including reachability of its direct dependencies (database, cache, object storage).
- **Compose healthcheck**: Docker Compose's periodic in-container probe (`test`, `interval`, `timeout`, `retries`) that drives the `healthy`/`unhealthy`/`starting` status shown in `docker compose ps` and usable by `depends_on: condition: service_healthy`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After `docker compose up`, every service in the stack (postgres, api, ollama, minio) reports a `healthy` or `unhealthy` status in `docker compose ps` — no service shows only `running` with no health state.
- **SC-002**: When any one of api's dependencies (Postgres, Redis, MinIO) is stopped, the api's readiness check reflects "not ready" and names the failing dependency within one healthcheck interval (≤5s after the dependency becomes unreachable, matching Postgres's existing interval).
- **SC-003**: An operator can determine, without reading logs, which service in a broken stack is the root cause, by reading `docker compose ps` health statuses alone.
- **SC-004**: Restarting a single unhealthy dependency (e.g. Postgres) allows dependent services (api) to automatically transition back to `healthy` without a manual container restart, once `depends_on: condition: service_healthy` is honored on next compose reconcile.

## Assumptions

- `apps/jobspy-sidecar` is being removed from the project (deleted in the current working tree, alongside its Go adapter); it is out of scope for this feature even though the original request mentioned "sidecar". This feature covers api, ollama, and minio only.
- Liveness and readiness are exposed as two distinct HTTP endpoints on the api (extending the existing `/health` liveness endpoint and adding a new readiness endpoint), consistent with common liveness/readiness separation practice.
- ollama and minio are used as their official upstream images; this feature only adds compose-level healthcheck wiring for them (using endpoints/commands they already expose), not new code in those services.
- Redis has no compose healthcheck today and is out of explicit scope per the user's list (api, ollama, minio) — its reachability is still covered indirectly via the api's readiness check (FR-002), but no `healthcheck:` block is required on the redis service itself.
- Tools available inside each image (curl/wget/etc.) for the healthcheck `test` command are assumed present in the respective upstream images.
