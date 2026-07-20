# Phase 0 Research: Ghost-Job Detector

**Date**: 2026-07-20. Decisions below resolve the open questions the spec deliberately left to planning.

---

## Decision 1: a separate `JobSignal` table, not columns on `MatchResult`

**Decision**: New table `"JobSignal"` with `UNIQUE("jobId","kind")`, `kind='ghost'`.

**Rationale**: `MatchResult` answers "how well does this job fit *me*" and is invalidated when the user's profile changes. Ghost detection answers "is this posting real" and is invalidated when the *market* changes (a repost, another 30 days open). Different inputs, different invalidation triggers, different re-score cadence. Folding ghost columns into `MatchResult` would mean a profile edit that re-runs fit scoring either wipes the ghost score or has to carefully preserve it — a coupling with no upside. `MatchResult` also carries `UNIQUE("jobId")`, which forbids the multi-kind future the `kind` column exists for.

**User-approved**: yes, explicitly.

**Alternatives considered**:

- *Columns on `MatchResult`* — rejected above.
- *Columns on `Job`* — rejected: `Job` is the ingestion record, a faithful normalization of what a board published. Derived judgements do not belong on it, and every future signal kind would widen the hottest table in the schema.
- *A dedicated `GhostScore` table* — rejected: correct today, but the second signal kind (salary realism, seniority mismatch) then adds a third near-identical table. `kind` costs one `text` column and one composite index and absorbs all of them.

---

## Decision 2: repost count joins on `dedupeKey`, never a recomputed identity

**Decision**: The repost signal groups on `Job.dedupeKey` — the value `ingestion.DedupeKey` already wrote — and counts appearances across ingestion runs.

**Rationale**: `DedupeKey` is `sha256(lower(company)|lower(title)|canonicalUrl)` (`apps/api/internal/ingestion/dedupe.go`), and its doc comment warns it must match `ingestion.processor.ts:74` byte-for-byte "or duplicate jobs flood in". A detector that recomputed the identity with even slightly different normalization would report reposts that ingestion considers distinct jobs, or miss reposts it considers the same. Reading the stored column removes the possibility of drift entirely. This is FR-003.

**Consequence**: `Job` carries `Job_dedupeKey_unique`, so a "repost" is *not* multiple `Job` rows. The count must come from run/appearance history (`SourceRun` correlation, or an appearance counter), not from `SELECT count(*) FROM "Job" GROUP BY "dedupeKey"` — which can only ever return 1. **This is the single most likely implementation bug in the feature** and the reason the quickstart asserts a repost count > 1 is actually reachable.

**Alternatives considered**: recomputing from `company`/`title`/`url` at score time — rejected, drift risk above. Grouping on `url` alone — rejected, misses the same posting relisted under a new URL, which is the exact behaviour the signal targets.

---

## Decision 3: cross-board duplicate detection via a normalized description hash, not exact match and not `pg_trgm`

**Decision**: Compute a similarity hash (simhash over shingled, normalized description tokens) in Go, store it alongside the score computation, and compare within a 60-day window across distinct `sourceKey` values.

**Rationale**:

- **Exact string match is useless here.** Boards re-wrap, strip, and re-render HTML; two copies of one JD almost never match byte-for-byte. It would report ~0 duplicates and the signal would be dead weight.
- **`pg_trgm` is not installed.** The schema enables `vector` only (`00001_init.sql:5`). Adding a second extension for one signal is a heavier dependency than the signal justifies, and trigram similarity over multi-kilobyte descriptions is expensive.
- **The existing `embedding` column is tempting but wrong.** `Job.embedding` is a `vector(768)` for *semantic* similarity, which is precisely what we do not want: two genuinely different Go backend roles are semantically close and would be scored as duplicates. We need *textual near-identity*, which simhash gives and embeddings do not.

**Alternatives considered**:

- *pgvector cosine on `embedding`* — rejected: measures the wrong kind of similarity (see above). Would produce false duplicates across every similar role in the corpus.
- *`pg_trgm` `similarity()`* — rejected: new extension, new migration, poor performance on long text.
- *MinHash / LSH* — rejected as over-engineering at single-user corpus size (thousands of rows). Revisit if the corpus reaches six figures.

**Guardrail carried into the design**: this signal is *weak on its own*. A recruitment agency legitimately posting one JD to four boards is the textbook false positive (spec edge case). The prompt must state that cross-board duplication alone never justifies a red-band score; it only compounds with repost count or always-hiring.

---

## Decision 4: "always hiring" is measured against user progression, with `Job.status` as a fast path and `Application` as the authority

**Decision**: Count `Job` rows sharing `lower(company)`, ingested within 90 days, whose status never advanced past `found` — verified by left-joining `Application` and excluding any job whose application status is in {`shortlisted`, `docs_generated`, `applied`, `interview`, `offer`, `rejected`}.

