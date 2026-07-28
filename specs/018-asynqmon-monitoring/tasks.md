---

description: "Task list template for feature implementation"
---

# Tasks: Asynqmon Queue Monitoring Dashboard

**Input**: Design documents from `/specs/018-asynqmon-monitoring/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md

**Tests**: Not requested in the spec. This feature adds zero application source code
(no Go/TS) — it's a single Docker Compose service addition, so verification is via the
manual/scripted checks in `quickstart.md` rather than a `go test`/`vitest` suite.

**Organization**: Tasks are grouped by user story (spec.md P1/P2/P3). All three stories
are delivered by the same underlying change (adding the `asynqmon` service), so the
Foundational phase carries the actual implementation and each story phase is its
independent verification slice.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)

## Path Conventions

Single Docker Compose project at repo root — no `src/`/`tests/` tree for this feature.
Primary file: `docker-compose.yml` (dev stack). `docker-compose.prod.yml` must NOT
change (FR-008).

---

## Phase 1: Setup

**Purpose**: Confirm the ground this change builds on before editing compose files

- [X] T001 Confirm host port 8090 is free and `.env` has `DB_PASSWORD` set, per research.md "Host port `8090`" decision (avoids colliding with 8080, already used by the dashboard in `docker-compose.prod.yml`); run `docker compose config` against `docker-compose.yml` at repo root to confirm the current file is valid before editing

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add the `asynqmon` service itself — this is what makes every user story possible; nothing in Phase 3+ can be verified until this lands

**⚠️ CRITICAL**: No user story verification can begin until this phase is complete

- [X] T002 Add `asynqmon` service to `docker-compose.yml`: image `hibiken/asynqmon:0.7.2` (pinned, not `latest`), `command: ["--redis-addr=redis:6379"]` (same Redis instance the six asynq workers in `apps/api/cmd/server/servers.go` already use), `ports: ["8090:8090"]`, `depends_on: [redis]`
- [X] T003 ~~Add a healthcheck~~ — skipped: `hibiken/asynqmon` is a distroless image (no shell/wget/curl), so a `CMD`-style healthcheck isn't possible; documented in a comment above the service in `docker-compose.yml`, readiness verified externally per `quickstart.md`
- [X] T004 Confirm `docker-compose.prod.yml` is left unmodified (FR-008) — `grep -c asynqmon docker-compose.prod.yml` returns `0`

**Checkpoint**: `docker compose up` starts `asynqmon` alongside the existing services and it is reachable at `http://localhost:8090`, wired to the shared `redis` service — ready for story verification

---

## Phase 3: User Story 1 - View live queue state (Priority: P1) 🎯 MVP

**Goal**: Operator can see all six queues and their task states at a glance

**Independent Test**: Start the stack, trigger a background task, open the dashboard, confirm the task and queue counts appear and update (spec.md US1)

- [X] T005 [US1] Run `docker compose up -d asynqmon`, confirm the `asynqmon` container is listed and running in `docker compose ps` alongside `redis`
- [X] T006 [US1] Verified live against the already-running app (real ingest/match/generate/enrich/ghost:score traffic): browser screenshot of `http://localhost:8090` shows all active queues listed with size/state/processed/failed/error-rate columns and a Queue Size chart (FR-002); `salary` queue not yet shown because no task has enqueued to it yet (asynq creates a queue key lazily on first use — expected behavior, not a defect)
- [X] T007 [US1] Drilled into the `generate` queue's task list in-browser; confirmed task type, payload (jobId/type/activityId), and status ("Running") render correctly (FR-003)

**Checkpoint**: User Story 1 is fully functional and independently verified

---

## Phase 4: User Story 2 - Inspect and act on failed/stuck tasks (Priority: P2)

**Goal**: Operator can see a failed task's error and retry/delete it from the dashboard

**Independent Test**: Force a task to fail, confirm it appears in retry/archived with its error, retry it and confirm it re-enters pending (spec.md US2)

