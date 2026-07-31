# Phase 0 Research: Batched, Atomic Ingest Persistence

All Technical Context unknowns resolved. Findings verified against this repository, not from memory.

---

## R1 — What does the current persist phase actually cost?

**Decision**: 2–4 queries per posting, plus 2 activity inserts per new posting. Confirmed by reading the code path, not inferred.

**Evidence** — `internal/jobsources/interfaces/worker/handler.go:191-199` loops `persistIfNew` over every posting. `persistIfNew` (`handler.go:247+`):

| Case | Queries |
|---|---|
| Already known | `GetJobByDedupeKey` + `RecordJobRepost` = **2** |
| New, non-board source | `GetJobByDedupeKey` + `InsertJob` = **2**, plus 2 × `activity.New` insert = **4** |
| New, board vendor, merged | `GetJobByDedupeKey` + `FindJobByCompany` + `MergeJobBoard` = **3** |
| New, board vendor, not merged | `GetJobByDedupeKey` + `FindJobByCompany` + `InsertJob` + 2 activity = **5** |

A 500-posting run of new board-vendor postings is therefore ~2500 sequential round trips.

**Additional finding**: `FindJobByCompany` is `SELECT … WHERE LOWER("company") = LOWER($1) AND "sourceKey" != $2 ORDER BY "ingestedAt" DESC LIMIT 1` (`queries/job.sql`). `grep -rn 'CREATE.*INDEX.*Job' internal/db/migrations/` returns indexes on `ingestedAt`, `status`, `subscriptionId` and a partial detail-pending index — **none on `company`, and none functional on `LOWER(company)`**. Every board-vendor posting scans the whole `Job` table. This was not in the original problem statement and is the larger of the two costs once the table grows.

---

## R2 — Bulk insert: copy-style load, batch protocol, or one multi-row statement?

**Decision**: one multi-row `INSERT … SELECT … FROM unnest($1::text[], $2::text[], …) ON CONFLICT ("dedupeKey") DO NOTHING RETURNING "id", "dedupeKey"` per chunk.

**Evidence**: `Job` carries `CONSTRAINT "Job_dedupeKey_unique" UNIQUE("dedupeKey")` (`00001_init.sql:48`), and `sqlc.yaml` generates for `sql_package: "pgx/v5"`.

**Rationale**:

- **Copy-style bulk load rejected**: PostgreSQL's `COPY` has no `ON CONFLICT` clause. Conflict handling is mandatory here — concurrent runs collide on `dedupeKey` (FR-013), and a run's own results can contain duplicate identities (FR-008). A copy-style load would fail the whole batch on the first collision.
- **Batch protocol (sqlc `:batchexec`) rejected**: it pipelines N statements into one round trip, which fixes latency but leaves N statements for the planner and gives no single `RETURNING` set identifying which rows were newly inserted. That set is needed to queue downstream work for new postings only (FR-010).
- **`unnest` chosen**: one statement, one plan, `ON CONFLICT DO NOTHING`, and `RETURNING` yields exactly the newly-inserted rows. Passing one array per column also sidesteps the 65535 bind-parameter ceiling entirely — ~14 arrays rather than 14 × N scalars.

**Consequence**: repeat sightings cannot ride the same statement, because `DO NOTHING` yields no row for conflicting postings. They are handled by a second set-based statement (R3).

---

## R3 — How is at-most-once sighting counting achieved across retries?

**Decision**: add `lastSeenRunId uuid` to `Job`. The bulk repost update becomes:

```sql
UPDATE "Job" SET "seenCount" = "seenCount" + 1, "ingestedAt" = now(),
  "subscriptionId" = COALESCE("subscriptionId", $2),
  "lastSeenRunId" = $3
WHERE "dedupeKey" = ANY($1) AND ("lastSeenRunId" IS DISTINCT FROM $3)
```

**Evidence of the bug**: `RecordJobRepost` (`queries/job.sql:41`) increments `seenCount` unconditionally. `ProcessTask` returns the error on any persist failure (`handler.go:194-197`), and the ingest task is retryable, so asynq re-runs the whole loop. Every posting stored before the failure is then re-classified as known and re-counted. `00016_job_seen_count.sql` documents that `seenCount` exists specifically to feed the ghost-job repost signal — so the inflation degrades a user-visible quality signal.

**Rationale**: the run's own id is already available (`InsertSourceRun` returns `run.ID` at `handler.go:96`), so the guard costs one column and one predicate. `IS DISTINCT FROM` rather than `!=` because the column is NULL for every pre-existing row.

