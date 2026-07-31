# Phase 1 Data Model: Batched, Atomic Ingest Persistence

## 1. Schema change (migration `00032_batch_ingest.sql`)

### 1.1 `Job.lastSeenRunId`

```sql
ALTER TABLE "Job" ADD COLUMN "lastSeenRunId" uuid;
```

Nullable, no default, no FK. Records which `SourceRun` last incremented this posting's `seenCount`.

- **Why nullable**: every pre-existing row has no such run. NULL means "never counted by a tracked run" and is handled by `IS DISTINCT FROM`, so no backfill is needed.
- **Why no foreign key**: `SourceRun` rows are pruned over time; a FK would either block pruning or cascade-null the guard, and the column is a comparison token, not a reference to be navigated.

### 1.2 Functional index on company

```sql
CREATE INDEX "Job_lower_company_idx" ON "Job" (LOWER("company"));
```

Serves the merge-candidate lookup. Without it the batched lookup is a single sequential scan per run instead of N — better, but still O(table). FR-007 requires index-served.

### 1.3 Supporting index for the guard predicate

None. `dedupeKey` already carries `CONSTRAINT "Job_dedupeKey_unique"` (`00001_init.sql:48`), which backs `WHERE "dedupeKey" = ANY($1)`.

---

## 2. In-process batch structures

### `PostingBatch`

The run's results after in-batch deduplication, before any database contact.

| Field | Meaning |
|---|---|
| `Postings []dto.NormalizedJob` | Deduplicated by `DedupeKey(company, title, url)` |
| `Keys []string` | Parallel dedupe keys, order-aligned with `Postings` |
| `SubscriptionID pgtype.UUID` | Run's subscription attribution, or zero |
| `RunID pgtype.UUID` | The `SourceRun` id — the retry guard token |
| `NeedsDetail bool` | From `jobsources.NeedsDetail(adapter)`; routes downstream work |

**In-batch deduplication (FR-008)** happens here, before the database is touched. When two postings in one run share a dedupe key, the **first** is kept. Not the last: adapters return results in source order, and the first occurrence is the one the source ranked higher. The dropped duplicates are counted and reported in the run's `found` total, so `found` still describes what the source returned.

### `Classification`

Result of the single batch lookup.

| Field | Meaning |
|---|---|
| `Known map[string]pgtype.UUID` | dedupe key → existing job id |
| `NewPostings []dto.NormalizedJob` | Not present in `Known` |
| `MergeTargets map[string]pgtype.UUID` | dedupe key → job to merge into (board vendors only) |

### `PersistResult`

Returned from the transaction, consumed by the post-commit enqueue phase.

| Field | Meaning |
|---|---|
| `Inserted []InsertedJob` | `{JobID, DedupeKey, Posting}` — only genuinely new rows |
| `Reposted int` | Postings whose sighting count was incremented |
| `Merged int` | Board postings folded into an existing job |
| `Skipped int` | In-batch duplicates dropped |

Only `Inserted` drives enqueueing. Reposted, merged and skipped postings queue nothing — matching today's behaviour exactly.

---

## 3. Chunking

**Chunk size**: 500 postings, configurable via `INGEST_PERSIST_CHUNK_SIZE`.

**Chunks are inside the transaction, not around it.** All chunks of a run commit together or none do. Chunking bounds statement size and lock duration; it is explicitly not a consistency boundary (spec Clarifications). A chunk failing rolls back the whole run.

**Parameter budget**: the `unnest` form passes one array per column (~14 arrays), so PostgreSQL's 65535 bind-parameter ceiling is not approached regardless of chunk size. 500 is chosen for statement size and lock duration, not parameter count.

---

## 4. Statement sequence per chunk

| # | Statement | Purpose |
|---|---|---|
| 1 | `GetJobsByDedupeKeys` | Classify known vs new (replaces N lookups) |
| 2 | `FindJobsByCompanies` | Merge candidates, board vendors only — skipped when none |
| 3 | `BulkInsertJobs` | `ON CONFLICT DO NOTHING RETURNING id, dedupeKey` |
| 4 | `BulkRecordJobReposts` | Set-based increment, guarded by `lastSeenRunId` |
| 5 | `BulkMergeJobBoards` | Board postings folded into existing jobs — skipped when none |
| 6 | `BulkInsertActivities` | Activity rows for the postings about to be enqueued |

Six statements, two of them conditional. **SC-002's budget is 10 per chunk**; the design uses at most 6, leaving margin.

Per run, outside the chunk loop: `InsertSourceRun` (already before the persist phase) and `FinishSourceRunOk` — both unchanged in count.

---

## 5. Repeat-sighting semantics

Unchanged from today except for the guard:

- `seenCount` increments by 1
- `ingestedAt` refreshes to `now()`
- `subscriptionId` backfills via `COALESCE` when previously null
- **New**: `lastSeenRunId` is set, and the row is skipped entirely when it already equals this run's id

The guard is `("lastSeenRunId" IS DISTINCT FROM $3)`, not `!=`, because NULL is the initial state for every existing row and `NULL != $3` evaluates to NULL, which would exclude every pre-existing row from ever being counted.

---

## 6. Transaction boundary

```
InsertSourceRun                    ← before the transaction (records the attempt)
adapter.Search                     ← outside; network I/O, must not hold a transaction
┌─ db.WithinTx ────────────────────────────────────────┐
│  for each chunk:                                     │
│    classify → insert → repost → merge → activities   │
│  FinishSourceRunOk (found, new totals)               │
└──────────────────────────────────────────────────────┘
enqueue match / ghost / enrich     ← after commit; Redis cannot join the transaction
```

`InsertSourceRun` stays outside so a run that fails during `adapter.Search` still leaves a record of the attempt — today's behaviour, preserved.

`FinishSourceRunOk` moves **inside**, so a run's totals cannot describe a storage phase that was rolled back (FR-011). On rollback, `finishError` runs after the failed transaction, marking the run failed — as today.

---

## 7. Concurrency

Two runs storing the same posting concurrently (FR-013):

- Both reach `BulkInsertJobs`; the unique constraint on `dedupeKey` arbitrates.
- The loser gets no `RETURNING` row for that key under `ON CONFLICT DO NOTHING`, so it treats the posting as not-inserted and queues nothing for it. Exactly one posting exists, neither run fails.
- The loser does **not** retroactively record a repeat sighting for it. That is a deliberate simplification: the posting was genuinely new at classification time, and a run that classified it as new should not also count it as re-seen. The sighting is recorded on the next run that sees it.

---

## 8. Accepted scope limits

- **Enqueue is not atomic with storage.** Unchanged from today; `ListJobsMissingMatch` remains the safety net (research.md R5). SC-005 requires only that the sweep's workload not increase.
- **`found` counts what the source returned**, including in-batch duplicates; `new` counts rows actually inserted. Both were already approximate under partial failure and are now exact.
- **Posting identity is unchanged.** `DedupeKey(company, title, canonicalURL)` is untouched, so every previously stored posting is still recognised (FR-012).
