> **ARCHIVED — SHIPPED.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/platform-operations.md`](../../domains/platform-operations.md) — read that first.

---
# Feature Specification: Asynqmon Queue Monitoring Dashboard

**Feature Branch**: `018-asynqmon-monitoring`

**Created**: 2026-07-28

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "add https://github.com/hibiken/asynqmon to project. i need to monitor all the data"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - View live queue state (Priority: P1)

As the operator running job-finder locally, I want a dashboard where I can see all background job queues (ingest, match, generate, enrich, salary, ghost) at a glance, so I know what work is pending, running, or stuck without querying Redis by hand.

**Why this priority**: Without visibility, a stalled or backed-up queue (e.g. ingestion silently failing) goes unnoticed until downstream data looks wrong. This is the core value of the feature.

**Independent Test**: Start the stack, trigger a background task (e.g. an ingestion run), open the dashboard, and confirm the task and its queue counts (pending/active/scheduled) appear and update.

**Acceptance Scenarios**:

1. **Given** the stack is running with tasks flowing through any of the six queues, **When** I open the monitoring dashboard, **Then** I see each queue listed with counts of pending, active, scheduled, retry, archived, and completed tasks.
2. **Given** a task is currently being processed, **When** I view that queue's task list, **Then** I see the task's type, payload summary, and current state update without a full page reload being required to notice new activity.

---

### User Story 2 - Inspect and act on failed/stuck tasks (Priority: P2)

As the operator, I want to inspect a failed or archived task's error details and payload, and retry or delete it, so I can recover from transient failures without redeploying or writing scripts.

**Why this priority**: Monitoring alone tells you something broke; the ability to act on it (retry, delete, requeue) is what turns visibility into an operational tool.

**Independent Test**: Force a task to fail (e.g. stop a dependency it needs), confirm it appears in the retry/archived list with its error message, then retry it from the dashboard and confirm it re-enters the pending state.

**Acceptance Scenarios**:

1. **Given** a task has failed and is archived, **When** I open it in the dashboard, **Then** I see its last error message, retry count, and payload.
2. **Given** an archived task, **When** I choose to retry it, **Then** it moves back into the pending/scheduled state for its queue.
3. **Given** a task I no longer want processed, **When** I choose to delete it, **Then** it is removed and no longer counted in any queue total.

---

### User Story 3 - Monitor historical throughput (Priority: P3)

As the operator, I want to see recent daily processed/failed counts per queue, so I can spot trends (e.g. a queue's failure rate creeping up) rather than only the current snapshot.

**Why this priority**: Nice-to-have trend visibility; the point-in-time views in P1/P2 already cover the operational need.

**Independent Test**: Let the stack run for a day with a mix of successful and failed tasks, then confirm the dashboard's history view shows a daily breakdown matching what actually ran.

**Acceptance Scenarios**:

1. **Given** tasks have processed over multiple days, **When** I view a queue's history, **Then** I see per-day processed and failed counts for a recent window.

---

### Edge Cases

- What happens when the queue backend (Redis) is unreachable? Dashboard MUST show a clear connection error rather than a blank or broken page.
- What happens when a queue has zero tasks (idle)? Dashboard MUST show it as empty/healthy, not as an error.
- What happens when a task payload is large or binary? Dashboard MUST still render the task entry without crashing, truncating or summarizing the payload if needed.
- What happens if someone tries to reach the dashboard from outside the local/dev network? Access MUST be restricted per the access-control assumption below.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide a web dashboard that connects to the project's existing Redis-backed task queue backend and lists all six queues in use (ingest, match, generate, enrich, salary, ghost).
- **FR-002**: Dashboard MUST show, per queue, live counts of tasks in each state: pending, active, scheduled, retry, archived, completed.
- **FR-003**: Dashboard MUST allow drilling into a queue to list its individual tasks with type, payload, state, and (for failed tasks) error details.
- **FR-004**: Dashboard MUST allow retrying, archiving, and deleting individual tasks or performing these actions in bulk on a queue.
- **FR-005**: Dashboard MUST show recent historical daily processed/failed counts per queue.
- **FR-006**: Dashboard MUST be reachable via a dedicated URL/port distinct from the main application API.
- **FR-007**: Dashboard MUST run alongside the existing local development stack (started the same way as other services) so it requires no separate manual setup step beyond normal `make run-all` / `docker compose up` usage.
- **FR-008**: Access to the dashboard MUST be restricted to the local/development network only; it MUST NOT be exposed on the production deployment.

### Key Entities

- **Queue**: One of the six named task queues (ingest, match, generate, enrich, salary, ghost); has aggregate counts by task state.
- **Task**: A unit of background work within a queue; has a type, payload, current state, retry count, and (if failed) last error message.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Operator can determine the pending/active/failed count for any of the six queues within 5 seconds of opening the dashboard, without running any command-line tool.
- **SC-002**: Operator can locate the error reason for any failed task and retry it in under 30 seconds, without writing or running a script.
- **SC-003**: Dashboard reflects a newly enqueued or state-changed task within 5 seconds of that change occurring.
- **SC-004**: Dashboard adds no more than one additional local service to start and requires zero manual configuration beyond what's already needed to run the stack.

## Assumptions

- "Monitor all the data" means all background task/queue data currently flowing through the project's task queue backend (the six existing queues: ingest, match, generate, enrich, salary, ghost) — not application/business data such as jobs or applications, which have their own UI.
- The dashboard is for local/development operational use, run as part of the existing docker-compose stack; it is not exposed on the production deployment (no auth system currently fronts internal dev tooling like this, so restricting network exposure is the chosen safeguard rather than adding new auth).
- The dashboard is read/write for task management (retry, delete, archive) since that operational control is the main reason to add it over a read-only view.
- No additional queue backends beyond the existing Redis instance need to be monitored.