**Alternatives rejected**:

- *Make ingest non-retryable* — discards transient-failure recovery for every source, and scraping sources are explicitly best-effort/unstable per the project's architecture constraints. Trading real resilience for a counting fix is a bad trade.
- *Time-window heuristic* ("don't re-count within 5 minutes") — a retry delayed past the window still double-counts. It fixes the common case and hides the rest, which is worse than not fixing it.

**Backfill**: `lastSeenRunId` is nullable with no default; existing rows stay NULL and are counted once by their next run. No backfill needed.

---

## R4 — How is merge-candidate resolution batched?

**Decision**: one `FindJobsByCompanies` taking a company array and returning the best candidate per company, plus a functional index `CREATE INDEX "Job_lower_company_idx" ON "Job" (LOWER("company"))`.

**Evidence**: `FindMergeCandidate` (`dedupe.go:95`) calls `FindJobByCompany` per posting and then applies `titlesOverlap` in Go. Only the lookup is batched; `titlesOverlap` stays in Go, unchanged, applied to the batch result — it is pure string logic with existing test coverage that should not be reimplemented in SQL.

**The index is a prerequisite, not an optimisation**: without it the batched form is one sequential scan instead of N, which is better but still O(table) per run. With it, the lookup is index-served, satisfying FR-007.

**Consequence**: the per-company `ORDER BY "ingestedAt" DESC LIMIT 1` becomes a `DISTINCT ON (LOWER("company"))` over the company set. Semantics are preserved: most recently ingested candidate per company, excluding the same source key.

---

## R5 — What is the transaction scope, and where does enqueueing sit?

**Decision**: the entire persist phase — classification, bulk insert, bulk repost update, merge updates, activity inserts, and the `FinishSourceRunOk` totals — runs inside one `db.WithinTx`. Enqueueing happens **after** commit.

**Evidence**: `db.WithinTx` exists at `internal/db/db.go:56` with a doc comment describing exactly this use ("when two writes must not diverge"), and has exactly one caller today — `internal/applications/application/service.go:49`, via the port at `internal/applications/domain/port.go:44`. The pattern to copy is already in the codebase.

**Why enqueue is outside**: asynq writes to Redis. A Redis call cannot enrol in a Postgres transaction, and holding the transaction open across it converts a queue stall into a database lock held across a network call. `ListJobsMissingMatch` already exists as the compensating sweep — its own comment says "Insert and enqueue aren't atomic, so a crash between the two strands the job". That guarantee is unchanged by this feature; SC-005 requires only that the sweep's workload not *increase*.

**Consequence**: the handler needs a transaction port alongside its existing `Repository` port, mirroring `applications/domain/port.go`. The port takes `func(*sqlcgen.Queries) error` because `db.WithinTx` does, deliberately, so `db` need not import use-case packages.

---

## R6 — What about the activity rows?

**Decision**: batch them too, via a new `BulkInsertActivities`.

**Evidence**: `enqueueMatch` and `enqueueGhostScore` each call `activity.New(ctx, h.q, …)` before enqueueing (`handler.go:320-325`, and the ghost equivalent). At 500 new postings that is 1000 individual inserts.

**Rationale**: leaving them per-row means the N+1 simply relocates from the persist loop to the enqueue loop, and SC-002 (≤10 interactions per chunk) is not met. Batching them is not scope creep — it is required for the feature's own success criterion.

**Constraint**: activity ids are needed as asynq `TaskID`s (`handler.go:335`), so the bulk insert must `RETURNING "id"` in a form that can be correlated back to each posting. Ordering the `RETURNING` set is not guaranteed in general; correlate by returning `jobId` alongside `id` rather than relying on row order.

---

## Summary of decisions

| ID | Decision |
|---|---|
| R1 | Current cost confirmed at 2–5 queries/posting; `LOWER(company)` has no index — new finding |
| R2 | `unnest` + `ON CONFLICT DO NOTHING` + `RETURNING`; not `COPY`, not batch protocol |
| R3 | `lastSeenRunId` column guards the repost update; retry cannot double-count |
| R4 | Batched company lookup + functional index on `LOWER("company")`; `titlesOverlap` stays in Go |
| R5 | Whole persist phase in `db.WithinTx`; enqueue after commit, sweep unchanged |
| R6 | Activity rows batched too, correlated by `jobId` not row order |
