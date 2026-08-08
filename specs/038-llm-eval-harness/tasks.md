---

description: "Task list for the scored golden-set evaluation harness"
---

# Tasks: Scored Golden-Set Evaluation Harness

**Input**: Design documents from `/specs/038-llm-eval-harness/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/contracts.md, quickstart.md

**Revised**: 2026-08-07 after audit, and **renumbered**. The previous list put every file in a
subpackage that cannot reach the pipeline, asked for eight scorers of which three do not exist,
required migrating a directory that was never in the repository, and made the gate's only proof three
manual `$EDITOR` edits that revert themselves. Old T005, T037 and T052 (the `resume_test/` migration
chain) are **deleted, not renumbered** — there is nothing to migrate. Old T023/T024/T025 are replaced
by committed tests. See research.md's corrections log.

**Tests**: This feature *is* tests, which makes the usual framing inverted — the deliverable is a gate,
so the tasks that matter most are the ones proving the gate can fail. Those are now **T008, T014, T015
and T018**: four committed tripwires that fail if their mechanism is removed. A gate proved by an edit
that was reverted before the session ended is a gate proved to nobody.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to
- Exact file paths included in every task

## Path Conventions

Go API at `apps/api/`. The harness is **package `application`**, in `eval_*_test.go` files at
`apps/api/internal/generation/application/`; fixtures at
`apps/api/internal/generation/application/evaldata/`. There is no `eval/` subpackage — the entire
tailoring path is unexported and a subpackage could not call it (research R10). No dashboard, no
`packages/shared`, no migration. Paths are repository-relative.

---

## Phase 1: Setup

**Purpose**: The corpus and its discipline, before any scoring machinery exists to run against it.

- [X] T001 Create the corpus layout `evaldata/cases/`, `evaldata/replays/`, `evaldata/baselines/` under `apps/api/internal/generation/application/`, per data-model §1. **evaldata/{cases,replays,baselines} under internal/generation/application/**
- [X] T002 Author the `baseline` case in `apps/api/internal/generation/application/evaldata/cases/baseline/` — `master.yaml`, `vacancy.txt`, and `case.yaml` with a non-empty `why`, a grounding level, shape overrides and a `page_counts` sequence. Copy and shape `internal/generation/testdata/sample_rendercv.yaml`, which **is already synthetic** — it is the upstream RenderCV demo (Jane Doe, Princeton, rendercv.com). There is no anonymisation step because there is no real person in it. Use **closed date ranges only**, no `present` (FR-019, FR-030). **Shaped from the RenderCV demo document with every `present` closed to a fixed month, plus a `summary` section — without one the grounding check correctly refuses the tailored document for adding a section the master does not declare**
- [X] T003 Implement case discovery by walking `evaldata/cases/`, with no case name enumerated in any Go file, in `apps/api/internal/generation/application/eval_corpus_test.go` (FR-020, contracts C5-1). **discoverCases walks the directory; no case name appears in any Go file**
- [X] T004 [P] Add `TestCorpusDiscipline` asserting every case has `case.yaml`, `master.yaml`, `vacancy.txt`, a non-empty `why` and a `page_counts` sequence; that no fixture contains a real-identity marker; and that **no fixture contains an open-ended date** — in `apps/api/internal/generation/application/eval_corpus_test.go` (contracts C5-2/C5-3/C5-5/C5-7, FR-030, SC-011). **TestCorpusDiscipline: required files, non-empty `why`, page_counts, synthetic-identity patterns, and the open-ended-date check as a value match rather than a substring (so the word 'presentation' does not trip it)**

**Checkpoint**: One case exists, discovery works, and the discipline test fails if a case is malformed, non-synthetic, or dated open-endedly.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The pipeline seam, replay, scoring and baselines.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

### The seam (new — no task previously existed for this, and nothing else compiles without it)

- [X] T005 Build the harness's pipeline entry point in `apps/api/internal/generation/application/eval_run_test.go`: construct a `Service` via `NewService` with all five `GenerationRouters` fields (`Analyze`, `Select`, `Premium`, `Summary`, `Cover`, `service.go:71-79`) set to the `ReplayProvider`, and a `renderDeps` (`service.go:518`) whose `render` returns a fixed path and whose `countPages` returns the case's `page_counts` in order, with `expand`/`condense` left as the production `expandContent`/`condenseContent`. Drive `tailorRendercvResume` (`service.go:421`) then `renderToPageTarget` (`service.go:581`). This is why the harness is in package `application`: every one of those is unexported (research R10, FR-028, contracts C3-5). **eval_run_test.go: NewService with all five routers on ReplayProviders, stubbed render/countPages with production expand/condense, driving tailorRendercvResume then renderToPageTarget. renderOutcome carries no document — page fitting mutates the merged map in place — so the scorers see what would have been rendered**

### Replay

- [X] T006 Implement `ReplayProvider` as a full `llm.Provider` — `ModelName`, `Complete`, `CompleteJSON`, `Embed` (`internal/platform/llm/domain/port.go:149-154`) — keyed by a hash over **model key, prompt, `opts.System`, `opts.Temperature`, `opts.MaxTokens`, `opts.ResponseMode` and `opts.JSONSchema`**, in `apps/api/internal/generation/application/eval_replay_test.go` (contracts C3-1, research R4). There is no message list; do not hash one. **eval_replay_test.go. **Deviation recorded in the file:** 037 landed first, so the request genuinely IS a message list now and the hash covers it. C3-1's instruction 'do not hash one' was written before CompleteStructured routed through CompleteChat; its intent — every field that can change the response is in the key — is preserved exactly**
- [X] T007 Implement the no-fixture-match path as a loud failure naming the case and the request summary — never a live call, never a default, never the nearest fixture — in `apps/api/internal/generation/application/eval_replay_test.go` (FR-010, contracts C3-2). **A miss returns no content, records itself, and names the case, stage, request shape and the re-record command**
- [X] T008 **Tripwire**: `TestReplayHashCoversEveryRequestField` — perturb each hashed field of a committed fixture in turn and assert **every** perturbation misses, in `apps/api/internal/generation/application/eval_replay_test.go` (FR-029, contracts C3-1a, research R12). This is the mechanical replacement for old T024, and it is strictly stronger: it would have caught the original key, which omitted temperature and token caps. **TestReplayHashCoversEveryRequestField perturbs eleven fields including temperature and max tokens — the two the original key omitted — and asserts every perturbation misses, plus the converse that an identical request still matches**
- [X] T009 [P] Implement fixture recording behind `//go:build eval_live` as a `-run TestEvalRecord` target with `-eval.record`, in `apps/api/internal/generation/application/eval_live_test.go`, so the recorder is not compiled into the binary that runs the gate (contracts C3-6, C3-7). **eval_live_test.go behind //go:build eval_live, as -run TestEvalRecord with -eval.record**

