---
title: Local development
sidebar_position: 1
description: Prerequisites, environment setup, every Makefile target, ports, and per-worktree isolation.
---

# Local development

## Prerequisites

| Tool | Why |
| --- | --- |
| Go (version from `apps/api/go.mod`) | backend |
| Node ≥ 20 (CI uses 22) | dashboard |
| pnpm 11 | workspace package manager |
| Docker + Compose | Postgres, Redis, MinIO, Ollama, asynqmon |
| `sqlc` at the pinned version | `make sqlc-install` |
| `tygo` at the pinned version | `make tygo-install` |
| `rendercv` | PDF resume rendering (`RENDERCV_BIN`) |

## First run

```bash
cp .env.example .env
# set DB_PASSWORD and CONFIG_ENCRYPTION_KEY (openssl rand -hex 32)

make up                                        # postgres, redis, asynqmon, ollama, minio
pnpm install
pnpm --filter @job-finder/shared build
make run-backend                               # :3000 — migrates on startup
make run-frontend                              # :5173
```

`db.Migrate` runs inside `main.run`, so there is no separate migration step.

:::warning Long-lived processes
`make run-backend`, `make run-frontend` and `make run-all` never return. Start them under a
process supervisor rather than in a blocking shell call (`AGENTS.md`).
:::

## Topology

```mermaid
flowchart LR
    DEV["Vite :5173"] -->|"proxy /api"| API["Go API :3000"]
    API --> PG[("postgres :5432+ — pgvector/pgvector:pg16")]
    API --> RD[("redis :6379")]
    API --> OL["ollama :11434"]
    API --> MIN[("minio :9000, console :9001")]
    AM["asynqmon :8090"] --> RD
    FS["flaresolverr :8191 — profile scraping-extras"] -.-> API
```

## Ports

| Service | Port | Notes |
| --- | --- | --- |
| Dashboard (dev) | 5173 | Vite, proxies `/api` to 3000 |
| API | 3000 | `PORT` |
| Postgres | `POSTGRES_HOST_PORT` | derived per worktree, base 5432 |
| Redis | 6379 | test suite uses DB index 1 |
| asynqmon | 8090 | dev only — deliberately not 8080 |
| MinIO | 9000 / 9001 | API / console |
| Ollama | 11434 | local inference |
| FlareSolverr | 8191 | only with `--profile scraping-extras` |

## Per-worktree isolation

```makefile
WORKTREE_NAME := $(shell basename "$$(git rev-parse --show-toplevel 2>/dev/null || pwd)")
WORKTREE_HASH := $(shell echo "$(WORKTREE_NAME)" | cksum | cut -d' ' -f1)
export COMPOSE_PROJECT_NAME := jobfinder-$(WORKTREE_NAME)
export POSTGRES_HOST_PORT   := $(shell echo "$$(( 5432 + ( $(WORKTREE_HASH) % 100 ) ))")
```

Each worktree gets its own compose project and Postgres host port, *"so migration state
from one branch never leaks into another's test run."* Two branches can run
simultaneously.

## Makefile targets

### Infrastructure

| Target | Effect |
| --- | --- |
| `make up` | `docker compose up -d` |
| `make down` | stop the stack |
| `make logs` | follow compose logs |
| `make ps` | container status |
| `make clean` | `down -v` plus remove `node_modules` and `dist` |

### Running

| Target | Effect |
| --- | --- |
| `make run-backend` | `go run ./cmd/server` |
| `make run-frontend` | `pnpm dev` |
| `make run-all` | `up`, then backend and frontend |

### Tests

| Target | Effect |
| --- | --- |
| `make test` | `test-go` + `test-react` |
| `make test-go` | Go tests against `jobfinder_test` and Redis DB 1 |
| `make test-react` | `npx vitest run` |
| `make test-integration` | `test-db-setup`, wait for Postgres, `go test -tags integration` |
| `make test-e2e` | compose up, then Playwright |
| `make test-lint` | `test-go` + `test-react` |
| `make test-db-setup` | drop and recreate `jobfinder_test` |

### Code generation

| Target | Effect |
| --- | --- |
| `make sqlc-install` / `make sqlc-generate` / `make sqlc-check` | sqlc at the pin in `apps/api/.sqlc-version` |
| `make tygo-install` / `make tygo-generate` / `make tygo-check` | tygo at the pin in `apps/api/.tygo-version` |

### Data

| Target | Effect |
| --- | --- |
| `make seed` / `make seed-clean` | `go run ./cmd/seed` |
| `make truncate-db` | truncate the ten core tables with `RESTART IDENTITY CASCADE` |

### Production stack

| Target | Effect |
| --- | --- |
| `make prod-build` / `make prod-up` / `make prod-down` | `docker-compose.prod.yml` |

## Compose services

| Service | Image | Notes |
| --- | --- | --- |
| `postgres` | `pgvector/pgvector:pg16` | healthcheck `pg_isready`; `DB_PASSWORD` is required — compose fails loudly without it |
| `redis` | `redis:7-alpine` | |
| `asynqmon` | `hibiken/asynqmon:0.7.2` | dev only; distroless, so no healthcheck |
| `ollama` | `ollama/ollama:latest` | uncomment the `deploy` block for GPU |
| `flaresolverr` | ghcr image | `profiles: [scraping-extras]` — opt in |
| `minio` | `minio/minio:latest` | healthcheck probes via bash `/dev/tcp`, since the image ships no curl or mc |
| `createbuckets` | `minio/mc:latest` | one-shot; creates the documents bucket, then exits |

Two nice touches to copy: `${DB_PASSWORD:?set DB_PASSWORD in .env}` turns a missing
variable into a clear error, and `createbuckets` removes a manual setup step.

## Bring-up sequence

```mermaid
sequenceDiagram
    participant D as Developer
    participant C as docker compose
    participant PG as postgres
    participant M as minio
    participant CB as createbuckets
    participant API as go run ./cmd/server
    participant V as vite
    D->>C: make up
    C->>PG: start, healthcheck pg_isready
    C->>M: start, healthcheck /minio/health/live
    M->>CB: reachable
    CB->>M: mc mb documents then exit
    D->>API: make run-backend
    API->>PG: goose migrate to head
    API->>API: buildPlatform, buildContexts, buildServers
    API-->>D: API listening on 3000
    D->>V: make run-frontend
    V-->>D: dashboard on 5173
```

## Common tasks

| Task | Commands |
| --- | --- |
| Reset the database | `make clean && make up && make run-backend` |
| Clear job data only | `make truncate-db` |
| Add a query | edit `internal/db/queries/*.sql`, `make sqlc-generate` |
| Add a DTO field | edit `internal/dto`, `make tygo-generate`, mirror in `packages/shared/src/index.ts`, rebuild shared |
| Try a source | `POST /api/sources/{key}/test` then `/run` |
| Watch the queues | asynqmon at http://localhost:8090 |
| Use Ollama Cloud | `OLLAMA_URL=https://ollama.com`, `OLLAMA_KEY=…`, and point `EMBED_URL` at a local Ollama |

:::note Embeddings need a local Ollama
Ollama Cloud serves no embedding models. With `OLLAMA_URL` pointed at the cloud, set
`EMBED_URL=http://localhost:11434`.
:::
