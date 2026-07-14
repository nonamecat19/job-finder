# Spec: "Status" tab — under-the-hood activity visibility

## Context

Job-finder does a lot of work asynchronously that is **invisible to the user**: AI
tailoring a resume, writing a cover letter, scraping a vacancy list, scraping a
single vacancy's detail, computing a match. Today the only feedback is a finished
row appearing in the DB (a `SourceRun` says "found 12", a `GeneratedDocument`
row shows up). There is **no way to see what is happening right now**, which step
a task is on, or why one failed. The user does not understand what runs under the
hood.

This spec adds a new dashboard tab **"Status"** that shows live and recently-finished
activity across all four async operations, with a human-readable current step per
run (e.g. `grounding check (attempt 2/2)`, `rendering PDF`, `scraping page 3`).

**Deliverable of the planning task:** write this spec as a markdown file at
`plan/status-tab.md` in the repo (the repo `plan/` dir is currently empty).

### Decisions (confirmed with user)
- **Depth:** full activity log + per-step progress (new table + handler instrumentation).
- **Transport:** polling via TanStack Query `refetchInterval` (existing pattern; no SSE/WS today).
- **Scope:** live in-flight tasks **and** recent completed/failed history.

## Current architecture (facts)

- One Go binary `cmd/server/main.go`: chi HTTP API + **4 asynq (Redis) worker
  servers** — queues `ingest`/`match`/`generate`/`enrich` (`internal/queue/queue.go:14-35`),
  concurrency 2/1/1/1 (`main.go:144-172`) — + in-process cron scheduler.
- **No SSE, no WebSocket, no event bus, no asynq Inspector usage.** All realtime
  feedback is client polling of terminal-state rows.
- Existing progress signals: `SourceRun` (ingest), `Job.detailScrapedAt` (enrich),
  `MatchResult` presence (match), `GeneratedDocument` + `Application.events[]` (generate).
- Handlers: `ingestion/handler.go`, `matching/handler.go`, `generation/handler.go`,
  `enrichment/handler.go`; each `ProcessTask` unmarshals a payload from
  `internal/queue/queue.go` and calls its service.
- Enqueue points: `ingestion/service.go` `RunSearch`/`RunSource`/`RunSubscription`,
  `jobs/service.go` `EnqueueGeneration`, `ingestion/handler.go` `enqueueMatch`/`enqueueEnrich`,
  `enrichment/handler.go` `enqueueMatch`.
- Routes mounted per handler via `Mount(chi.Router)`, wired in `cmd/server/main.go:125-127`,
  router in `internal/httpapi/router.go:81`. JSON helpers `writeJSON`/`writeError` there.
- Frontend: React 19 + Vite + TanStack Query. API client is `apps/dashboard/src/lib/api.ts`
  (`src/api.ts` just re-exports it). DTO types imported from `@job-finder/shared`.
  Nav in `src/app/shell.tsx:6-11` (`navItems`), routes in `src/app/routes.tsx:8-18`.
  Polling precedent: `features/job-detail/hooks.ts:14-19` (`refetchInterval: 3000`).

## Design

A single generic **`ActivityRun`** table records one row per enqueued async task,
updated in place as the task progresses. Rows are created at **enqueue** time
(state `queued`) so tasks appear before a worker picks them up. A tiny
`internal/activity` recorder threads a run id from enqueue → handler → service so
deep functions (e.g. the grounding retry loop) can post step labels without wide
signature churn.

### 1. Schema — new migration `internal/db/migrations/00004_activity_run.sql`

```sql
-- +goose Up
CREATE TABLE "ActivityRun" (
  "id"         uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
  "op"         text NOT NULL,                    -- ingest|match|generate|enrich
  "state"      text NOT NULL DEFAULT 'queued',   -- queued|running|succeeded|failed
  "label"      text NOT NULL,                    -- human summary e.g. "Acme — Backend Engineer" / "djinni scrape"
  "step"       text,                             -- current step label, updated in place
  "jobId"      uuid,                             -- match/generate/enrich (nullable)
  "sourceKey"  text,                             -- ingest/enrich (nullable)
  "queueTaskId" text,                            -- asynq task id (for reference/debug)
  "refId"      text,                             -- produced artifact id (documentId / sourceRunId)
  "error"      text,
  "meta"       jsonb NOT NULL DEFAULT '{}'::jsonb, -- counts/attempt/version/page/score
  "createdAt"  timestamp (3) DEFAULT now() NOT NULL,
  "startedAt"  timestamp (3),
  "finishedAt" timestamp (3)
);
ALTER TABLE "ActivityRun" ADD CONSTRAINT "ActivityRun_jobId_Job_id_fk"
  FOREIGN KEY ("jobId") REFERENCES "public"."Job"("id") ON DELETE cascade;
CREATE INDEX "ActivityRun_createdAt_idx" ON "ActivityRun" ("createdAt");
CREATE INDEX "ActivityRun_active_idx" ON "ActivityRun" ("state") WHERE "state" IN ('queued','running');
-- +goose Down
DROP TABLE IF EXISTS "ActivityRun";
```

