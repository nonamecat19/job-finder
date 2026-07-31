# Quickstart: Verifying Explicit Database Connection Capacity

Manual verification of every acceptance scenario. Run from the repository root. Long-lived processes are started via `process-hive`, per AGENTS.md.

## Prerequisites

```bash
docker compose up -d postgres redis
cd apps/api && go build ./... && cd ../..
```

---

## 1. Explicit capacity replaces the driver default (US2 §1)

```bash
make run-backend      # via process-hive
```

Expect exactly one line in the startup log:

```
level=INFO msg="db pool configured" max_conns=25 derived=true workers=15 background=2 reserve=8 ...
```

**Fails if**: `max_conns` reports `4` (or your core count), meaning the pool is still defaulted.

Cross-check against the live pool:

```bash
curl -s localhost:3000/api/health/ready | jq .pool.max_conns   # 25
```

---

## 2. Inconsistent concurrency is rejected at startup (US2 §2)

```bash
DB_MAX_CONNS=6 make run-backend
```

Expect startup to fail with:

```
config: DB_MAX_CONNS=6 is below the 25 connections required by worker concurrency
(workers=15 background=2 reserve=8). Raise DB_MAX_CONNS, or lower
AI_CONCURRENCY_CLOUD / INGEST_CONCURRENCY / ENRICH_CONCURRENCY.
```

**Fails if**: the process starts anyway, or the message does not name both the capacity setting and the concurrency settings.

Then confirm the inverse — raising concurrency past a fixed capacity is caught too:

```bash
DB_MAX_CONNS=25 AI_CONCURRENCY_CLOUD=20 make run-backend   # must fail
```

---

## 3. Invalid values are rejected (US2 §5)

```bash
DB_MAX_CONNS=-1 make run-backend        # config: DB_MAX_CONNS must be >= 0
DB_MIN_CONNS=99 make run-backend        # config: DB_MIN_CONNS (99) exceeds DB_MAX_CONNS (25)
DB_ACQUIRE_TIMEOUT=0s make run-backend  # config: DB_ACQUIRE_TIMEOUT must be > 0
DB_INTERACTIVE_RESERVE=0 make run-backend  # config: DB_INTERACTIVE_RESERVE must be >= 1
```

Each must fail before opening any connection, naming the offending key.

---

## 4. Over-declared capacity warns but starts (US2 §4)

```bash
DB_MAX_CONNS=250 DB_SERVER_MAX_CONNS=100 make run-backend
```

Expect a warning naming both keys, and a successful start. This is a warning rather than a failure because `DB_SERVER_MAX_CONNS` is operator-declared and may be stale (research.md R4).

---

## 5. Defaults start clean on any host (SC-005)

```bash
GOMAXPROCS=1 make run-backend    # no warning
GOMAXPROCS=32 make run-backend   # no warning; still max_conns=25
```

The second case is the Edge Case where the old incidental default (32) exceeded the new explicit one. Deliberate — see contracts/config.md "Backwards compatibility".

---

## 6. Dashboard stays responsive under full background load (US1, SC-001, SC-002)

Baseline first:

```bash
for i in $(seq 20); do
  curl -s -o /dev/null -w '%{time_total}\n' localhost:3000/api/jobs?limit=50
done | awk '{s+=$1} END {print "idle mean:", s/NR}'
```

Saturate every worker pool — trigger ingestion across all enabled sources and let match/ghost queues fill:

```bash
# There is no run-all endpoint; fan out over the enabled sources.
curl -s localhost:3000/api/sources | jq -r '.[] | select(.enabled) | .key' \
  | xargs -P8 -I{} curl -s -XPOST localhost:3000/api/sources/{}/run
watch -n2 'curl -s localhost:3000/api/health/ready | jq .pool'
```

Once `acquired_conns` is consistently high, repeat the timing loop.

**Pass**: loaded mean ≤ 1.5 × idle mean (SC-001), and zero non-200 responses (SC-002).

**Fails if**: any request returns a connection-capacity error, or the mean exceeds 1.5×.

---

## 7. Saturation is visible before anyone complains (US3, SC-004)

Force it:

```bash
DB_MAX_CONNS=25 DB_INTERACTIVE_RESERVE=1 make run-backend
# then saturate as in step 6
curl -s localhost:3000/api/health/ready | jq .pool
```

Expect `saturated: true`, `empty_acquire_count` climbing between two successive reads, and `acquire_duration_ms` climbing.

After ~2 minutes of continuous saturation, expect exactly one warning in the log per sustained episode — not one per sample:

```
level=WARN msg="db pool saturated" max_conns=25 acquired_conns=25 consecutive_saturated_samples=4
```

**Fails if**: the log emits on every 30s sample regardless of duration (that is a burst, not exhaustion — see data-model.md §4), or never emits.

---

## 8. Interactive requests fail fast rather than hanging (FR-008a)

With the pool deliberately starved as in step 7:

```bash
time curl -s -o /dev/null -w '%{http_code}\n' localhost:3000/api/jobs?limit=50
```

**Pass**: either a normal `200`, or a failure returned within roughly `DB_ACQUIRE_TIMEOUT` (5s) whose body identifies connection-capacity exhaustion.

**Fails if**: the request hangs past 30s. That is the pre-feature behaviour.

---

## 9. Recovery after database restart (SC-006, FR-011)

```bash
docker compose restart postgres
sleep 5
curl -s localhost:3000/api/health/ready | jq '.ok, .pool'
```

**Pass**: within 60s, `ok: true` and `total_conns` back at or above `DB_MIN_CONNS`, with no API restart.

---

## 10. Automated suites

```bash
make lint-go
go test ./internal/config/... ./internal/db/... ./internal/httpapi/...
go test -tags integration ./internal/db/...
make test-lint
```

All must pass. `make test-lint` is the merge gate per AGENTS.md.