### Scoring

- [X] T010 Implement the `Scorer` interface with `Name`, `Direction`, and a `Score` taking `master`, `result`, `analysis`, `cfg`, `level` and `runErr` — `cfg` and `level` are required because `VerifyRendercvGrounding` and `VerifyCompleteness` take them — in `apps/api/internal/generation/application/eval_scorer_test.go` (contracts C1-4/C1-5). **scoreInput carries master, result, analysis, cfg, level and runErr, with one shared CompletenessReport per run**
- [X] T011 Implement the **six** scorers in `apps/api/internal/generation/application/eval_scorer_test.go`, each evaluating exactly the expression in data-model §3 (FR-002, contracts C1-1):. **All six, each delegating to the named domain function. structural_violations wired to VerifyStructureIntegrity (rendercv_structure.go), required_skills_missing as a count, bullet_shortfalls lower-is-better**
  `grounding_violations` = `len(domain.VerifyRendercvGrounding(...))` (lower);
  `structural_violations` = `len(domain.VerifyStructureIntegrity(...))` — in `rendercv_structure.go:107`, **not** `rendercv_shape.go`, which contains only the `ApplyHardLimits` mutator (lower);
  `highlight_drift` = `len(domain.VerifyHighlightGrounding(...))` (lower);
  `required_skills_missing` = `len(report.RequiredMissing)` (**lower** — the retention *ratio* cannot be built, its denominator never leaves `VerifyCompleteness`);
  `nice_to_have_retention` = `report.NiceToHaveRetained` (higher);
  `bullet_shortfalls` = `len(report.BulletShortfalls)` (**lower** — this was declared higher-is-better over a count of violations, which would have failed the gate on improvement).
  Compute `domain.VerifyCompleteness` once per run and share it. Do **not** implement `json_parse_failures` or `empty_output`: neither has anything to delegate to (research R9, deferred below)
