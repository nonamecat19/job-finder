# Implementation Plan: Batched, Atomic Ingest Persistence

**Branch**: `025-batch-ingest-persistence` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/025-batch-ingest-persistence/spec.md`

## Summary

`worker.Handler.ProcessTask` (`internal/jobsources/interfaces/worker/handler.go:191`) loops over every posting a source returned and calls `persistIfNew`, which issues 2–4 sequential queries per posting: `GetJobByDedupeKey`, then either `RecordJobRepost` or (for board vendors) `FindJobByCompany` + `MergeJobBoard` or `InsertJob`, plus `activity.New` inserts before each enqueue. Nothing wraps the loop in a transaction, and `RecordJobRepost` increments `seenCount` unconditionally — so an asynq retry after a mid-loop failure re-counts every already-stored posting, corrupting the ghost-job repost signal.

Five changes, all in `apps/api`:

1. **Batch classification** — one `GetJobsByDedupeKeys` returning the known subset, replacing N `GetJobByDedupeKey` calls.
2. **Batch merge-candidate resolution** — one `FindJobsByCompanies` for all board-vendor postings, plus the missing index on `LOWER("company")`.
3. **Bulk upsert** — one multi-row `INSERT … SELECT FROM unnest(…) ON CONFLICT ("dedupeKey") DO NOTHING RETURNING` per chunk, and one set-based `UPDATE` for repeat sightings.
4. **Run-scoped transaction** — the whole persist phase runs inside `db.WithinTx`, which exists but currently has exactly one caller.
5. **Retry-safe sighting counts** — a `lastSeenRunId` column so a retried run cannot double-count.

## Technical Context

**Language/Version**: Go 1.26 (`apps/api` only)

**Primary Dependencies**: existing — `jackc/pgx/v5` v5.10.0, sqlc (pinned in `apps/api/.sqlc-version`), goose v3.27.2, `hibiken/asynq` v0.26.0. **No new dependencies.**

**Storage**: PostgreSQL 16. **One migration required** (`00032_*`): a functional index on `LOWER("company")`, and a `lastSeenRunId` column on `Job`.

**Testing**: `go test ./...` for batching/dedupe logic; `go test -tags integration ./...` against real Postgres for transaction and concurrency behaviour, per Constitution Principle IV. Existing coverage in `internal/jobsources/interfaces/worker/{merge_test,scheduler_test}.go` must keep passing.

**Target Platform**: Linux; Docker Compose dev and prod.

**Project Type**: Backend ingestion change. No dashboard change, no API contract change.

**Performance Goals**: storing 500 postings in ≤5% of current wall-clock time (SC-001); ≤10 database interactions per chunk regardless of posting count (SC-002).

**Constraints**:

- **`apps/api` does not currently compile** — seven packages broken by the DDD restructure (see tasks.md Phase 0). This feature cannot start until that is resolved.
- `Job.dedupeKey` carries `CONSTRAINT "Job_dedupeKey_unique" UNIQUE("dedupeKey")` (`00001_init.sql:48`), which is what makes `ON CONFLICT ("dedupeKey") DO NOTHING` viable and what forces conflict handling rather than a copy-style bulk load.
- `FindJobByCompany` filters on `LOWER("company") = LOWER($1)` and **no index supports it** — `Job` has indexes only on `ingestedAt`, `status`, `subscriptionId` and a partial detail-pending index. Every board-vendor posting currently triggers a sequential scan. The index is a prerequisite for the batched form, not an optional extra.
- PostgreSQL's bind-parameter ceiling is 65535. The `unnest` form passes one array per column (~14 arrays) rather than one parameter per value, so the ceiling is not the binding constraint — but chunking at 500 is retained to bound statement size and lock duration.
- sqlc's `pgx/v5` driver supports array parameters and `:many` returning; **no `:copyfrom`** is used because it cannot express `ON CONFLICT`.
- `db.WithinTx(ctx, fn func(*sqlcgen.Queries) error)` already exists (`internal/db/db.go:56`) and deliberately takes the concrete generated type so `db` need not import use-case packages. The ingest handler currently holds `h.q Repository` (a structural interface over `*sqlcgen.Queries`); it needs a transaction port alongside it, mirroring `internal/applications/domain/port.go:44`.
- Queueing cannot join the transaction (Redis). Work is enqueued after commit; `ListJobsMissingMatch` remains the safety net, exactly as today.
- `activity.New` writes an activity row per enqueued task. At 500 new postings that is 1000 inserts — batching these is in scope, otherwise the N+1 simply moves.

**Scale/Scope**: realistic run sizes 50–500 postings; ~8 files touched, 1 migration, sqlc regeneration required.

## Constitution Check

| Principle | Assessment |
|---|---|
| I. No Auto-Apply, Ever | **N/A** — ingestion discovers and stores; no application is submitted. |
| II. Grounded Generation | **N/A** — no LLM-generated content. Downstream scoring is unchanged. |
| III. Typed Contracts | **Respected.** New queries regenerate through sqlc (`make sqlc-generate`, enforced by the `sqlc-drift` CI job). No DTO change, so no tygo regeneration and no `packages/shared` edit. |
| IV. Test Discipline | **Directly exercised.** Transaction rollback, concurrent-collision and retry-idempotency behaviour are only meaningful against real Postgres; all three get integration tests rather than mocks. |
| V. Local-First | **Respected.** No external service. Reduces load on the self-hosted database. |
| **Migration rule** (Tech Constraints) | Next free goose version is `00032`; `00031_djinni_scraping_enhance.sql` is the current maximum. Version must be unique and sequential. |

**Re-check after Phase 1**: no violations. One accepted scope limit (post-commit enqueue) is inherited from today's behaviour, not introduced here.

## Project Structure

### Documentation (this feature)

```text
specs/025-batch-ingest-persistence/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── queries.md      # New/changed sqlc queries and their semantics
│   └── migration.md    # 00032 schema change and backfill
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── db/
│   │   ├── migrations/00032_batch_ingest.sql   # NEW: LOWER(company) index, lastSeenRunId
│   │   ├── queries/job.sql                     # + GetJobsByDedupeKeys, FindJobsByCompanies,
│   │   │                                       #   BulkInsertJobs, BulkRecordJobReposts
│   │   ├── queries/activityrun.sql             # + BulkInsertActivities
│   │   └── sqlcgen/                            # regenerated (never hand-edited)
│   └── jobsources/
│       ├── domain/repository.go                # + batch method signatures on the port
│       └── interfaces/worker/
│           ├── handler.go                      # persist phase → batched, transactional
│           ├── persist.go                      # NEW: batch classification + chunking
│           ├── persist_test.go                 # NEW: unit tests
│           ├── dedupe.go                       # FindMergeCandidate → batch form
│           └── persist_integration_test.go     # NEW: //go:build integration
└── (no dashboard, no packages/shared changes)
```

**Structure Decision**: the batching logic goes in a new `persist.go` beside `handler.go` in the existing `interfaces/worker` package rather than a new package. It is an adapter concern with one caller, and `handler.go` is already 475 lines — splitting the persist phase out improves it without inventing a boundary that has no second consumer.

## Phase 0: Research

See [research.md](./research.md). Six questions resolved: current cost (R1), bulk-insert mechanism (R2), retry idempotency (R3), merge-candidate batching and the missing index (R4), transaction scope and the enqueue boundary (R5), activity-row batching (R6).

## Phase 1: Design

- [data-model.md](./data-model.md) — batch structures, chunking rules, the `lastSeenRunId` semantics.
- [contracts/queries.md](./contracts/queries.md) — each new sqlc query, its signature and its guarantees.
- [contracts/migration.md](./contracts/migration.md) — the `00032` schema change, index rationale, backfill and rollback.
- [quickstart.md](./quickstart.md) — manual verification per acceptance scenario.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|--------------------------------------|
| New `lastSeenRunId` column on `Job` | FR-005 (at-most-once sighting per run) cannot be satisfied without per-posting knowledge of which run last counted it. | Making the ingest task non-retryable was rejected — it discards genuine transient-failure recovery for every source, which is worse than the counting bug. A time-window heuristic was rejected — a retry delayed past the window still double-counts, so it fixes the common case and hides the rest. |
| Batching activity-row inserts (FR-010 support) | At 500 new postings the per-task `activity.New` calls are 1000 inserts — leaving them per-row means the N+1 moves from persist to enqueue and SC-002 is not met. | Skipping activity rows for batched ingestion was rejected: they drive the dashboard Status page, and losing them for exactly the largest runs is the opposite of useful. |
| Post-commit enqueue (not atomic with storage) | Redis cannot participate in a Postgres transaction. | Holding the transaction open across Redis calls was rejected — it converts a queue stall into a database lock held across a network call. This is today's guarantee unchanged, and `ListJobsMissingMatch` already exists precisely for it. |
