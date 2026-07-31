# Quickstart: Verifying Batched, Atomic Ingest Persistence

Manual verification per acceptance scenario. Run from the repository root. Long-lived processes go through `process-hive` per AGENTS.md.

## Prerequisites

```bash
docker compose up -d postgres redis
cd apps/api && go build ./... && cd ../..   # must succeed — see tasks.md Phase 0
make seed                                    # gives the planner a non-trivial table
```

Enable query logging so interaction counts are measurable rather than asserted:

```bash
psql "$DATABASE_URL" -c "ALTER SYSTEM SET log_statement = 'all';"
docker compose restart postgres
```

---

## 1. Interaction count is constant in posting count (US1, SC-002)

Run a source twice at different result sizes and count statements against `"Job"`:

```bash
count_stmts() {
  docker compose logs --since 2m postgres \
    | grep -cE 'INSERT INTO "Job"|UPDATE "Job"|SELECT .* FROM "Job"'
}

curl -s -XPOST localhost:3000/api/sources/remotive/run   # ~50 postings
sleep 20 && echo "50 postings: $(count_stmts) statements"

curl -s -XPOST localhost:3000/api/sources/jobspy/run     # ~500 postings
sleep 60 && echo "500 postings: $(count_stmts) statements"
```

**Pass**: the second count is not ~10× the first. Expect ≤6 per 500-posting chunk (data-model.md §4).

**Fails if**: statement count scales with posting count — the batching did not take effect.

---

## 2. Storage wall-clock (SC-001)

The run's `SourceRun` row carries its timing. Compare the persist phase before and after:

```sql
SELECT "sourceKey", "found", "new", "finishedAt" - "startedAt" AS duration
FROM "SourceRun" ORDER BY "startedAt" DESC LIMIT 5;
```

**Pass**: for a ~500-posting run, storage time is ≤5% of the recorded pre-change baseline.

**Record the baseline first**, on the pre-change build — SC-001 is a ratio and is unverifiable without it. This is a task, not an afterthought (tasks.md T002).

---

## 3. Mixed known/new classification is correct (US1 §3)

```bash
curl -s -XPOST localhost:3000/api/sources/remotive/run   # first run: all new
curl -s -XPOST localhost:3000/api/sources/remotive/run   # second run: all known
```

```sql
SELECT "found", "new" FROM "SourceRun" WHERE "sourceKey"='remotive'
ORDER BY "startedAt" DESC LIMIT 2;
```

**Pass**: first run `new == found`; second run `new == 0`, `found` unchanged. Every posting's `seenCount` is exactly 2:

```sql
SELECT DISTINCT "seenCount" FROM "Job" WHERE "sourceKey"='remotive';   -- expect a single row: 2
```

---

## 4. The merge-candidate lookup is index-served (US1 §4, FR-007)

```bash
psql "$DATABASE_URL" -c 'EXPLAIN ANALYZE SELECT DISTINCT ON (LOWER("company")) LOWER("company"), "id"
  FROM "Job" WHERE LOWER("company") = ANY(ARRAY[$$acme$$,$$globex$$]) AND "sourceKey" != $$greenhouse$$
  ORDER BY LOWER("company"), "ingestedAt" DESC;'
```

**Pass**: plan shows `Index Scan using "Job_lower_company_idx"`.

**Fails if**: `Seq Scan`. On a small seeded table the planner may legitimately prefer a scan — re-check with `SET enable_seqscan = off` to confirm the index is usable, then re-verify on a larger table. An unused index does not satisfy FR-007.

---

## 5. A failed run leaves nothing behind (US2 §1)

Inject a failure mid-persist. Easiest reliable method — revoke insert permission after classification:

```bash
psql "$DATABASE_URL" -c 'REVOKE INSERT ON "Job" FROM jobfinder;'
curl -s -XPOST localhost:3000/api/sources/remotive/run
psql "$DATABASE_URL" -c 'GRANT INSERT ON "Job" TO jobfinder;'
```

```sql
SELECT count(*) FROM "Job" WHERE "sourceKey"='remotive';           -- unchanged from before
SELECT "error" FROM "SourceRun" ORDER BY "startedAt" DESC LIMIT 1; -- run marked failed
```

**Pass**: zero postings from the failed run, run recorded as failed.

**Fails if**: some postings landed — the transaction boundary is wrong.

---

## 6. Retry does not inflate sighting counts (US2 §2, SC-003) — the correctness fix

This is the scenario that justifies the feature. Record counts, force a mid-run failure, allow asynq to retry, compare.

```sql
-- before
SELECT "dedupeKey", "seenCount" FROM "Job" WHERE "sourceKey"='remotive' ORDER BY "dedupeKey";
```

Force failure and retry as in step 5, then let asynq retry (`ingest` has `MaxRetry` > 0):

```sql
-- after
SELECT "dedupeKey", "seenCount", "lastSeenRunId" FROM "Job"
WHERE "sourceKey"='remotive' ORDER BY "dedupeKey";
```

**Pass**: every `seenCount` is exactly **one** higher than before, regardless of attempt count, and `lastSeenRunId` equals the successful run's id.

**Fails if**: `seenCount` rose by the number of attempts. That is the pre-feature behaviour and means the `IS DISTINCT FROM` guard is not working — check it is `IS DISTINCT FROM` and not `!=` (NULL semantics, contracts/queries.md §4).

For SC-003, repeat the forced failure ten times and confirm the increment is still exactly one.

---

## 7. In-batch duplicates collapse (Edge Case)

Point a source at a fixture returning the same posting twice.

**Pass**: one `Job` row, `seenCount == 1`, run's `found` counts both (what the source returned), `new` counts one.

**Fails if**: two rows (dedup missed) or the run errors on a unique-constraint violation (`ON CONFLICT` missing).

---

## 8. Concurrent runs do not collide (FR-013)

```bash
curl -s -XPOST localhost:3000/api/sources/remotive/run &
curl -s -XPOST localhost:3000/api/sources/arbeitnow/run &
wait
```

With overlapping postings across the two sources:

**Pass**: one row per dedupe key, neither run reports an error.

**Fails if**: either run fails with a unique-constraint violation.

---

## 9. Downstream work still reaches every new posting (US3, SC-005)

```sql
-- jobs stored but never scored — the stranded-job sweep's workload
SELECT count(*) FROM "Job" j
LEFT JOIN "MatchResult" m ON m."jobId" = j."id"
WHERE m."id" IS NULL AND j."ingestedAt" < now() - interval '10 minutes';
```

**Pass**: not higher than the pre-change baseline (SC-005). Record that baseline before starting.

Also confirm no posting is scored twice:

```sql
SELECT "jobId", count(*) FROM "MatchResult" GROUP BY "jobId" HAVING count(*) > 1;  -- expect zero rows
```

---

## 10. Rolled-back runs queue nothing (US3 §4)

After the forced failure in step 5, inspect the queues:

```bash
curl -s localhost:8090/api/queues/match/pending_tasks | jq length
```

**Pass**: no tasks queued for postings that were never stored.

---

## 11. Automated suites

```bash
make sqlc-generate && git diff --exit-code apps/api/internal/db/sqlcgen   # no drift
make lint-go
go test ./internal/jobsources/...
go test -tags integration ./internal/jobsources/...
make test-lint
```

All must pass. `make test-lint` is the merge gate.

---

## Cleanup

```bash
psql "$DATABASE_URL" -c "ALTER SYSTEM RESET log_statement;"
docker compose restart postgres
```