- [X] T012 [P] Add `ScorerSetVersion` and a test asserting it is bumped whenever the scorer set changes, in `apps/api/internal/generation/application/eval_scorer_test.go` (contracts C1-6). **TestScorerSetVersionMatchesTheSet fails if the set changes without the version**
- [X] T013 [P] Add a determinism test running every scorer twice on the same input and asserting identical values, in `apps/api/internal/generation/application/eval_scorer_test.go` (contracts C1-3). **TestScorersAreDeterministic, five repeats**
- [X] T014 **Tripwire**: `TestScorerDelegationIsExact` — for each scorer, independently call the domain function it names and assert **equality** with the scorer's output, over every corpus result and a set of mutated documents, in `apps/api/internal/generation/application/eval_scorer_test.go` (FR-029, contracts C1-1a). Without this, C1-1 is a rule with no detector — which is how `structural_violations` came to be documented as wrapping a file with no verifier in it. **TestScorerDelegationIsExact calls each domain function independently and asserts equality over the healthy document and five mutated ones**
- [X] T015 **Tripwire**: `TestScorersDetectInjectedDefects` — inject a company absent from master, a highlight sharing no words with any master bullet, highlights stripped below `ExperienceBulletsMin`, and a removed required skill; assert the relevant scorer moves in its worse direction, in `apps/api/internal/generation/application/eval_scorer_test.go` (FR-029, research R12). Mechanical replacement for old T023; touches no production code. **TestScorersDetectInjectedDefects over five injected defects; each asserts the relevant scorer moves in its worse direction**

### Baselines

- [X] T016 Implement `Baseline` load/save with `case`, `scorer_set_version`, `recorded_at`, `reason` and `scores`, one JSON file per case, in `apps/api/internal/generation/application/eval_baseline_test.go` (FR-008, contracts C2-7). **eval_baseline_test.go, one JSON file per case**
- [X] T017 Implement `Compare` with all five outcomes — version mismatch refuses, worse fails, better fails with "needs updating", missing baseline reports unbaselined, and a **declared overlapping pair reports one defect seen twice rather than two regressions, never summed** — in `apps/api/internal/generation/application/eval_baseline_test.go` (FR-006/FR-007/FR-009/FR-011/FR-027, contracts C2-1 to C2-4, C2-8). The overlap is real: `VerifyRendercvGrounding` performs the drift comparison inline at `rendercv_grounding.go:145-150`, so one ungrounded highlight moves both `grounding_violations` and `highlight_drift`. **compareToBaseline returns all five outcomes, and reports a declared overlapping pair as one defect seen twice with an explicit 'do not add them together'**
- [X] T018 **Tripwire**: `TestVersionMismatchRefuses` — construct a baseline at `ScorerSetVersion-1` and assert a refusal containing no delta, in `apps/api/internal/generation/application/eval_baseline_test.go` (FR-009, contracts C2-1). Mechanical replacement for old T025. **TestVersionMismatchRefuses, asserting the refusal names no scorer and carries no measured value**

**Checkpoint**: A case can be run against replayed responses with the renderer stubbed, scored by production's own checks, and compared to a baseline — and four committed tests fail if any of that stops being true.

---

## Phase 3: User Story 1 - A change that would make resumes worse is caught before merge (Priority: P1) 🎯 MVP

**Goal**: Deterministic mode runs in the ordinary suite, costs nothing, needs no toolchain, and fails
the build on regression.

**Independent Test**: The four tripwires pass, and each fails when its mechanism is removed — with no
provider involved.

### Tests for User Story 1

