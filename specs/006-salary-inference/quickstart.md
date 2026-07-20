# Quickstart: Validating Salary Inference

How to prove the feature works once implemented. Implementation belongs in [tasks.md](./tasks.md), not here.

## Prerequisites

```bash
pnpm install
pnpm --filter @job-finder/shared build   # must precede other workspace packages
make up                                   # Postgres + Redis + Ollama via Docker Compose
```

Seeding runs against `jobfinder_test` only — never the dev or main database.

## Level 1 — Unit tests (no network, no model)

```bash
cd apps/api
go test ./internal/salary/... -v
```

Expected:

**Parser** (`parse_test.go`) — table-driven, one case per pattern in the corpus:

- `$3000-5000` → `{Min: 3000, Max: 5000, Currency: "USD", Period: "month"}`
- `від 60000 грн` → `{Min: 60000, Max: 60000, Currency: "UAH", Period: "month"}`
- `up to €70k` → `{Max: 70000, Currency: "EUR", Period: "year"}`
- `80k-100k PLN`, `3000–5000 $` (en-dash, trailing symbol), bare `$4500` all parse.
- `competitive`, `договірна`, `negotiable` → **no band and no error**. Never a zero-valued band — a `0`–`0` band is filtered out by every non-zero floor and would silently hide the job.
- A `$` amount on a posting with UA geo resolves to the geo-appropriate currency, not blindly to USD.
- Every band normalizes to annual before storage; a monthly figure stored as annual is a 12× error nothing downstream would catch.

**Blender** (`blend_test.go`):

- Two sources at `0.4` and `0.5` → band is the confidence-weighted average, confidence is `0.9`.
- Three sources at `0.5` each → confidence is **`1.0`, not `1.5`** (the cap).
- **A single source at `0.4` blends to exactly `0.4`, not `1.0`.** The blend-of-one no-op — the easiest bug here and an invisible one.
- A source returning an error contributes nothing and does not abort the others (FR-023).
- Zero usable sources → no band, no error (FR-009).
- A parsed `posting` band replaces the estimate outright rather than blending (FR-008), and `"salarySource"` is `posting`.

**Buckets** (`bucket_test.go`):

- `Sr. Backend Engineer` → title `backend engineer` with seniority extracted separately.
- A posting with no geo → `geoBucket` `"*"`.
- Fallback widening runs exact → size `unknown` → geo `*`, lowering confidence at each step.

## Level 2 — Integration against real Postgres and the local model

```bash
cd apps/api
make test-integration
```

Expected:

- **Migration 00009 applies and rolls back cleanly.** Up adds five `"Job"` columns, the partial index, and `"SalaryCache"`; down removes all of them.
- **The cache load is idempotent** — running it twice leaves the row count unchanged. Without the unique constraint the table grows by a full dataset per restart.
- **An unreachable dataset does not fail startup** (SC-010) — point the loader at a dead URL, confirm the server starts and serves the feed from whatever is already cached.
- **The LLM source** returns a schema-valid `SalaryBand` through `CompleteStructured` against the local Ollama instance. No third-party API is contacted (FR-025) — assert by inspecting the configured provider, not by trusting the config file.
- **Idempotence (FR-022, SC-009)**: run inference twice over an unchanged job set; the second run issues **zero** model calls.
- **Column integrity (SC-002)**: after a run, every job has all five salary columns set or all five `NULL`. Query directly for partial rows; the count must be zero.

## Level 3 — End-to-end through the running stack

```bash
make seed     # against jobfinder_test only
make dev
```

Then in the dashboard (`http://localhost:5173`):

1. **Feed** → jobs whose postings hid compensation now show a band instead of nothing (Story 1, SC-001).
2. **Low confidence** → a job whose confidence is below `0.3` renders visibly discredited, not with the same weight as a grounded estimate (FR-006).
3. **No band** → a job no source could estimate falls back to today's display and is not marked as anything (FR-009).
4. **Set `SALARY_FLOOR_USD`** to a value above some jobs' bands; reload. Those jobs are gone from the default feed (Story 2, FR-015).
5. **Toggle the below-floor filter off** → they reappear, each carrying a visible below-floor marker (FR-016).
6. **Straddling band** → a job whose min is below the floor and max above it stays visible with the filter on (Story 2, scenario 6).
7. **Band-less job** → still visible with the filter on. Absence of a band is never below-floor (FR-019).
8. **Set the floor to `0`** → nothing is filtered on salary grounds at all (FR-018).
9. **Check a filtered job's record** → status still `found`, unchanged (FR-017, SC-007).
10. **Pagination** → with the filter active, the reported total matches the rows actually returned across pages. A `CountJobs` predicate that diverges from the list queries shows up exactly here.
11. **Job detail** → the estimate breakdown names each contributing source with its own band and confidence (Story 3, FR-021, SC-008).
12. **A job with stated compensation** → the breakdown identifies the posting itself as the source, shows no estimate, and the original `salaryRaw` text is still displayed alongside (FR-024).

## Level 4 — Failure modes and accuracy

| Scenario | How to force it | Expected |
|---|---|---|
| Model unavailable (FR-023) | Stop the Ollama container, run inference | LLM source contributes nothing; dataset and ingested-cache sources still produce a blended band. No run failure. |
| Dataset unreachable (SC-010) | Point the loader at a dead URL, restart | Server starts, feed serves, previously cached buckets still answer lookups. Warning logged. |
| Empty cache, fresh install | Truncate `"SalaryCache"`, drop parsed bands, run inference | Estimates come from the LLM alone at its own confidence. Expected on day one, not a defect. |
| Every source empty | A job with an unmatched title in a geo with no buckets, model stubbed to fail | All five columns stay `NULL`; job renders as today; floor never hides it. |
| Unparseable currency vs. floor (FR-020) | A job with a band in a currency with no conversion rate, floor set | Job is **not** filtered out. Unfilterable fails open, never closed. |
| `NULL` band vs. floor predicate | Non-zero floor, job with `NULL` `"salaryMax"` | Job appears. This is the SQL three-valued-logic trap — `"salaryMax" >= $1` silently drops `NULL` rows and inverts FR-019. |
| Source disagreement | A job where the model and the dataset differ sharply | Both contribute at their own confidences; the resulting wide band and the breakdown expose the disagreement rather than hiding it. |

### Accuracy measurement

The parser's held-out set is the only ground truth available, and it is why the parser ships first ([research.md](./research.md) Decision 2).

- **SC-003 — parser accuracy**: hand-check a sample spanning every currency in the corpus; ≥95% of parsed bands must match the stated figures.
- **SC-004 — estimate accuracy**: take jobs with parseable stated compensation, hide it, estimate as if hidden. The estimated band must contain the true figure for ≥60% of them, **and reported confidence must correlate with whether it did.** The correlation is the point. A source that is wrong but honestly low-confidence is behaving correctly; one that is confidently wrong is the failure this feature can actually cause harm through.

## Regression gate

```bash
make test-lint
```

This change spans `apps/api` and `apps/dashboard`, so per Principle IV `make test-lint` is **binding, not optional** — `go test` alone does not gate it.

**SC-011**: with no floor set and no bands stored, feed ingestion, scoring, and display must be byte-for-byte what they were before this feature. Verify against the existing feed tests, unmodified.

**Constitution Principle I check**: grep the feature for any write to `"Job"."status"`. There should be none — the floor filter hides rows from a view and never mutates state.
