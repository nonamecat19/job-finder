# Implementation Plan: AI Job Throughput & Stuck-Run Recovery

**Branch**: `019-ai-job-throughput` | **Date**: 2026-07-28 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/019-ai-job-throughput/spec.md`

## Summary

AI work is serialized at one item per task type because every worker is constructed with
`Concurrency: 1` (`apps/api/cmd/server/servers.go:70-77`), and stuck runs never resolve
because `ActivityRun` rows only leave `running` from a live handler — when the process dies
and the retry budget is spent, the row stays `running` forever.

Three changes, in dependency order:

1. **Per-provider-class admission gate.** Size each AI worker's asynq pool at
   `max(cloud, local)` and gate execution with two semaphores per task type, choosing the
   one that matches the provider class resolved from the live `llm.SnapshotHolder`. Hosted
   providers (Cerebras, OpenRouter, and Ollama Cloud — which is the same `OllamaProvider`
   pointed at `ollama.com`) get 3; local Ollama keeps 1.
2. **Deadline + heartbeat + sweeper.** Per-task-type `context.WithTimeout` in worker
   middleware terminates a run as `timed_out`; a `heartbeatAt` column plus a 60s sweeper
   closes rows whose worker vanished as `interrupted`. Both are new terminal states.
3. **Local matching cost cuts.** Cache the derived profile text per process, skip
   re-embedding when a job's content hash is unchanged, send `keep_alive` so a local model
   stays resident, and raise `MaxIdleConnsPerHost` on the LLM clients.

FR-003 ("no throttle on AI traffic") is already true — LLM clients never use the paced
`retrieval.DefaultTransport` — so it becomes a guard test rather than a change. See
[research.md](./research.md) R1.

## Technical Context

**Language/Version**: Go 1.x (`apps/api`), TypeScript 5 + React 18 (`apps/dashboard`,
`packages/shared`)

**Primary Dependencies**: asynq (Redis-backed queue), pgx v5 + sqlc, goose migrations,
pgvector, chi, viper; TanStack Query + Tailwind on the dashboard

**Storage**: PostgreSQL (`ActivityRun`, `Job`, `MatchResult`) + Redis (asynq queues)

**Testing**: `go test ./...`, `vitest`, Docker-backed `make test-integration`; `make test-lint`
required — this change spans `apps/api`, `packages/shared`, and `apps/dashboard`

**Target Platform**: Linux, Docker Compose (dev + prod); single API binary hosting the HTTP
server, six asynq workers, and the ingestion scheduler

**Project Type**: Web service + SPA dashboard (existing monorepo layout)

**Performance Goals**: ≥3 concurrent hosted AI items per task type; 700-item backlog in
≤40% of current wall-clock; median local match latency −30%; every non-terminal run
resolved within its deadline + 5 minutes

**Constraints**: provider routing flips at runtime with no restart, so concurrency must be
resolved per task rather than fixed at server construction; the local Ollama runtime is the
bottleneck on the local path and must not be over-driven; recovery must work under SIGKILL
and power loss, so it cannot depend on graceful shutdown

**Scale/Scope**: single-user self-hosted deployment; backlogs of ~1k queued AI items; six
queues; ~15 Go files touched plus shared types and one dashboard view

## Constitution Check

*GATE: evaluated before Phase 0 and re-evaluated after Phase 1 design. Both passes clean.*

| Principle | Assessment |
|---|---|
| **I. No Auto-Apply** | No new code path touches application submission. Untouched. **PASS** |
| **II. Grounded Generation** | Prompt content is unchanged. The profile-text cache (data-model §5) memoizes the *same* derived text from the same master profile and is version-invalidated on profile write, so traceability to source fields is preserved. The embedding-hash skip (§6) reuses an embedding only when the exact embedded text is byte-identical. **PASS** |
| **III. Typed Contracts** | Two new activity states and one new endpoint cross the Go↔TS boundary. `ACTIVITY_STATES` / `ActivityRunDto` / new `QueueBacklogDto` are updated in `packages/shared` as the single source of truth; the Go DTO and dashboard consume from there. No hand-duplicated types. **PASS** |
| **IV. Test Discipline** | New Go unit tests (policy validation, provider-class resolution, sweeper, deadline middleware, transport guard), integration tests against real Postgres/Redis for crash recovery and concurrent-match convergence, vitest for the widened state rendering. `make test-lint` is a release gate for this change. **PASS** |
| **V. Local-First, Self-Hosted** | The feature raises concurrency *only* for hosted providers and explicitly preserves local defaults (`AI_CONCURRENCY_LOCAL=1`). With no hosted credential configured every task resolves to local Ollama and behaves exactly as today (FR-018); the gate never makes a hosted provider mandatory. `keep_alive` benefits precisely the local path. **PASS** |

No violations — Complexity Tracking section omitted.

## Project Structure

### Documentation (this feature)

```text
specs/019-ai-job-throughput/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output — R1..R9, all code-anchored
├── data-model.md        # Phase 1 output — ActivityRun extensions, TaskPolicy, gate, caches
├── quickstart.md        # Phase 1 output — 8 validation scenarios + baseline capture
├── contracts/
│   ├── activity-api.md  # widened ActivityState, GET /api/activity/queues
│   └── config.md        # new env keys, defaults, validation rules
├── checklists/
│   └── requirements.md  # spec quality checklist (all pass)
└── tasks.md             # Phase 2 output — created by /speckit-tasks, NOT here
```

### Source Code (repository root)

```text
apps/api/
├── cmd/server/
│   ├── servers.go              # MODIFY: pool sizing from TaskPolicy, wrap handlers in middleware
│   ├── platform.go             # MODIFY: construct policies, gates, sweeper
│   └── compose_tasks.go        # MODIFY: pass policy/gate into worker construction
├── internal/
│   ├── queue/
│   │   ├── policy.go           # NEW: TaskPolicy, defaults, validation
│   │   └── middleware.go       # NEW: deadline + admission gate + heartbeat wrapper
│   ├── llm/
│   │   ├── router.go           # MODIFY: ProviderClass() on Router
│   │   ├── ollama.go           # MODIFY: keep_alive, IsHosted(), tuned transport
│   │   ├── cerebras.go         # MODIFY: tuned transport (MaxIdleConnsPerHost)
│   │   └── openrouter.go       # MODIFY: tuned transport
│   ├── activity/
│   │   ├── recorder.go         # MODIFY: heartbeat write, TimedOut finisher
│   │   └── sweeper.go          # NEW: periodic + startup stale-run sweep
│   ├── matching/
│   │   ├── service.go          # MODIFY: use profile snapshot cache
│   │   └── scoring.go          # MODIFY: embedding hash skip, cached profile text
│   ├── profile/
│   │   └── snapshot.go         # NEW: versioned profile-text cache
│   ├── httpapi/
│   │   └── activity.go         # MODIFY: GET /activity/queues, widened retry states
│   ├── config/
│   │   ├── config.go           # MODIFY: new fields
│   │   └── defaults.go         # MODIFY: new defaults / optional keys
│   └── db/
│       ├── migrations/00030_activity_run_liveness.sql   # NEW
│       ├── queries/activityrun.sql                      # MODIFY: heartbeat, timed-out, sweep queries
│       └── queries/job.sql                              # MODIFY: embedding hash
packages/shared/src/index.ts    # MODIFY: ACTIVITY_STATES, ActivityRunDto, QueueBacklogDto
apps/dashboard/src/
├── features/status/StatusPage.tsx   # MODIFY: render new states, queue backlog panel
└── lib/api.ts                       # MODIFY: /activity/queues client
.env.example                          # MODIFY: document new keys
```

**Structure Decision**: Existing monorepo layout, unchanged. Backend work concentrates in
`apps/api/internal/queue` (new policy + middleware boundary), `internal/activity` (sweeper),
and `internal/matching`. Cross-boundary types go through `packages/shared` per
Constitution III. No new app, no new service.

## Implementation Sequence

Ordered so each step is independently shippable and testable, matching the spec's story
priorities.

**Step 0 — Baseline capture (blocking for SC-001/SC-003).** Record current wall-clock for a
50-job benchmark drain, median per-job elapsed, and per-job scores into
`specs/019-ai-job-throughput/baseline.json`. Must happen on the pre-change build.

**Step 1 — US1: hosted concurrency (P1).** `queue.TaskPolicy` + config keys →
`llm.Router.ProviderClass()` + `OllamaProvider.IsHosted()` → admission gate middleware →
`servers.go` pool sizing. Ships FR-001…FR-005, FR-018, and the FR-003 guard test.
Verifiable by quickstart Scenarios 1–2.

**Step 2 — US2: deadlines and recovery (P1).** Migration `00030` (`heartbeatAt`,
`timeoutMs`, `Job.embeddingHash`) → sqlc queries → `Recorder` heartbeat +
`FinishActivityRunTimedOut` → deadline in the same middleware from Step 1 →
`activity.Sweeper` wired in `platform.go` → widen `ListFailedActivityRuns` → shared types +
dashboard rendering. Ships FR-006…FR-012. Verifiable by Scenarios 3–5, 8.

**Step 3 — US3: local match speedup (P2).** `profile.Snapshot` cache → embedding hash skip →
`keep_alive` → LLM transport tuning. Ships FR-013, FR-014. Verifiable by Scenario 6 against
the Step-0 baseline.

**Step 4 — US4: backlog visibility (P3).** `GET /api/activity/queues` + `QueueBacklogDto` +
dashboard panel. Ships FR-016.

FR-015 (backoff under quota rejection) is largely existing behaviour via
`llm.rateLimitBreaker` (`internal/llm/errors.go:114-149`); Step 1 adds the parallel-load
test (Scenario 7) and confirms the breaker is shared per provider instance rather than per
call.

FR-017 (no conflicting writes at N>1) is satisfied by the existing `UpsertMatchResult`
keyed on `jobId`; Step 1 adds the concurrent-match integration test.

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Hosted concurrency 3 trips a provider's quota faster | Existing breaker backs off on 429 with `Retry-After`; Scenario 7 exercises it under parallel load; the knob is configurable down to 1 |
| Sweeper marks a live run `interrupted` after laptop suspend | A late in-process finish overwrites the sweeper's verdict (data-model §1.2); heartbeat interval is 4× under the stale threshold |
| Two API instances (not the current deployment shape) | Detection is heartbeat-staleness based, not process-start based, so it stays correct if the deployment ever scales out |
| Profile cache serves stale text after a profile edit | Version-keyed on profile `updatedAt` and invalidated on write; Constitution II makes this a hard requirement, not a nicety |
| 30% local speedup not reached by Step 3's levers | Baseline captured first, so the gap is measurable; `AI_CONCURRENCY_LOCAL` remains an escape hatch, and the spec's assumption that local parallelism stays at 1 can be revisited with data |