- [X] T019 [P] [US1] Test that deterministic mode runs with **no** build tag and **no** environment variable, in `apps/api/internal/generation/application/eval_test.go` (FR-005, contracts C4-1). **TestGateRunsWithNoTagAndNoEnvVar reads its own source and fails if a build constraint or an opt-in env var appears**
- [X] T020 [P] [US1] Test that the harness passes with every provider credential unset and no network available, in `apps/api/internal/generation/application/eval_test.go` (FR-004, SC-002, contracts C4-4). **TestGateRunsWithNoCredentials unsets all eight provider variables and runs a case**
- [X] T021 [P] [US1] Test that the harness passes with the `rendercv` binary absent from `PATH` and never invokes it, Python or Typst, in `apps/api/internal/generation/application/eval_test.go` (FR-028, SC-002, contracts C4-4a). This is the risk nothing previously measured: the 60s budget was never in danger — a render is ~800ms — but a CI runner without the Python+Typst toolchain could not have run the gate at all. **TestGateRunsWithNoRenderToolchain empties PATH, asserts rendercv/python/typst are unresolvable, and runs a case**
- [X] T022 [P] [US1] Test that a failure message carries case, scorer, baseline value, actual value and direction, in `apps/api/internal/generation/application/eval_baseline_test.go` (FR-006, contracts C4-5). **TestWorseScoreFailsWithAFullMessage checks case, scorer, baseline, actual and direction all appear**
- [X] T023 [P] [US1] Test that a passing run writes no baseline file, in `apps/api/internal/generation/application/eval_baseline_test.go` (contracts C2-5). **TestPassingRunWritesNoBaseline**
- [X] T024 [P] [US1] Test that a baseline update without a `reason` is rejected, in `apps/api/internal/generation/application/eval_baseline_test.go` (contracts C2-6). **TestBaselineWriteRejectsAMissingReason — rejected at the write, not only at the flag, so no code path can produce a reasonless baseline**

### Implementation for User Story 1

- [X] T025 [US1] Implement `TestEvalCorpus` — run every case, compare against baselines, fail on regression, refusal or unbaselined case — in `apps/api/internal/generation/application/eval_test.go` (FR-005, contracts C4-2). **TestEvalCorpus, running cases in parallel with the 60s budget asserted in cleanup**
- [X] T026 [US1] Record replay fixtures and the initial baseline for the `baseline` case with `reason: "initial baseline"`, using the recorder from T009 (FR-008, FR-024). Fixtures are recorder-produced; hand-editing one is hand-authoring an expectation (contracts C3-7). **Recorded live and baselined: 4 fixtures, all six scorers clean — which is what the healthy-path case should produce**
- [X] T027 [US1] Implement the deliberate baseline update as `-eval.update-baseline -eval.case <name> -eval.reason "<why>"` on the test binary, producing a reviewable one-case diff, in `apps/api/internal/generation/application/eval_baseline_test.go` (FR-007, contracts C2-5). **Not** a `cmd/update-baseline/main.go` — `main` cannot import test files, and the harness is test files (research R10). **Flags on the test binary, as specified; used to record all six initial baselines. A one-case diff by construction — updating without -eval.case is refused**
- [X] T028 [US1] Declare the replay-fixture count bound and assert it in `TestCorpusDiscipline`, in `apps/api/internal/generation/application/eval_corpus_test.go` (FR-024, contracts C5-8). Set the bound from the observed count after T026 plus headroom, and treat a breach as a signal to shrink the corpus, not to raise the bound — one case can issue ten or more requests once the retry ladders engage (research R4b). **Bound set at 200 against an observed 29 across six cases, with the comment stating a breach means shrinking the corpus rather than raising the bound**
- [X] T029 [US1] Confirm the whole deterministic run completes in under 60 seconds; if not, parallelise cases rather than raising the budget — a slow gate is a skipped gate (SC-002, contracts C4-3). **The whole deterministic corpus runs in ~0.03s, three orders of magnitude inside the 60s budget. The budget is asserted rather than assumed, so a future case that makes it slow fails rather than being tolerated**

**Checkpoint**: Every change now runs the corpus for free, on any machine, and a quality regression fails the build. This alone is the feature's whole value.

---

## Phase 4: User Story 3 - The corpus covers what actually goes wrong (Priority: P2)

**Goal**: Five more cases, each traceable to a failure this repository actually recorded.

**Independent Test**: Each case fails when run against the code as it was when its failure occurred.

*Sequenced before US2 because a live model comparison over a one-case corpus measures one thing.*

### Tests for User Story 3

- [X] T030 [P] [US3] Test that every case's `why` field names a concrete failure mode, not a generic description, in `apps/api/internal/generation/application/eval_corpus_test.go` (contracts C5-3). **TestCaseWhyNamesAConcreteFailure requires a substantive `why` naming a failure mode**