**Rationale**: The system has no visibility into the employer's ATS. The only evidence it holds about whether a posting led anywhere is the user's own progression, recorded as `Application.status` transitions by `applications.Service.Update` (`apps/api/internal/applications/service.go:71`). That same method mirrors the new status onto `Job.status` (line 100), so `Job.status = 'found'` is a cheap correct filter — but it is a *mirror*, and mirrors desync. The `Application` join is the authoritative check; `Job.status` narrows the candidate set first.

**`rejected` counts as progression.** A rejection means a human at the company engaged with the application. That is evidence of a real hiring process — the opposite of a ghost job. Counting it as "unprogressed" would flag the most-real postings in the corpus.

**Known limitation, accepted**: this signal is only meaningful once the user has actually worked some of their pipeline. On a fresh install every job sits at `found`, so a company with 9 postings scores 9 on this signal regardless of intent. Mitigated by (a) the LLM weighing it against the other three rather than a threshold rule firing, and (b) confidence. Not mitigated further — the alternative is inventing employer-side data the system cannot see.

**Alternatives considered**: counting *all* postings per company regardless of progression — rejected, that measures company size, not ghosting. A large employer genuinely hiring 40 engineers would score maximum on it.

---

## Decision 5: the LLM blends the signals; it does not measure them

**Decision**: All four signals are computed deterministically in SQL/Go. The model receives those numbers and returns `GhostJobResult{Score, Confidence, Explanation, TopSignals}` via `llm.CompleteStructured[GhostJobResult]`, mirroring `matching.FitResult`.

**Rationale**: A fixed threshold rule ("45 days ⇒ +30 points") is brittle and cannot express the interactions that actually matter — 60 days open is unremarkable for a niche senior role but damning for a junior role reposted three times. The model is good at that weighing and bad at arithmetic over a corpus. So: measurement is code's job, judgement is the model's job. This also satisfies Constitution Principle II — every number in the explanation traces to a measurement, and the prompt forbids asserting facts about the employer that the signals do not support (FR-019).

**Mechanics**: `llm.CompleteStructured` already implements the parse → validate → retry-with-error loop, and `matching.FitResult.Validate()` is the exact precedent for the range check (`score` 0-100). `GhostJobResult.Validate()` reproduces it and adds `confidence` ∈ [0,1]. A result failing after the retry budget persists nothing (FR-010).

**Model selection**: follow the `cfg.ModelOr` / per-task-model pattern (`apps/api/internal/config/config.go:32`). A `LLM_MODEL_GHOST` value falling back to `LLMModel` keeps the feature tunable without a redeploy and mirrors `LLM_MODEL_MATCH`. Runs on local Ollama — FR-020, Principle V.

**Alternatives considered**: a pure rules engine — rejected, brittle and unable to express confidence honestly. A model that reads the raw job corpus and counts for itself — rejected, non-deterministic measurements would break SC-006 and burn context.

---

## Decision 6: badge only — no auto-hide, no reorder

**Decision**: The score changes what the user *sees on a card*, never what the feed *contains or orders*.

**Rationale**: **User-approved and explicitly chosen over auto-hide.** The failure modes are asymmetric. A wrongly-flagged real job that stays visible with a red badge costs the user two seconds of skepticism. A wrongly-flagged real job that is auto-hidden costs them an opportunity they never learn existed, and they cannot correct an error they cannot see. Given that all four signals are proxies with innocent explanations, silent suppression is not a defensible trade. This is FR-015 and SC-007, and it is what keeps Constitution Principle I trivially satisfied (Decision 7).

**Bands**: yellow 50-79, red 80-100, nothing below 50 — user-approved starting values, expected to be tuned once real scores exist. They are product constants, not calibrated thresholds, and the plan treats them as such.

---

## Decision 7: Constitution Principle I is satisfied by construction

**Decision**: No justification section is needed; the feature has no action path to constrain.

**Rationale**: "No Auto-Apply, Ever" forbids acting on a listing on the user's behalf. This feature reads data the system already holds, writes one score row, and renders a badge. It sends nothing, submits nothing, contacts no employer or board (FR-016), and — per Decision 6 — does not even hide a job. The schema has no column capable of expressing an action taken. Recorded here because "we added an automated scoring system that judges postings" is exactly the shape of change that *should* be checked against Principle I, and the answer happens to be clean.

---

## Decision 8: manual re-score only, no scheduler

**Decision**: Score at ingestion (alongside fit scoring) and on an explicit per-job button press. Nothing on a timer.

**Rationale**: User-approved (spec Story 3). Signals do age, but a scheduled re-score of the whole corpus means a steady LLM load for scores nobody is looking at, on a single-user self-hosted box that also runs embeddings and document generation. The user refreshing the one job they are considering is the same value at a fraction of the cost. Concurrency guard: the button disables while a refresh is in flight (Story 3, scenario 3), so a double-click cannot start two runs against one job — and the `UNIQUE("jobId","kind")` upsert makes a race idempotent regardless.

**Alternatives considered**: a nightly re-score of jobs older than 45 days — rejected as speculative load; revisit as its own feature if manual refresh proves tedious in practice.