Mirror the migration in the Drizzle source if that path is still authoritative
(see `00001_init.sql` header note); if Drizzle is retired, migration only.

### 2. Queries — `internal/db/queries/activityrun.sql` (sqlc; run `sqlc generate`)

- `InsertActivityRun` (op, label, jobId, sourceKey, queueTaskId, meta) → RETURNING row (state `queued`).
- `StartActivityRun(id)` → state `running`, `startedAt = now()`.
- `SetActivityStep(id, step, meta)` → update `step` (+ merge/replace `meta`).
- `FinishActivityRunOk(id, refId, meta)` → state `succeeded`, `finishedAt = now()`.
- `FinishActivityRunError(id, error)` → state `failed`, `finishedAt = now()`.
- `ListActiveActivityRuns` → `state IN ('queued','running')` order `createdAt` desc.
- `ListRecentActivityRuns(limit)` → order `createdAt` desc, limit (e.g. 100).
- `DeleteActivityRunsBefore(ts)` → retention cleanup (optional; call from scheduler tick).

### 3. Recorder — new package `internal/activity`

```go
type Recorder struct { q *sqlcgen.Queries; id uuid.UUID }
func New(ctx, q, op, label string, jobID *uuid, sourceKey *string, taskID string) (*Recorder, error) // inserts queued row
func FromID(q, id) *Recorder
func (r *Recorder) Start(ctx)
func (r *Recorder) Step(ctx, label string, meta map[string]any)
func (r *Recorder) Ok(ctx, refID string, meta map[string]any)
func (r *Recorder) Fail(ctx, err error)
```

All methods best-effort: log-and-continue on DB error so activity tracking never
breaks the real task. `Step` is what deep service funcs call.

### 4. Wire the run id through the queue

Add `ActivityID *string` to each payload struct in `internal/queue/queue.go`
(`IngestPayload`, `MatchPayload`, `EnrichPayload`, `GeneratePayload`).
Enqueue sites create the `ActivityRun` row first, then put its id on the payload:

- `jobs/service.go` `EnqueueGeneration` — op `generate`, label `company — title`, jobId, meta `{type}`.
- `ingestion/service.go` `RunSearch`/`RunSource`/`RunSubscription` — op `ingest`, sourceKey, label `"<sourceKey> scrape"`.
- `ingestion/handler.go` `enqueueMatch`/`enqueueEnrich`; `enrichment/handler.go` `enqueueMatch`.

Each `ProcessTask` builds `activity.FromID(payload.ActivityID)`, calls `Start`, posts
`Step`s at key points, and `Ok`/`Fail` at the end.

### 5. Step instrumentation (the "under the hood" labels)

- **generate** (`generation/service.go` `Generate` + `tailorResume`/`writeCoverLetter`):
  `loading profile & match` → `tailoring resume (LLM)` / `grounding check (attempt N/2)`
  → `rendering PDF` → Ok with `refId=documentId`, meta `{version}`. Pass the recorder
  into `tailorResume`/`writeCoverLetter` so the grounding retry loop can report attempts.
- **ingest** (`ingestion/handler.go` `ProcessTask`): `scraping <source>` → optional
  `page N` if the adapter can report → `persisting N new` → Ok meta `{found, new}`
  (alongside existing `SourceRun`). Fail on adapter error, mirror `SourceRun` error.
- **enrich** (`enrichment/handler.go`): `fetching detail (<sourceKey>)` → Ok, or Fail.
- **match** (`matching/service.go` `MatchJob`): `embedding` → `prefilter (similarity)`
  → `LLM fit analysis` → Ok meta `{score, similarity}`; short-circuit path Ok at prefilter.

### 6. HTTP endpoint — `internal/httpapi/activity.go`