### Implementation for User Story 3

All five use **closed date ranges only** (FR-030, C5-7) and declare a `page_counts` sequence (C5-2).

- [X] T031 [P] [US3] Author the `absent-skills` case — a vacancy demanding skills the synthetic candidate lacks — in `evaldata/cases/absent-skills/` (the failure that motivated 035). **absent-skills — a Rust systems vacancy against an ML profile**
- [X] T032 [P] [US3] Author the `oversized-profile` case — a large master profile that provoked silent selection truncation — in `evaldata/cases/oversized-profile/`. **oversized-profile — the large master with a tight shape and a [3, 2] page sequence forcing a condense round. Its baseline records nice_to_have_retention at 0.5, the only non-clean score in the corpus, which is the truncation signal this case exists to hold**
- [X] T033 [P] [US3] Author the `nearly-empty-profile` case — asserting the shortfall check does **not** fire on a small profile merely for being small — in `evaldata/cases/nearly-empty-profile/`. **nearly-empty-profile — small but complete, asserting no shortfall fires merely for being small**
- [X] T034 [P] [US3] Author the `vague-vacancy` case — no clear requirements, exercising the completeness verifier's structural-fallback path — in `evaldata/cases/vague-vacancy/`. **vague-vacancy — no stated requirements, exercising the structural-fallback path**
- [X] T035 [P] [US3] Author the `ambiguous-dates` case — overlapping roles, gaps and missing start dates making the derived experience figure contestable — in `evaldata/cases/ambiguous-dates/`. Ambiguity must come from those, **not** from an open-ended `present` role, which would make the fixture expire on 1 January (FR-030). **ambiguous-dates — overlapping roles, a gap and a missing start date; ambiguity from those rather than from an open-ended role**
- [X] T036 [US3] Record replay fixtures and baselines for all five new cases, each with a `reason`, and re-check the fixture-count bound from T028. **All five recorded and baselined; 29 fixtures total, inside the T028 bound**
- [X] T037 [US3] For each case, verify it fails against the code as it was when its failure occurred (`git stash`/checkout the relevant commit), proving the case would have caught its own bug (SC-007). Record the commit checked per case in the case's `case.yaml` `why` field, so this stays auditable after the session. ****Reopened and closed differently, 2026-08-08.** Not doable as specified, for a structural reason: replay fixtures are keyed by a hash over the request, and older code builds different prompts, so checking out the commit a failure occurred at yields replay misses rather than scorer movements — the case would fail for the wrong reason, proving nothing. What was built instead is `TestGroundingPassRemovesARecordedModelDefect` in `eval_provenance_test.go`: it drives each case through production's own stages against the committed fixtures, merges **without** the grounding pass, and compares the scores before and after applying it. This proves two things T015 cannot. First, the corpus holds a defect **the model genuinely emitted** — `vague-vacancy` returned an ungrounded skill and a bullet rephrased past the overlap threshold, and the grounding pass removes both — where T015's defects are injected by the test itself. Second, on every other case the grounding pass makes no score worse, which is the false-positive failure the `baseline` case exists to catch. The named case is asserted, not discovered, so a re-record that cleans it up fails loudly instead of leaving the corpus with nothing real in it. Seen to fail with the grounding pass stubbed out, then restored. Notably `absent-skills` is **not** the case that carries the defect: asked for skills the candidate lacks, the model declined to invent them.

**Note**: there is no `unbounded-deliberation` case here. A thinking model spending its whole token budget and returning nothing is provider-side, governed by `reasoning_effort` in `gateway/config.yaml`; a `ReplayProvider` never contacts a provider, so under replay the case would assert something about a fixture the harness itself wrote. It is a live-mode case — T045 (research R7, contracts C5-9).

**Checkpoint**: The corpus measures the failures that have actually happened, not imagined ones.

---

## Phase 5: User Story 2 - Model choices are made on measurement, not on one run (Priority: P1, sequenced third)

**Goal**: A reproducible live comparison across candidates over the whole corpus.

**Independent Test**: Compare two candidates; a second person reproduces the decision from the recorded artifact alone.

*P1 by value, sequenced third by dependency: it is worth little over a one-case corpus, and it cannot gate anything.*

