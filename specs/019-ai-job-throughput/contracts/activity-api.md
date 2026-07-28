# Contract: Activity API changes

**Feature**: 019-ai-job-throughput

Two changes to the existing activity surface (`apps/api/internal/httpapi/activity.go`,
mounted under the API router): a widened state enum on existing responses, and one new
read-only endpoint.

---

## 1. `ActivityState` enum widened

Source of truth: `packages/shared/src/index.ts` (Constitution III).

```ts
export const ACTIVITY_STATES = [
  'queued', 'running', 'succeeded', 'failed', 'cancelled',
  'timed_out',    // NEW: exceeded its task-type deadline
  'interrupted',  // NEW: worker vanished (crash / shutdown / power loss)
] as const;
```

Response shape of `ActivityRunDto` is otherwise unchanged, plus two optional fields:

```ts
export interface ActivityRunDto {
  // ...existing fields unchanged...
  heartbeatAt: string | null;  // NEW
  timeoutMs: number | null;    // NEW
}
```

**Compatibility**: additive. Existing clients that switch on state must gain two branches;
`StatusPage.tsx` renders `timed_out` as a danger variant and `interrupted` as a warning
variant, both surfacing `error` as the reason (FR-011, SC-007).

**Retry**: `POST /activity/retry` accepts runs in `timed_out` and `interrupted` exactly as
it accepts `failed`/`cancelled` today — `ListFailedActivityRuns` widens its `state IN (...)`
list to all four (FR-012).

---

## 2. `GET /api/activity/queues` (new)

Read-only backlog snapshot per AI task type (FR-016, SC user story 4).

**Request**: no parameters.

**Response** `200 application/json`:

```json
{
  "queues": [
    {
      "queue": "match",
      "providerClass": "hosted",
      "concurrency": 3,
      "pending": 684,
      "active": 3,
      "scheduled": 0,
      "retry": 2,
      "archived": 11,
      "processedPerMinute": 18.4,
      "etaSeconds": 2230
    },
    {
      "queue": "ingest",
      "providerClass": null,
      "concurrency": 2,
      "pending": 0, "active": 0, "scheduled": 4, "retry": 0, "archived": 0,
      "processedPerMinute": 0,
      "etaSeconds": null
    }
  ]
}
```

Field rules:

- One entry per queue in `queue.Queue*`, in the fixed order ingest, match, generate,
  enrich, salary, ghost.
- `providerClass` is `null` for queues with no LLM task key (`ingest`, `enrich`).
- `concurrency` is the effective admission capacity for the *currently resolved* provider
  class, not the pool size.
- `etaSeconds` is `null` when `processedPerMinute` is 0 or `pending` is 0.
- Inspector failure for one queue must not fail the whole response: that queue's counters
  are omitted and an `"error"` string is set on the entry.

**Errors**: `503` only if the Inspector is entirely unavailable (Redis down), matching the
existing health-check convention.

**TS type**: `QueueBacklogDto` added to `packages/shared/src/index.ts`.
