# Implementation Plan: Health/Readiness Endpoints + Compose Healthchecks

**Branch**: `008-health-readiness-checks` | **Date**: 2026-07-24 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/008-health-readiness-checks/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command; its definition describes the execution workflow.

## Summary

Add a `/api/health/ready` readiness endpoint to `apps/api` that pings Postgres, Redis, and MinIO (each bounded by a timeout) and returns per-dependency status, keeping the existing `/api/health` as pure liveness. Wire `healthcheck:` blocks into `docker-compose.yml` and `docker-compose.prod.yml` for `api` (curl against the new readiness endpoint), `ollama` (`ollama list`, the standard workaround since the image ships no curl/wget), and `minio` (a bash `/dev/tcp` HTTP probe against `/minio/health/live`, since the official image also ships no curl/wget/mc). Add `depends_on: condition: service_healthy` from `api` onto `postgres` and `minio` where compose currently only has plain `depends_on`/no ordering. `apps/jobspy-sidecar` is excluded — it is being deleted from the repo in this working tree (see spec Assumptions).

## Technical Context

**Language/Version**: Go 1.26 (apps/api); Docker Compose v2 YAML (both compose files)

**Primary Dependencies**: chi router (existing `internal/httpapi`), `pgxpool` (existing `internal/db`), `github.com/redis/go-redis/v9` (already an indirect dependency via asynq — promoted to direct use here), `github.com/minio/minio-go/v7` (existing `internal/storage`, reused for a lightweight `BucketExists`-style ping)

**Storage**: N/A (no schema change — this feature only adds read-only health probes against existing Postgres/Redis/MinIO connections)

**Testing**: `go test ./internal/httpapi/...` for the new handler (httptest, mocking dependency pings); manual `docker compose up` + `docker compose ps` verification for the compose healthchecks (compose YAML has no native unit-test story in this repo)

**Target Platform**: Linux containers via Docker Compose (dev: `docker-compose.yml`, prod: `docker-compose.prod.yml`)

**Project Type**: Web service (Go API) + infra config (compose) — no dashboard/frontend change

**Performance Goals**: Readiness endpoint responds within ~2s under normal conditions (each dependency check individually timeout-bounded per FR-004); not a high-throughput path (polled every 5s by compose, not user traffic)

**Constraints**: Must not introduce a hard dependency on tools absent from upstream images — `ollama/ollama` and `minio/minio` both ship no `curl`/`wget` (verified: both provide `bash` with `/dev/tcp` support; ollama ships its own `ollama` CLI which talks to its local API)

**Scale/Scope**: 4 services touched (api code + 3 services' compose entries: api, ollama, minio); single readiness endpoint, no new persistent state

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — N/A. Health/readiness endpoints are passive infra signals; no code path submits applications or acts on job listings.
- **II. Grounded Generation** — N/A. No LLM-generated content involved.
- **III. Typed Contracts Across Service Boundaries** — Readiness endpoint is a new internal HTTP surface consumed by compose's own healthcheck runner, not by the dashboard or `packages/shared`; no cross-language DTO needed. PASS (no violation — nothing to type-share here).
- **IV. Test Discipline Per Language, Enforced at the Boundary** — New Go handler gets `go test` coverage in `apps/api`'s native suite; readiness behavior against real Postgres/Redis/MinIO is exercised by the existing Docker-Compose-backed integration test setup if such a test is added (optional; see tasks). PASS.
- **V. Local-First, Self-Hosted by Default** — All three dependencies checked (Postgres, Redis, MinIO) are self-hosted services already in compose; no external API calls introduced. PASS.

No violations requiring Complexity Tracking justification.

## Project Structure

### Documentation (this feature)

```text
specs/008-health-readiness-checks/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
apps/api/
├── internal/httpapi/
│   ├── router.go            # existing: liveness /health stays as-is
│   ├── health.go            # NEW: HealthHandler with Mount(chi.Router), GET /health/ready
│   └── health_test.go       # NEW: httptest coverage for ready/not-ready paths
├── cmd/server/
│   ├── platform.go          # extend Platform (or a small sub-struct) with what readiness needs:
│   │                         #   DB.Pool (already present), a redis.UniversalClient, MinioEndpoint config
│   ├── compose.go           # wire httpapi.HealthHandler into App/composeApp
│   └── servers.go           # add app.Health.Mount to the NewRouter(...) mount list

docker-compose.yml           # add healthcheck: to api, ollama, minio; depends_on condition: service_healthy on api
docker-compose.prod.yml      # same changes, mirrored (api/ollama/minio blocks; ollama isn't in prod compose today — verify during implementation whether prod runs ollama in-cluster or externally)
```

**Structure Decision**: Single Go backend project (`apps/api`) plus two Docker Compose files at repo root — no frontend/mobile split applies. All new Go code lives in `apps/api/internal/httpapi` (handler) and `apps/api/cmd/server` (composition wiring), following the existing `*Handler` + `Mount(chi.Router)` + `compose*` pattern used by every other feature in this codebase (e.g. `GhostJobHandler`/`composeGhostJob`).

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations — table intentionally omitted.