### Tests for User Story 2

- [X] T038 [P] [US2] Test that live mode is excluded by its **build constraint**, not only by an environment variable, and never appears in `go test ./...`, in `apps/api/internal/generation/application/eval_live_test.go` (FR-012, FR-025, contracts C6-1). Assert the file's first line is `//go:build eval_live`. The file this replaces claimed `-tags benchmark` in its doc comment and had no build line at all. **TestLiveModeIsBuildConstrained asserts the first line is the build tag — the file it replaces claimed a tag in a doc comment and had none**
- [X] T039 [P] [US2] Test that a structurally failing model is scored as failing and the comparison continues, in `apps/api/internal/generation/application/eval_live_test.go` (FR-016, contracts C6-5). **TestIncompleteRunIsNotFoldedIntoMedians**
- [X] T040 [P] [US2] Test that a provider outage marks a model's run `Incomplete` and that a partial result is never presented as complete, in `apps/api/internal/generation/application/eval_live_test.go` (FR-017, contracts C6-6). **Incomplete is set with the failure text and excluded from every median**
- [X] T041 [P] [US2] Test that live mode writes no baseline under any circumstance, in `apps/api/internal/generation/application/eval_live_test.go` (contracts C6-8). **TestLiveModeWritesNoBaseline scans the file's own source above the guardrails section**
- [X] T042 [P] [US2] Test that the median statistic is computed as a median, with a fixture whose mean and median differ, in `apps/api/internal/generation/application/eval_live_test.go` (FR-022, contracts C6-4). **TestMedianIsAMedian with a fixture whose mean is 20 and median is 1**
- [X] T043 [P] [US2] Test that **every column in the reported table is assigned somewhere** — no declared-and-printed-but-never-written field, in `apps/api/internal/generation/application/eval_live_test.go` (FR-026, SC-014, contracts C6-4a). `benchmark_test.go`'s `structuralViolations` has printed 0 for every model since it was written because nothing increments it. **TestEveryReportedColumnIsAssigned checks every printed column and every stage-provenance field has an assignment**
- [X] T044 [P] [US2] Test that the artifact records, per stage, `ServedModel`, `ServedGroup`, `Substituted` and `Escalated` — all four already on `StageOutcome` (`stage_outcome.go:24-35`) — plus temperature, token cap, response mode, grounding level, shape config, corpus revision and run date, in `apps/api/internal/generation/application/eval_live_test.go` (FR-023, SC-008, contracts C6-7). Without these SC-008 is unverifiable: a task key does not determine which model answered. **StageRecord carries ServedModel, ServedGroup, Substituted and Escalated from StageOutcome, plus duration and cost; the artifact carries the request params, corpus revision and run date**

### Implementation for User Story 2

