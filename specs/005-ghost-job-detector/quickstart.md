# Quickstart: Validating the Ghost-Job Detector

How to prove the feature works end-to-end once implemented. Implementation steps belong in [tasks.md](./tasks.md), not here.

## Prerequisites

```bash
pnpm install
pnpm --filter @job-finder/shared build   # must precede other workspace packages (tygo output)
make up                                   # Postgres + Redis + Ollama via Docker Compose
```

Apply the new migration and confirm the table landed:

```bash
cd apps/api && goose -dir internal/db/migrations postgres "$DATABASE_URL" up
psql "$DATABASE_URL" -c '\d "JobSignal"'
```

Expect: `jobId` FK with `ON DELETE CASCADE`, `JobSignal_jobId_kind_unique` on `("jobId","kind")`, and `JobSignal_kind_score_idx`.

> **Seeding**: only ever seed `jobfinder_test`. Never run the seeder against the dev or main database.

## Level 1 — Unit tests (no network, no LLM)

```bash
cd apps/api
go test ./internal/ghostjob/... -v
```

Expected:

**Signal measurement (deterministic — SC-006)**

- `RepostCount` **can exceed 1**. Assert this against a fixture with the same `dedupeKey` seen in several ingestion runs. `Job` carries `Job_dedupeKey_unique`, so a `count(*) GROUP BY "dedupeKey"` implementation compiles, looks right, and can only ever return 1 — this assertion is the tripwire for the feature's most likely bug (research Decision 2).
- `DaysOpen` is `nil` — never `0`, never a huge number — when `Job.postedAt` is null, and `notes.daysOpen` records the reason (FR-011, SC-005).
- `AlwaysHiringCount` is `nil` when `Job.company` is `""`, whitespace, punctuation-only, or the ingestion placeholder `"Unknown"`. Two unrelated jobs both companied `"Unknown"` MUST NOT be counted as one employer.
- `AlwaysHiringCount` is `1` for a company with exactly one posting, and the prompt/test treats 1 as **no evidence** (SC-004).
- A job whose `Application.status` is `shortlisted`/`docs_generated`/`applied`/`interview`/`offer`/**`rejected`** is excluded from the always-hiring count; only `found` counts as unprogressed (FR-006).
- `CrossBoardCount` counts *distinct* `sourceKey` values other than the job's own, inside 60 days; the same JD seen twice on one board is not a cross-board duplicate.
- `CrossBoardCount` is `nil` for an empty or teaser-length description.
- Running measurement twice over unchanged fixtures yields byte-identical results (SC-006).

**Similarity hash**

- Two reformatted/re-wrapped copies of one JD hash within the duplicate threshold; two genuinely different postings do not.
- Cyrillic descriptions round-trip through normalization without mangling.

**Result validation**

- `GhostJobResult.Validate()` errors on `Score` < 0, `Score` > 100, `Confidence` < 0, `Confidence` > 1 (FR-010).
- A validation failure after the retry budget persists **nothing** and leaves a pre-existing row untouched.

**Persistence**

- Two upserts for the same `(jobId, 'ghost')` leave exactly one row, with the second's values (FR-009).
- Deleting the `Job` removes the `JobSignal` row (SC-010).
- Every persisted row has a non-null `score` in 0-100, a `model`, and a `signals` object containing a value or an explicit `"unknown: …"` note for all four signals (SC-003).
- The all-signals-unknown case writes **no row at all** and makes no LLM call.

**Dashboard components**

```bash
cd apps/dashboard && pnpm vitest run
```

- `GhostBadge` renders nothing for a score below 50, yellow for 50-79, red for 80-100 (FR-012).
- `GhostBadge` renders nothing when the job has no ghost signal — the card is unchanged from today (FR-017, SC-008).
- The detail panel renders all four signal values, the confidence, the model, and the explanation; an unmeasured signal shows as "unknown", not as `0` (FR-013).

## Level 2 — Live smoke (real local LLM, opt-in)

Behind the existing `live` build tag, matching repo convention:

```bash
cd apps/api
go test -tags live ./internal/ghostjob/ -run TestLive_GhostScore -v
```

Expected: a real Ollama call returns a schema-valid `GhostJobResult` with a score in 0-100, a confidence in 0-1, and a non-empty explanation. This is the canary for prompt or model drift — it is the only test that exercises the actual model.

Also assert here (Principle II, FR-019): the returned explanation mentions only the numbers it was given. It must not assert anything about the employer's intent, headcount, or hiring process that the four signals do not support.

## Level 3 — End-to-end click-through

```bash
make seed    # test database only
make dev
```

In the dashboard (`http://localhost:5173`):

1. **Feed** → a job with a stored ghost score of 85 shows a **red** badge next to its fit `ScoreBadge`; a job at 62 shows a **yellow** one; a job at 20 shows **none**; an unscored job shows none and is otherwise pixel-identical to today (Story 1; FR-012, FR-017, SC-008).
2. **Feed, ordering** → note the job order with the feature on. Every flagged job is still present, in the same position, at full opacity. Nothing is hidden, dimmed, or moved (FR-015, SC-007).
3. **Click a flagged job** → the detail page shows the ghost panel: score, confidence, model, all four signal values, and a plain-English explanation naming which signals drove it (Story 2; FR-013).
4. **Open a job with no `postedAt`** → the panel shows days-open as "unknown" with reduced confidence, and still shows a score (SC-005).
5. **Open an unscored job** → the panel is absent or shows an explicit "not scored yet" state — never an empty panel of zeroes (Story 2, scenario 4).
6. **Press the refresh button** → the panel updates in place within ~30s, no page reload; the button is disabled while the request is in flight (Story 3; FR-014, SC-011).
7. **Wait a day without pressing refresh** → the score is unchanged. Nothing re-scores on a schedule (Story 3, scenario 5).
8. **Fit score sanity** → re-run fit scoring on a ghost-scored job. The `MatchResult` changes; the `JobSignal` row does not (FR-008).

## Level 4 — Failure-mode checks

| Scenario | How to force it | Expected |
|---|---|---|
| No posting date (FR-011, SC-005) | Fixture job with `postedAt = NULL` | `daysOpen: null`, `notes.daysOpen` gives the reason, confidence lowered, score still produced |
| One-off company (SC-004) | Company with exactly one job in the corpus | `alwaysHiringCount: 1`, contributes no suspicion; job is not flagged on that signal alone |
| Unparseable company | Jobs with `company` = `""`, `"   "`, `"---"`, `"Unknown"` | `alwaysHiringCount: null` for each; the four jobs are **not** grouped into one employer cohort |
| Legitimate agency cross-post | One JD under 4 distinct `sourceKey`s, repost count 1, company with 1 posting | Score stays **below 80** — cross-board duplication alone never reaches the red band |
| Empty / teaser description | Job with a 50-character description | `crossBoardCount: null`, confidence lowered, no spurious duplicate matches |
| All signals unknown | No `postedAt`, blank company, empty description, first appearance | Service **declines to score**: no LLM call, no row written, no confident 50 emitted |
| Malformed / out-of-range model output (FR-010) | Fake provider returning `{"score": 140}` past the retry budget | Error returned, **nothing persisted**, any prior row byte-identical |
| Model unreachable (Story 3, scenario 4) | Stop Ollama, press refresh | Error surfaced in the UI, previous score intact, no partial row |
| Double-click refresh (Story 3, scenario 3) | Click the button twice rapidly | Second click ignored/disabled; one scoring run; upsert makes any race idempotent |
| Scoring failure isolation (FR-018, SC-009) | Fail scoring for one job mid-batch | Other jobs' scores and the ingestion run are unaffected |
| Job deleted (SC-010) | `DELETE FROM "Job" WHERE id = …` | Zero rows left in `JobSignal` for that id |
| Repost count trap (research Decision 2) | Same `dedupeKey` across 3 ingestion runs | `repostCount = 3`, not `1` |

## Regression gate

This change spans `apps/api` and `apps/dashboard`, so per Constitution Principle IV the binding gate is the full suite, not `go test` alone:

```bash
make test-lint
```

**Constitution Principle I check**: grep the feature for any code path that hides, filters, reorders, auto-rejects, or otherwise acts on a job based on its ghost score. There should be none — the score reaches exactly two places, a badge and a panel. Confirm the feed's job list is identical with the feature on and off (SC-007).

**Constitution Principle III check**: `packages/shared` was rebuilt from tygo output, not hand-edited. `git diff` on the generated TS files should show only regenerated content.
