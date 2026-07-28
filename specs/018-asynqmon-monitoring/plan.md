# Implementation Plan: Asynqmon Queue Monitoring Dashboard

**Branch**: `018-asynqmon-monitoring` | **Date**: 2026-07-28 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/018-asynqmon-monitoring/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command; its definition describes the execution workflow.

## Summary

Add the `asynqmon` web dashboard as a new Docker Compose service in the local/dev stack, pointed at the project's existing Redis instance, to give the operator live visibility into and control over the six asynq task queues (ingest, match, generate, enrich, salary, ghost). No application code changes — this is a compose-config-only addition, kept out of `docker-compose.prod.yml` per the dev-only access-control requirement (FR-008).

## Technical Context

**Language/Version**: N/A — no new application code; asynqmon ships as a prebuilt Docker image (`hibiken/asynqmon`)

**Primary Dependencies**: `hibiken/asynqmon` Docker image; existing `redis:7-alpine` service (already in `docker-compose.yml`)

**Storage**: Redis (existing service `redis`, same instance the six asynq workers already use — see `apps/api/cmd/server/servers.go`)

**Testing**: Manual verification via `quickstart.md` (compose up, enqueue a task, confirm it's visible/actionable in the dashboard); `docker compose config` validates the compose file syntactically. No new Go/vitest suite applies since no app code changes.

**Target Platform**: Docker Compose, local/dev only (`docker-compose.yml`); explicitly excluded from `docker-compose.prod.yml` (FR-008)

**Project Type**: Infra/ops addition (single new service in existing docker-compose stack)

**Performance Goals**: Dashboard reflects task/queue state changes within 5s (SC-003) — met by asynqmon's default polling, no tuning needed at this scale

**Constraints**: Zero new manual setup step (FR-007) — service starts with `make run-all` / `docker compose up` like every other service; dev-network-only exposure, no new auth system added (per spec Assumptions)

**Scale/Scope**: One Redis instance, six existing queues; single new compose service, no new source directories

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever**: N/A — this feature only monitors/manages background job-processing tasks (ingest/match/generate/enrich/salary/ghost queues), not job applications or employer-facing actions. No gate impact.
- **II. Grounded Generation**: N/A — no LLM-generated content involved.
- **III. Typed Contracts Across Service Boundaries**: N/A — no new cross-language boundary; asynqmon is a standalone prebuilt tool talking directly to Redis, not integrated into the Go API or TS dashboard.
- **IV. Test Discipline Per Language, Enforced at the Boundary**: Satisfied — no app-language code is added, so no new `go test`/`vitest` suite is required. Verification is via `quickstart.md` manual/compose validation, consistent with this being an infra-only change (not a cross-app code change), so `make test-lint` is not triggered.
- **V. Local-First, Self-Hosted by Default**: Satisfied — asynqmon is a self-hosted OSS tool, connects only to the existing self-hosted Redis, and adds no third-party API dependency.

Gate result: **PASS**, no violations, no Complexity Tracking entries needed.

**`contracts/` skipped**: asynqmon exposes its own bundled UI/API, not an interface this
project defines or owns; job-finder's API/dashboard expose no new interface for this
feature. Nothing to contract here.

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
docker-compose.yml          # add `asynqmon` service (new, dev-only)
docker-compose.prod.yml     # unchanged — asynqmon intentionally NOT added here (FR-008)
Makefile                    # unchanged — `make up` / `make run-all` already bring up all
                             # docker-compose.yml services, so the new service starts for free
```

**Structure Decision**: No new application source tree. This feature is a single
addition to the existing root-level `docker-compose.yml` (a new `asynqmon` service
alongside `redis`, `postgres`, `ollama`, `minio`), wired to the existing `redis`
service via `--redis-addr`/`REDIS_ADDR`. `docker-compose.prod.yml` is deliberately
left unchanged.

## Complexity Tracking

*No violations — section not applicable.*