- [X] T045 [US2] Author the live-only `unbounded-deliberation` case and run it in live mode only, in `evaldata/cases-live/unbounded-deliberation/` — a thinking model consuming its budget and returning nothing, the 033/035 precondition bug (contracts C6-9, C5-9). Discovery must exclude it from the deterministic corpus. **evaldata/cases-live/unbounded-deliberation/, outside the directory the gate walks — its case.yaml says why a replay provider makes the case meaningless**
- [X] T046 [US2] Implement `TestLiveComparison` behind `//go:build eval_live` plus an `EVAL_LIVE` opt-in, accepting several candidate task keys via `-eval.models` and running every case against each, in `apps/api/internal/generation/application/eval_live_test.go` (FR-013). **TestLiveComparison behind the build tag plus EVAL_LIVE=1, taking -eval.models**
- [X] T047 [US2] Implement the `ComparisonRun` artifact per data-model §6 — models, cases, per-model/per-case/per-stage scores, cost, latency, provenance, request params, corpus revision, `Incomplete` flags, timings — written to a durable diffable file (FR-015, FR-023, contracts C6-7). **ComparisonRun written as JSON, with corpus revision derived from case content rather than a git sha so an uncommitted fixture edit changes it**
- [X] T048 [US2] Capture cost and latency per model, per stage and per input from the routing service's reported cost that 035 already captures on `StageOutcome` (FR-014, SC-009). **Cost and latency read per stage from runProvenance, which is what 035 already populates**
- [X] T049 [US2] Port the benchmark into live mode, fixing all three of its defects in the process: the mean summed into `medianMs` and divided by attempt count then printed under a `Median ms` header (`benchmark_test.go:83/86/96`, FR-022), the **missing build tag** (FR-025), and the **never-incremented `structuralViolations` column** (FR-026). **Ported with all three defects fixed: a real median, the build tag, and no column that nothing writes**
- [X] T050 [US2] Delete `apps/api/internal/generation/application/benchmark_test.go`, having rewritten it into `eval_live_test.go` rather than leaving it beside (contracts C7-1). **Deleted. 035's TestBenchmarkSplitPipelineTargets was ported into eval_live_test.go first rather than being lost with the file — it belongs in live mode and gains the build tag it should have had**
- [X] T051 [US2] Run quickstart step 8 against two real candidates and confirm the artifact is reproducible by a second reader (SC-008). ****Run 2026-08-08.** `generation-summary` versus `generation-summary-fast` over all seven cases (six deterministic plus the live-only `unbounded-deliberation`), artifact at `evaldata/comparison-20260807-221152.json`. Both models completed 7/7 with **identical scores on all six scorers** — nothing ungrounded, nothing structurally broken, full nice-to-have retention. What separated them was cost and latency, and not in the direction the names suggest: the economy model was **7x cheaper** ($0.0035 versus $0.0249 median per case, $0.025 versus $0.173 total) and **3x slower** (57.8s versus 19.1s median), because Cerebras served four requests in ~56s each against sub-second responses for the rest — provider-side queuing, not model speed. Total spend for the run was about $0.20. The artifact is the record; do not copy these figures into a config comment (T055).

**Checkpoint**: A model swap is an evidenced decision with a durable record, not a comment in a YAML file.

---

## Phase 6: User Story 4 - Cost and speed are tracked alongside quality (Priority: P3)

**Goal**: The quality-versus-cost trade is readable from recorded results without re-running.

**Independent Test**: Two runs against differently-priced models show correspondingly different recorded cost and latency.

- [X] T052 [P] [US4] Test that per-input and total cost and latency appear per model and per stage in the artifact, in `apps/api/internal/generation/application/eval_live_test.go` (FR-014). **CaseResult carries per-case cost and duration, StageRecord carries them per stage, ModelResult carries the medians and the total**
- [X] T053 [US4] Add a comparison view over two `ComparisonRun` artifacts making the quality-versus-cost trade readable without re-running, in `apps/api/internal/generation/application/eval_report_test.go` (US4 scenario 2). **eval_report_test.go: compareRuns over two artifacts, refusing across scorer-set versions and across corpus revisions, reporting quality per scorer and never summed**
- [X] T054 [US4] Cross-check the harness's recorded cost against 035's per-stage cost recording, and against feature 036's traces if it has shipped, so the two figures are reconciled rather than independently believed. **Reconciled by construction rather than by comparison: the harness reads cost from the same runProvenance/StageOutcome that 035 records on the document and 036 sends to the collector. There is one figure, captured once, not two to reconcile**

**Checkpoint**: 035's cost and speed targets are verified from measurement rather than asserted.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T055 [P] Point the model-comparison comment block in `gateway/config.yaml` at the harness's recorded artifact instead of carrying an independent record that drifts (contracts C7-3). **gateway/config.yaml's generation block now points at the harness artifact and says why a figure typed into a YAML comment cannot be reproduced**
- [X] T056 [P] Document the harness in `specs/domains/resume-generation.md`: the two modes, what gates and what reports, how to add a case, how to update a baseline, and — stated plainly — **that the deterministic gate does not measure the PDF renderer**, which stays covered by the infrastructure tests and live mode (FR-028, research R11). **specs/domains/resume-generation.md, including the plain statement that the deterministic gate does not measure the PDF renderer and why the two cut scorers stay cut**
- [X] T057 [P] Add the rule that a production failure fixed from now on arrives with a corpus case that would have caught it, to `specs/domains/resume-generation.md` (FR-018, contracts C5-6). **The standing rule closes the same document section**
- [X] T058 Run the full quickstart, steps 1–10, in order. **Steps run: the gate passes offline with no credentials and no toolchain, all four tripwires pass, recording and baselining both work, and the corpus is bounded. The step asking a case to fail against historical code is the T037 exception above**
- [X] T059 Run `make test-lint` and confirm the deterministic harness runs inside the ordinary suite and passes (FR-005, SC-003). **`make test-lint` passes with the harness inside the ordinary suite — Go lint clean, go test ./... green, 228 dashboard tests green, 0 eslint errors. The integration suite also passes**

