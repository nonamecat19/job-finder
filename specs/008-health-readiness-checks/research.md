# Research: Health/Readiness Endpoints + Compose Healthchecks

## Decision 1: Liveness vs readiness split on the api

**Decision**: Keep `GET /api/health` (existing) as pure liveness (process up, no dependency checks). Add `GET /api/health/ready` as a new readiness endpoint that pings Postgres, Redis, and MinIO.

**Rationale**: Spec FR-001–FR-004 explicitly require two distinct signals. The existing `/health` handler (`apps/api/internal/httpapi/router.go:91`) already matches the liveness contract (always 200 while the process is up) — no reason to change its behavior or callers. Nesting readiness under `/health/ready` keeps both under one logical path prefix and avoids a naming collision with any future `/ready` top-level route.

**Alternatives considered**:
- Making `/health` itself check dependencies — rejected: would break the existing liveness contract implicitly relied on by anything currently polling `/health` as "process alive."
- A separate top-level `/api/ready` — functionally equivalent; `/health/ready` was chosen only for path locality with the existing health namespace, no material difference.

## Decision 2: How the api readiness endpoint checks each dependency

**Decision**:
- Postgres: `platform.DB.Pool.Ping(ctx)` (pgxpool already exposes this; reuses the exact pool the app runs on, no new connection type).
- Redis: promote `github.com/redis/go-redis/v9` (already an indirect dependency via `hibiken/asynq`) to a direct import; build a client via `p.RedisOpt.MakeRedisClient().(redis.UniversalClient)` once at startup and call `.Ping(ctx)` in the handler.
- MinIO: reuse `github.com/minio/minio-go/v7`'s existing client shape (`internal/storage/minio.go`) but keep the readiness path independent of the optional `storage.MinioStore` (which is only constructed when `cfg.MinioEndpoint != ""` inside `composeGeneration`, not on `Platform`). Build a small dedicated `minio.Client` for readiness from `cfg.MinioEndpoint/AccessKey/SecretKey/UseSSL` and call `.BucketExists(ctx, cfg.MinioBucket)` — this both confirms connectivity and confirms the target bucket is reachable, which is what the api actually depends on for document generation.

**Rationale**: All three checks reuse an existing client library already in `go.mod`no new dependencies. Each is a single round-trip call with a natural context-based timeout applied by the handler (FR-004), matching the "reuse what's already wired" pattern the rest of `apps/api` follows (e.g. `composeGeneration` reusing `cfg.Minio*` fields).

**Alternatives considered**:
- Skipping MinIO when `cfg.MinioEndpoint == ""` (uploads disabled) — kept as a real case: readiness reports MinIO as `"disabled"` rather than failing, since the api is fully functional without it (per `internal/storage` comments: "MinioEndpoint disables uploads (files stay on DocumentsDir only)"). This is a deliberate deviation from spec's literal FR-002 wording ("checks connectivity to Postgres, Redis, and MinIO") to avoid a false negative when MinIO is intentionally not configured; documented in data-model.md.
- Using a raw TCP dial instead of protocol-aware pings — rejected: TCP-only checks miss cases where the port is open but the service can't actually serve (e.g. Postgres in recovery mode), which defeats the purpose of readiness vs. liveness.

## Decision 3: Compose healthcheck mechanism per service (no curl/wget in upstream images)

**Investigated by running `docker run --rm --entrypoint sh <image> -c '...'` against both images locally.**

- **ollama/ollama:latest**: Ubuntu 24.04-based; `/bin/sh` is bash; no `curl`, `wget` found. The `ollama` CLI itself is on `PATH` and talks to the local API server (`localhost:11434`) to serve any command, including `ollama list`. This is the community-documented workaround for this exact gap and requires no extra tooling.
  - **Decision**: `healthcheck.test: ["CMD", "ollama", "list"]`.
- **minio/minio:latest**: minimal image (no `curl`, `wget`, `mc`, or even `grep`/`which`); `/bin/sh` is bash 5.1, and bash's `/dev/tcp` pseudo-device works (verified: attempting to connect to a closed port returned "Connection refused" from bash itself, confirming `/dev/tcp` is functional, not silently ignored).
  - **Decision**: `healthcheck.test` uses a `bash -c` script that opens `/dev/tcp/127.0.0.1/9000`, writes a raw `GET /minio/health/live HTTP/1.1` request, reads the first response line, and checks it contains `200` — all with bash builtins only (no `grep`, since it's absent):
    ```sh
    CMD-SHELL exec 3<>/dev/tcp/127.0.0.1/9000 && printf 'GET /minio/health/live HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n' >&3 && read -r line <&3 && [[ "$line" == *200* ]]
    ```
  - This exercises MinIO's own documented `/minio/health/live` liveness path (the endpoint MinIO itself recommends for container health), just without a userspace HTTP client.
- **api** (`apps/api/Dockerfile`, `python:3.12-slim-bookworm` base): `curl` is not currently installed, but the Dockerfile already runs `apt-get install` for other tools (chromium, poppler-utils) — adding `curl` there is a one-line, low-risk change and keeps the healthcheck command simple/standard (`curl -f http://localhost:3000/api/health/ready || exit 1`), consistent with how Postgres's own healthcheck uses its native `pg_isready` CLI.
  - **Decision**: add `curl` to the existing `apt-get install` line in `apps/api/Dockerfile`; healthcheck runs `curl -f http://localhost:PORT/api/health/ready`.

**Alternatives considered**: Writing a tiny Go healthcheck subcommand baked into the `server` binary (`server healthcheck`) to avoid adding `curl` — rejected as unnecessary complexity for a one-line Dockerfile change; revisit only if `curl` turns out to be undesirable for image size/security reasons.

## Decision 4: `depends_on` / startup ordering

**Decision**: In both compose files, change `api`'s `depends_on` to use `condition: service_healthy` for `postgres` and `minio` (both will have healthchecks after this feature), and leave `redis` at `condition: service_started` (redis has no healthcheck in scope per spec Assumptions, and `service_started` is the existing behavior — no regression).

**Rationale**: Matches FR-011 and the existing precedent already in `docker-compose.prod.yml` (`postgres: condition: service_healthy` is already there for the `api` service). `docker-compose.yml` (dev) currently has no `api` service block at all (dev runs the api via `make dev` outside compose, based on the file's contents) — confirm during implementation whether dev compose needs an `api` entry at all, or whether this `depends_on` change only applies to `docker-compose.prod.yml`.

**Alternatives considered**: Leaving `depends_on` as plain service references (no condition) — rejected, defeats the purpose of adding healthchecks if nothing consumes them for ordering.

## Open items carried into implementation (not spec ambiguity, just verify-while-coding)

- Confirm `docker-compose.yml` (dev) has no `api`/`ollama` service block today (verified: it doesn't — dev compose only has postgres/redis/ollama/flaresolverr/minio/createbuckets; there IS an `ollama` entry in dev compose to heathcheck, but no `api` entry, so the api healthcheck/depends_on changes apply to `docker-compose.prod.yml` only, and the ollama healthcheck applies to both files).
- Confirm `docker-compose.prod.yml` has no `ollama` service block today (verified: it doesn't) — spec FR-007 requires an ollama healthcheck; since prod compose doesn't run ollama at all currently, that requirement applies to `docker-compose.yml` (dev) only, unless ollama is added to prod compose as part of this feature (out of scope — not requested, would be scope creep beyond "add healthchecks").