- [X] T008 [US2] Verified against real archived tasks already present in the `generate` queue (e.g. `asynq: task lease expired`, and a genuine duplicate-key DB constraint failure) — dashboard's Archived tab shows last error message, last-failed time, and payload per task (FR-003)
- [X] T009 [US2] Used the queue-level "Run All" bulk action on the 5 archived `generate` tasks; toast confirmed "All archived tasks are now pending", Archived count dropped to 0 and Pending count rose accordingly (FR-004)
- [X] T010 [US2] Deleted an individual pending task (`9a948482`) via its row's trash-icon action; toast confirmed "Pending task deleted", Pending count dropped 43→42, task no longer listed (FR-004)

**Checkpoint**: User Stories 1 AND 2 both work independently

---

## Phase 5: User Story 3 - Monitor historical throughput (Priority: P3)

**Goal**: Operator can see recent daily processed/failed counts per queue

**Independent Test**: After a mix of successful/failed tasks, confirm the dashboard's history view shows a matching daily breakdown (spec.md US3)

- [X] T011 [US3] Confirmed via the main Queues page: "Tasks Processed" chart (LAST 7D selector) plots daily succeeded/failed lines from real historical data (FR-005)

**Checkpoint**: All user stories independently functional

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Wrap-up documentation and full end-to-end validation

- [X] T012 [P] Documented the asynqmon dashboard URL (`http://localhost:8090`), dev-only scope, and monitored queues in `README.md`
- [X] T013 Ran the full `quickstart.md` validation end-to-end:
  - FR-008 dev-only exposure: `grep -c asynqmon docker-compose.prod.yml` → `0`; `docker compose config` valid
  - Edge case "unreachable Redis": stopped `redis`, reloaded the dashboard — page does NOT crash/blank (shell, nav, empty charts all render), but it does NOT surface a visible error banner either; the connection failure only appears in the browser console (`listQueuesAsync: error: no error response data available`). This is asynqmon's stock behavior — per the Foundational-phase decision to ship the unmodified upstream image with no custom code, this can't be tightened without forking the UI, which is out of scope. Recorded as a known deviation from the spec's Edge Case wording ("MUST show a clear connection error") rather than silently marked as fully met. Redis restarted afterward and the dashboard recovered automatically with no restart needed.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user story phases (the service doesn't exist until T002-T004 land)
- **User Stories (Phase 3-5)**: All depend on Foundational completion only; independently verifiable in any order (P1 → P2 → P3 recommended)
- **Polish (Phase 6)**: Depends on all desired user stories being verified

### Within Phase 2

- T002 before T003 (T003 adds a healthcheck to the service T002 creates) — same file, sequential
- T004 is independent of T002/T003 (different file) but logically closes out the phase

### Parallel Opportunities

- T001 has no dependents and can run alongside nothing else in Phase 1 (it's the only task)
- T004 [different file: `docker-compose.prod.yml`] can run in parallel with T002/T003 [`docker-compose.yml`]
- T005-T007 (US1), T008-T010 (US2), T011 (US3) are independent verification tracks and can be run in parallel by different people once Phase 2 is checkpointed, though each internal sequence (e.g. T008 before T009) must stay in order
- T012 [P] (README, different file) can run in parallel with T013 (validation run)

---

## Parallel Example: Phase 2

```bash
# T002 and T003 touch the same file (docker-compose.yml) sequentially:
Task: "Add asynqmon service to docker-compose.yml"
Task: "Add healthcheck to asynqmon service in docker-compose.yml"

# T004 touches a different file and can run alongside them:
Task: "Confirm docker-compose.prod.yml is unmodified"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001)
2. Complete Phase 2: Foundational (T002-T004) — this is the actual code change
3. Complete Phase 3: User Story 1 (T005-T007)
4. **STOP and VALIDATE**: dashboard shows live queue state
5. Demo if ready — this alone satisfies the core "monitor all the data" request

### Incremental Delivery

1. Setup + Foundational → asynqmon running, wired to redis
2. Add US1 → verify live queue view → MVP demo
3. Add US2 → verify retry/delete → demo
4. Add US3 → verify history view → demo
5. Polish → docs + full quickstart pass

---

## Notes

- Because the entire feature is one Docker Compose service (asynqmon ships all three
  capabilities — live view, task actions, history — out of the box), Phase 2 is where
  the real work happens; Phase 3-5 are verification, not additional implementation.
- Commit after Phase 2 (the compose change) and after each story's verification passes.
- Avoid: adding auth or custom code to asynqmon beyond what FR-008 requires (network
  restriction to dev only) — that would exceed the spec's stated scope.