---

## Deferred — not tasks in this feature

Two measures were specified as scorers and cut because nothing exists to delegate to (research R9,
contracts C1-1b). They are recorded here so they are not silently lost, and deliberately **not**
scheduled here: a harness that ships production instrumentation so it has something to grade has
inverted FR-002's dependency direction and becomes the author of the quality signal it measures.

| Measure | What it needs first |
|---|---|
| `json_parse_failures` | `CompleteStructured` (`internal/platform/llm/domain/port.go:299-320`) must surface its attempt count. Today it retries up to three times and discards it; only the last error survives |
| `empty_output` | A `VerifyNonEmpty(merged) []StructureViolation` in `internal/generation/domain`. No zero-content check exists anywhere |

Each is a small feature against production code with its own justification, tests and review. Once
either lands, adding it here costs a scorer, a `ScorerSetVersion` bump and a re-baseline.

---

## Dependencies

**Phase order**: Setup (T001-T004) → Foundational (T005-T018) → US1 → US3 → US2 → US4 → Polish.

**Story dependencies**:

- **US1** depends on Setup and Foundational. It is the MVP and the only part that gates anything.
- **US3** depends on US1 — cases need somewhere to be scored. Sequenced before US2 because a live
  comparison over one case measures one thing.
- **US2** depends on US3 for a corpus worth comparing over, and on Foundational for the scorers.
- **US4** rides on US2's runs.

**Hard sequencing**:

- **T005 before everything in Phase 2.** Nothing can be scored until a case can be driven through the
  pipeline. No task previously covered this, which is what let the subpackage error survive review.
- **T008, T014, T015 and T018 are the gate's proof, and they are committed tests.** Each must be
  observed failing when its mechanism is removed — and unlike the manual edits they replace, the test
  remains afterwards as the record. A tripwire that has only ever passed since the day it was written
  is worth checking once against a deliberately broken mechanism.
- **T026 after T025.** Baselines recorded before the comparator exists are baselines nobody validated.
- **T028 after T026.** The bound is set from an observed fixture count, not guessed.
- **T049 before T050.** Port the benchmark's three defects out before deleting the file that documents
  them.

**Within Foundational**: T005 blocks everything; T006 blocks T007 and T008; T010 blocks T011–T015;
T011 blocks T014 and T015; T016 blocks T017 and T018.

**Sequencing correction**: old T023 pointed at quickstart step 2, which expects a failure naming
`absent-skills` — a case authored two phases later. The replacement tripwires (T014, T015) run against
whatever cases exist, starting with `baseline`, so they are correctly placed in Phase 2 and have no
forward dependency on the corpus.

## Parallel Execution Examples

**Setup**: T003 then T004; T002 parallel with both.

**Foundational**: T009 parallel with T010/T011 (recording versus scoring); T012 and T013 parallel with
T016/T017 (scorer versus baseline files). T014 and T015 must follow T011.

**US1 tests**: T019–T024 all in parallel — two test files, sequence within each.

**US3**: T031–T035 are five independent fixture directories, fully parallel. T036 must follow all five.

**US2 tests**: T038–T044 share one file; sequence within it, but they are independent of the US2
implementation tasks.

**Polish**: T055, T056, T057 in parallel — two different documents.

## Implementation Strategy

**MVP** = Phase 1 + Phase 2 + US1. One case, replayed, scored by production's own checks, baselined,
running free on every change — on a machine with no credentials and no PDF toolchain — and failing the
build on regression. Even at one case this is more quality assurance than the repository has today.

**Ship boundary** = MVP + US3. A gate over a single case gates a single case; the five failure cases
are what make the gate mean something. All four tripwires — T008, T014, T015, T018 — must be committed
and each must have been checked against a deliberately broken mechanism before this is called done.

**Increment 2** = US2. Live comparison, replacing the assertionless benchmark and giving 035's model
choice a reproducible record.

**Increment 3** = US4 + Polish. Cost/quality reporting and the documentation.
