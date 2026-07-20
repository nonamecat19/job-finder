# Implementation Plan: Ghost-Job Detector

**Branch**: `005-ghost-job-detector` | **Date**: 2026-07-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-ghost-job-detector/spec.md`

## Summary

Score each job 0-100 on how likely it is to be a posting the employer has no intent to fill. Four signals are measured deterministically (repost count via the existing `dedupeKey`, days open, cross-board JD duplication, "always hiring" company pattern), then blended by the local LLM through `llm.CompleteStructured[GhostJobResult]` — the same shape `matching.FitResult` already uses. The result persists to a new `JobSignal` table (`kind='ghost'`, `UNIQUE(jobId, kind)`), surfaces as a coloured badge on the feed and a breakdown panel on the detail page, and is refreshed only when the user asks.

**This feature takes no action on any job.** It renders a badge and stops. Nothing is hidden, dimmed, reordered, or auto-rejected.

## Technical Context

**Language/Version**: Go 1.23+ (`apps/api`), TypeScript/React (`apps/dashboard`)

**Primary Dependencies**: existing `internal/llm` (Ollama, structured output), existing sqlc + goose tooling. **No new Go or npm dependency** — the similarity hash is ~40 lines of stdlib (see research Decision 3).

**Storage**: PostgreSQL. **One new table (`JobSignal`), one new goose migration (`00007_job_signal.sql`), one new sqlc query file.** No existing table or column is altered.

**Testing**: `go test ./internal/ghostjob/... ./internal/httpapi/...`; `vitest` for the dashboard components; live LLM smoke behind the existing `live` build tag. Change crosses `apps/api` and `apps/dashboard`, so **`make test-lint` is a required gate** (Principle IV).

**Target Platform**: Linux server via Docker Compose, same containers as today.

**Project Type**: Web service (Go API) + React dashboard. New internal package plus UI surfaces on two existing screens.

**Constraints**: Signal measurement must be deterministic (SC-006) — all four counts come from SQL, never from the model. Scoring runs on local Ollama only (FR-020). The repost signal must join on the stored `dedupeKey`, never a recomputed identity (FR-003).

**Scale/Scope**: Single-user self-hosted; corpus in the thousands of jobs. One new internal package (~400 LOC + tests), one migration, one query file, two dashboard surfaces.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| **I. No Auto-Apply, Ever** | ✅ **PASS** | **This feature does not act on jobs — it is badge-only.** No submission, no message, no employer or board contact (FR-016), and per the user-approved policy it does not even hide, dim, reorder, or auto-reject a flagged job (FR-015, SC-007). The feed's membership and ordering are identical with the feature on and off. The score informs a human decision; every downstream action stays a human action. `JobSignal` has no column capable of recording an action taken, so the schema cannot express a violation. See research Decision 7. |
| **II. Grounded Generation** | ✅ PASS | The model never measures — it receives four numbers computed in SQL and blends them. Every claim in the stored explanation traces to a measured signal, and unmeasurable signals are passed as explicit `null` + a reason in `signals.notes` rather than guessed (FR-011, FR-019). The prompt forbids asserting facts about the employer or the role that the signals do not support. |
| **III. Typed Contracts** | ✅ PASS | `GhostJobResult` and the signal DTO are authored in Go and regenerated to TS via tygo — no hand-written TS mirror. DB access is sqlc-generated from `jobsignal.sql`. `packages/shared` must be rebuilt before the dashboard compiles (Workflow gate). |
| **IV. Test Discipline** | ✅ PASS | `go test` for signal computation, validation, and the HTTP surface; `vitest` for the badge/panel components; live LLM smoke behind the `live` tag. The change spans `apps/api` and `apps/dashboard`, so **`make test-lint` is mandatory** here — unlike spec 001, this is not optional. |
| **V. Local-First, Self-Hosted** | ✅ PASS | Scoring runs against local Ollama via the existing provider, using the `LLM_MODEL_GHOST` → `LLMModel` fallback that mirrors `LLM_MODEL_MATCH`. No third-party paid inference API. No external network call of any kind — scoring reads only rows the system already holds (FR-016). |

**Additional architecture-constraint check**: goose version numbers must be unique and sequential. `00007_job_signal.sql` is the next free number (`00006_adhoc_documents.sql` is current HEAD). If another in-flight feature lands `00007` first, this migration renumbers rather than duplicating.

**Post-Phase-1 re-check**: ✅ Still passing. Phase 1 added one table, one read-only query set, and two presentational components. It added no action path, no external dependency, and no hand-written cross-language type. Principle I's margin actually widened during design: auto-hide was considered and explicitly rejected (research Decision 6), so the badge-only boundary is now a recorded product decision rather than an implementation accident.

**No violations. Complexity Tracking section omitted.**

## Project Structure

### Documentation (this feature)

```text
specs/005-ghost-job-detector/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0: signal-source and storage decisions
├── data-model.md        # Phase 1: JobSignal table + reused types
├── quickstart.md        # Phase 1: validation guide (Levels 1-4)
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── db/
│   │   ├── migrations/
│   │   │   └── 00007_job_signal.sql      # NEW — JobSignal table, FK, unique, index
│   │   └── queries/
│   │       └── jobsignal.sql             # NEW — upsert/get/list + 3 signal counts
│   ├── ghostjob/                         # NEW package
│   │   ├── types.go                      # GhostJobResult + Validate() (mirrors matching/types.go)
│   │   ├── signals.go                    # deterministic measurement of the four signals
│   │   ├── simhash.go                    # normalized-description similarity hash
│   │   ├── service.go                    # measure → prompt → CompleteStructured → upsert
│   │   ├── ports.go                      # Repository interface (mirrors matching/ports.go)
│   │   └── *_test.go                     # unit tests over fixtures + fakes
│   ├── httpapi/
│   │   ├── jobs.go                       # MODIFIED — expose ghost signal on job + detail
│   │   └── ghostjob.go                   # NEW — POST /api/jobs/{id}/ghost-score (manual refresh)
│   ├── dto/dto.go                        # MODIFIED — JobSignalDto (tygo-exported)
│   └── config/config.go                  # MODIFIED — LLM_MODEL_GHOST
└── cmd/server/main.go                    # MODIFIED — construct + register the service