- `ActivityHandler` with `Mount(r)`: `r.Get("/activity", h.list)`.
- `GET /api/activity?limit=100` returns one payload for a single poll:
  `{ "active": ActivityRunDto[], "recent": ActivityRunDto[] }`
  (`active` = `ListActiveActivityRuns`, `recent` = `ListRecentActivityRuns`).
- `ActivityRunDto` (add to `@job-finder/shared` types): `id, op, state, label, step,
  jobId, sourceKey, refId, error, meta, createdAt, startedAt, finishedAt, elapsedMs`.
- Register `activityHandler.Mount` in `cmd/server/main.go:125-127` mounts list.

### 7. Frontend — new feature `apps/dashboard/src/features/status/`

- `apps/dashboard/src/lib/api.ts`: add group
  `activity: { list: (limit?) => request<{active: ActivityRunDto[]; recent: ActivityRunDto[]}>(\`/activity${limit?`?limit=${limit}`:''}\`) }`.
- `src/lib/queryKeys.ts`: add `activity` key.
- `features/status/hooks.ts`: `useActivity()` → `useQuery({ queryKey: activity, queryFn: api.activity.list, refetchInterval: 2000 })`.
- `features/status/StatusPage.tsx` (default export, mirror `features/tracker/TrackerPage.tsx`):
  - **Active** section: card per run — op badge (color per op), `label`, current `step`,
    `Spinner`, live elapsed since `startedAt`. `EmptyState` "Nothing running" when empty.
  - **Recent** section: table — op, label, state (✓ succeeded / ✗ failed), duration
    (`finishedAt-startedAt`), `error` on failure. Row links to `/jobs/{jobId}` when present.
  - Reuse `PageHeader`/`SectionTitle` (`src/components/layout/PageHeader.tsx`),
    primitives from `src/components/ui.tsx`, `Spinner`/`ErrorState`/`EmptyState`.
- `src/app/routes.tsx`: add `<Route path="/status" element={<StatusPage />} />` + import.
- `src/app/shell.tsx:6-11`: add `{ to: '/status', label: 'Status', icon: <Activity /> }`
  (lucide `Activity`).

## Files to create / modify

**Backend (`apps/api`)**
- new `internal/db/migrations/00004_activity_run.sql`
- new `internal/db/queries/activityrun.sql` → `sqlc generate` → `internal/db/sqlcgen/activityrun.sql.go`
- new `internal/activity/recorder.go`
- new `internal/httpapi/activity.go` (+ test mirroring `searches_test.go`)
- edit `internal/queue/queue.go` (payload `ActivityID` fields)
- edit `internal/{ingestion,matching,generation,enrichment}/handler.go` + relevant `service.go`
- edit `internal/jobs/service.go`, `internal/ingestion/service.go` (enqueue-time row creation)
- edit `cmd/server/main.go` (construct recorder deps + mount handler)

**Frontend (`apps/dashboard`)**
- new `src/features/status/StatusPage.tsx`, `src/features/status/hooks.ts`
- edit `src/lib/api.ts`, `src/lib/queryKeys.ts`, `src/app/routes.tsx`, `src/app/shell.tsx`
- add `ActivityRunDto` to `@job-finder/shared` types

## Verification

1. **Backend unit:** `cd apps/api && go build ./... && go test ./...`. Add
   `internal/httpapi/activity_test.go` asserting `GET /api/activity` shape.
2. **End-to-end (Redis + Postgres up, `goose up`, run server):**
   - `POST /api/jobs/{id}/generate` then `curl /api/activity` repeatedly → row goes
     `queued → running` with `step` cycling `tailoring resume (LLM)` → `grounding check…`
     → `rendering PDF` → `succeeded` with `refId`.
   - `POST /api/searches/{id}/run` → an `ingest` row appears, then spawned `match`
     rows per new job; confirm a failing source shows `state=failed` + `error`.
3. **Dashboard:** `pnpm dev`, open `/status`; trigger a generate/scrape from other
   tabs; watch the Active section populate live (2s poll), elapsed tick, then move to
   Recent with ✓/✗ and duration.

## Notes / non-goals
- No SSE/WebSocket — polling matches the existing codebase; can upgrade later.
- Activity tracking is best-effort: a recorder DB failure must never fail the task.
- Retention: add `DeleteActivityRunsBefore` swept from the ingestion scheduler tick
  (e.g. keep 7 days) to bound table growth.
