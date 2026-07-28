# Research: Asynqmon Queue Monitoring Dashboard

## Decision: Deploy asynqmon as a prebuilt Docker image, not vendored/built from source

**Rationale**: asynqmon (`github.com/hibiken/asynqmon`) publishes an official Docker
image (`hibiken/asynqmon`) that ships the compiled Go binary + bundled React UI. The
project has no existing Go module dependency on asynqmon and doesn't need one — it's an
operational tool, not a library the API imports. Using the image matches how the stack
already runs off-the-shelf tools (`redis:7-alpine`, `ollama/ollama`, `minio/minio`)
rather than building them from source.

**Alternatives considered**:
- *Vendor asynqmon as a Go dependency inside `apps/api` and mount its handler on an
  internal route.* Rejected: pulls a UI+admin tool into the API's dependency graph and
  binary for no functional gain, and would make FR-008 (dev-only exposure) harder to
  guarantee — the route would need to be actively excluded in prod builds instead of
  simply absent from `docker-compose.prod.yml`.
- *Build asynqmon from source in a project Dockerfile.* Rejected: no need to customize
  the tool; adds build time and image-maintenance burden for zero benefit over the
  maintained upstream image.

## Decision: Point asynqmon at the existing `redis` compose service by hostname

**Rationale**: All six asynq workers already connect to the single `redis` service
(`docker-compose.yml`, `redis:7-alpine`, default port 6379, no auth) via `RedisOpt`
built from `REDIS_URL` (see `apps/api/internal/config/config.go`,
`REDIS_URL=redis://localhost:6379/1` in `Makefile`). asynqmon takes a
`--redis-addr` flag (or `REDIS_ADDR`); pointing it at `redis:6379` inside the compose
network reuses the same backend with zero new infrastructure, satisfying FR-001 and the
Assumptions section ("no additional queue backends need to be monitored").

**Alternatives considered**:
- *Separate Redis instance for asynqmon.* Rejected: the six queues live in the shared
  `redis` service; a separate instance would show nothing.

## Decision: Expose asynqmon only in `docker-compose.yml` (dev), not `docker-compose.prod.yml`

**Rationale**: FR-008 requires dev/local-only exposure, and the spec's Assumptions
explicitly choose "restrict network exposure" over "add new auth" since no auth system
currently fronts internal dev tooling in this stack (compare `flaresolverr`, `minio`
console — both dev-facing, unauthenticated-by-default, reachable only via
locally-published ports). Omitting the service entirely from `docker-compose.prod.yml`
is the simplest way to guarantee it never ships to production, with no runtime feature
flag to misconfigure.

**Alternatives considered**:
- *Add asynqmon to prod compose behind basic auth.* Rejected: out of scope per FR-008
  and spec Assumptions; adding auth machinery for a P1-scope feature increases surface
  area without a stated requirement driving it.
- *Add it to prod but firewalled by network policy.* Rejected: relies on deployment-time
  network configuration outside this repo's control; omission from the compose file is
  a stronger, self-contained guarantee.

## Decision: Host port `8090` for the asynqmon web UI

**Rationale**: Checked all `ports:` mappings in `docker-compose.yml` — 5432 (postgres),
6379 (redis), 11434 (ollama), 8191 (flaresolverr), 9000/9001 (minio) — and the API's own
default port (3000, `apps/api/internal/config/config.go`) and dashboard dev port (5173,
`apps/dashboard/vite.config.ts`). asynqmon's own default listen port is 8080, but that
port is already claimed: `docker-compose.prod.yml` maps the dashboard's nginx to host
`8080`, and `README.md` documents `Dashboard: http://localhost:8080` as the full-stack
entry point. Reusing 8080 for asynqmon would collide with that convention the moment
`docker-compose.yml` grows a dashboard service (it currently has none). `8090` is free
across every compose/doc reference and only needs a `--port` override on the asynqmon
container.

**Alternatives considered**:
- *Port 8080 (asynqmon's default).* Rejected after cross-checking `docker-compose.prod.yml`
  and `README.md` — it's reserved for the dashboard stack-wide, not actually free.
- *Any other unused port (e.g. 8092, 9090).* Equally valid; 8090 chosen as the nearest
  free neighbor to asynqmon's own default, easy to remember as "8080 + 10".

## Decision: No new automated test suite; verification via `quickstart.md`

**Rationale**: This feature adds zero application source code (no Go, no TS) — only a
compose service definition. Constitution Principle IV ties `go test`/`vitest` discipline
to changes in `apps/api`/`apps/dashboard` source; there is none here. Verification is a
runnable manual/scripted check (`docker compose config`, bring the stack up, enqueue a
task, confirm dashboard visibility and retry/delete actions) captured in
`quickstart.md`.

**Alternatives considered**: *Add a smoke-test script under `apps/api` that curls the
asynqmon health endpoint.* Considered but not required by any FR; `quickstart.md`
already covers this as a manual step and a dedicated script would be new
process/tooling for a single dev-convenience container.