packages/shared/                          # REGENERATED — tygo output for the new DTO

apps/dashboard/src/
├── components/ui.tsx                      # MODIFIED — GhostBadge next to ScoreBadge
├── features/feed/FeedPage.tsx             # MODIFIED — badge on the card (line ~151)
├── features/job-detail/JobDetailPage.tsx  # MODIFIED — breakdown panel (line ~148)
└── api/api.ts, api/queryKeys.ts           # MODIFIED — refresh mutation + cache key
```

**Structure Decision**: Ghost detection gets its own `internal/ghostjob` package rather than living inside `internal/matching`. The two answer different questions with different invalidation triggers (research Decision 1), and mixing them would mean a profile-driven fit re-score shares a package — and eventually a code path — with a market-driven ghost re-score. `ghostjob` deliberately mirrors `matching`'s file layout (`types.go` / `ports.go` / `service.go` / `handler.go` shape) so the two read the same way.

## Key Design Decisions

Full reasoning in [research.md](./research.md). Summary:

1. **Separate `JobSignal` table, keyed `UNIQUE(jobId, kind)`** — not columns on `MatchResult` (whose `UNIQUE(jobId)` forbids multi-kind) and not columns on `Job` (the ingestion record holds no derived judgements). `kind` makes future signal families free. User-approved.
2. **Repost count joins the stored `dedupeKey`** — `sha256(lower(company)|lower(title)|canonicalUrl)`, computed by `ingestion.DedupeKey`. Never recompute it; drift between detector and ingestion is the whole risk.
   > **Trap**: `Job` carries `Job_dedupeKey_unique`, so `count(*) GROUP BY "dedupeKey"` can only ever return 1. The count must come from appearance/run history. Quickstart Level 1 asserts a repost count > 1 is reachable specifically to catch this.
3. **Cross-board duplication via simhash over normalized description text** — not exact match (boards re-wrap; finds nothing), not `pg_trgm` (extension not installed; slow on long text), and explicitly **not** `Job.embedding`, which measures semantic similarity and would call every similar Go role a duplicate.
4. **"Always hiring" measured against user progression**, with `Job.status = 'found'` narrowing and the `Application` join authoritative. `rejected` counts as *progression* — a rejection proves a human engaged, which is the opposite of ghosting.
5. **The model blends, code measures.** All four counts are SQL; the model returns `{Score, Confidence, Explanation, TopSignals}` through the existing `CompleteStructured` retry loop. Keeps SC-006 (deterministic measurements) and Principle II honest.
6. **Badge only, thresholds 50/80.** User-approved over auto-hide: a wrongly-flagged job that stays visible costs two seconds of skepticism; one that is silently hidden costs an opportunity the user never learns about.
7. **Manual re-score only.** No scheduler — a nightly corpus re-score is steady LLM load for scores nobody is reading, on a box that also runs embeddings and generation.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Repost count implemented as `GROUP BY "dedupeKey"` and silently always 1 | **High** — the unique constraint makes the wrong query compile and return plausible data | Explicit Level 1 assertion that a repost count > 1 is reachable; called out in research Decision 2 and in the task text, not left to reviewer memory. |
| "Always hiring" signal is noise on a fresh install (everything sits at `found`) | High, and accepted | The LLM weighs it against three other signals rather than a threshold firing; confidence carries the uncertainty. Documented as a known limitation, not hidden. |
| False positives on legitimate agency cross-posts | Medium | Prompt states cross-board duplication alone never justifies the red band; it only compounds. Asserted as a failure-mode test (quickstart Level 4). |
| Placeholder company `"Unknown"` groups unrelated employers into one giant cohort | Medium — adapters fall back to it per spec 001 FR-006 | `alwaysHiringCount` is `null` for empty/placeholder/punctuation-only companies; never grouped. Dedicated unit test. |
| Model returns an out-of-range score or malformed JSON | Medium | `GhostJobResult.Validate()` inside `CompleteStructured`'s retry loop; on final failure nothing persists and the prior row survives (FR-010). |
| Simhash threshold mis-tuned → duplicates over- or under-reported | Medium | Threshold is a named constant with a fixture-based test over known-duplicate and known-distinct JD pairs; tunable without touching the query. |
| Feed N+1 when fetching a badge per card | Medium | `ListJobSignalsByJobIds` batch query; the list endpoint issues one query, not one per job. |
| goose `00007` collides with another in-flight feature | Low | Renumber before merge; never duplicate a version (Constitution architecture constraint). |
| Scoring latency blocks ingestion | Low | Scoring is asynchronous relative to ingestion; a scoring failure on one job never fails the run or affects another job (FR-018, SC-009). |

## Phase Status

- [x] Phase 0: research.md — storage, signal-source, and policy decisions recorded
- [x] Phase 1: data-model.md, quickstart.md
- [x] Constitution re-check post-design — passing (Principle I passes by construction: badge only, no action)
- [x] Phase 2: tasks.md
