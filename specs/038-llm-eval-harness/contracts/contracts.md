# Phase 1 Contracts: Scored Golden-Set Evaluation Harness

**Feature**: `038-llm-eval-harness` | **Date**: 2026-08-07
**Revised**: 2026-08-07 after audit. §3 and §7 specified a directory that was never in the repository,
C3-5 contradicted C4-4, and §1's scorer set named checks that do not exist. See research.md's
corrections log.

**Seven** contracts — the header previously said six over seven sections. No HTTP API, no database, no
cross-language boundary.

**Package note**: everything below is in package `application`
(`apps/api/internal/generation/application/`), in `eval_*_test.go` files. Two different packages are
abbreviated `domain` in this repository, and the previous version of this document mixed them, which
would not have compiled. The convention already used in `service.go` is followed here:

| Identifier | Package | Import path |
|---|---|---|
| `domain.RendercvMaster`, `domain.VacancyAnalysis`, `domain.VerifyRendercvGrounding`, `domain.ShapeConfig` | generation domain | `internal/generation/domain` |
| `llm.Provider`, `llm.CompleteOptions` | provider facade (alias for `internal/platform/llm/domain`) | `internal/platform/llm` |

---

## 1. Scorer contract

```go
type Score struct {
    Name      string
    Value     float64
    Direction Direction // LowerIsBetter | HigherIsBetter
}

type Scorer interface {
    Name() string
    Direction() Direction
    Score(master domain.RendercvMaster, result domain.RendercvMaster,
          analysis domain.VacancyAnalysis, cfg domain.ShapeConfig,
          level domain.GroundingLevel, runErr error) Score
}
```

`cfg` and `level` are parameters because the checks require them:
`VerifyRendercvGrounding(master, merged, level, analysis)` and
`VerifyCompleteness(master, merged, analysis, cfg)`. The previous signature omitted both, so three of
the six scorers could not have been implemented against it.

- **C1-1**: Every scorer MUST delegate to an existing check in `internal/generation/domain`. A scorer
  containing its own quality rule is a defect against FR-002 — the harness must measure what
  production enforces.
- **C1-1a**: C1-1 MUST have a detector. `TestScorerDelegationIsExact` independently calls the domain
  function each scorer names and asserts **equality** with the scorer's output, over every corpus
  result and a set of mutated documents (FR-029, research R12). Without it, C1-1 is a rule with no way
  to observe a violation — which is how `structural_violations` came to be documented as wrapping
  `rendercv_shape.go`, a file containing no verifier.
- **C1-1b**: A proposed measure with no existing check behind it MUST NOT be added as a scorer. It is
  a request for production instrumentation, justified and reviewed on its own. `json_parse_failures`
  and `empty_output` were cut on this rule (research R9).
- **C1-7**: Where two scorers can move on the same underlying defect, the overlap MUST be declared in
  data-model §3, the failure report MUST present the co-movement as one defect observed twice, and
  scores MUST NOT be summed into a total (FR-027). `grounding_violations` and `highlight_drift` are
  such a pair: `VerifyRendercvGrounding` runs the drift comparison inline at
  `rendercv_grounding.go:145-150`.
- **C1-2**: No scorer may call a language model (FR-003). Scoring is deterministic Go.
- **C1-3**: A scorer MUST be deterministic: the same inputs MUST produce the same value on every run.
  A scorer that cannot MUST NOT be admitted to the gating set (spec Edge Cases).
- **C1-4**: A scorer MUST declare its `Direction`, so the comparator never infers whether a rise is a
  regression.
- **C1-5**: A scorer MUST produce a value even when the run failed — `runErr` is an input, not an
  early return. A structurally failing model is scored as failing (FR-016), not skipped.
- **C1-6**: Adding, removing or changing the behaviour of any scorer — including a change in an
  underlying domain check — MUST bump `ScorerSetVersion`.

---

## 2. Baseline comparison contract

```go
func Compare(b Baseline, got []Score, version int) Report
```

- **C2-1**: `b.ScorerSetVersion != version` MUST produce a **refusal**, not a delta. A comparison
  across instruments is meaningless (FR-009, SC-005).
- **C2-2**: A score worse than baseline in its declared direction MUST fail, naming the case, the
  scorer, the baseline value and the actual value (FR-006).
- **C2-3**: A score better than baseline MUST be reported **and** MUST fail with a "baseline needs
  updating" result (FR-007). A silently-accepted improvement leaves a baseline that no longer
  describes reality.
- **C2-4**: A case with no baseline MUST be reported as unbaselined and MUST NOT count as a pass
  (FR-011, SC-004).
- **C2-5**: A passing run MUST NOT write baselines. Updating is a separate explicit invocation
  producing a reviewable diff (FR-008).
- **C2-6**: A baseline update MUST require a non-empty `reason` field.
- **C2-7**: Baselines MUST be committed JSON, one file per case (research R6).
- **C2-8**: A regression involving a declared overlapping pair (C1-7) MUST be reported as one defect
  seen by two scorers, with both values shown, and MUST NOT be counted twice (FR-027).

---

## 3. Replay provider contract

```go
type ReplayProvider struct{ /* fixtures keyed by request hash */ }
var _ llm.Provider = (*ReplayProvider)(nil)
```

- **C3-1**: The fixture key MUST be a hash over everything that determines the question asked: **model
  key, prompt, `opts.System`, `opts.Temperature`, `opts.MaxTokens`, `opts.ResponseMode` and
  `opts.JSONSchema`** (research R4, corrected). There is no message list — the interface is
  `CompleteJSON(ctx, prompt string, opts *CompleteOptions)` (`port.go:152`).
- **C3-1a**: C3-1 MUST have a detector. `TestReplayHashCoversEveryRequestField` perturbs each hashed
  field of a committed fixture in turn and asserts every perturbation misses (FR-029, research R12).
  This is what would have caught the original key, which omitted temperature and token caps — under it,
  raising `selectMaxTokens` invalidated no fixture at all.
- **C3-2**: A request with no matching fixture MUST fail with an error naming the case and the
  request summary. It MUST NOT fall through to a live call, return a default, or select the nearest
  fixture (FR-010, SC-006).
- **C3-3**: `ReplayProvider` MUST make zero network calls and MUST require zero credentials (FR-004,
  SC-002).
- **C3-4**: It MUST implement the full `llm.Provider` interface as it exists at the time — today
  `ModelName`, `Complete`, `CompleteJSON`, `Embed` (`internal/platform/llm/domain/port.go:149-154`).
  Feature 037 has **not** landed; there is no `CompleteChat` to implement. If it lands, this becomes a
  compile error, which is the right way to be told.
- **C3-5**: Replay MUST substitute the provider **and the PDF renderer**, and nothing else. Prompts,
  schemas, merge, grounding, completeness, structure fixing and the expand/condense half of page
  fitting MUST run as in production.

  **Corrected**: the previous C3-5 said page fitting must "run as in production", which directly
  contradicted C4-4 (no network) and quickstart's `go build`-only prerequisite. Production page
  fitting shells out to the `rendercv` binary
  (`internal/generation/infrastructure/rendercv_renderer.go:58`), a Python + Typst toolchain. The
  harness stubs `renderDeps.render` and `renderDeps.countPages` through the seam at `service.go:518`,
  which exists for this — its own comment says "injected so the page-target loop can be tested without
  a Typst toolchain or a live LLM". `renderDeps.expand` and `renderDeps.condense` keep their
  production implementations and go through the `ReplayProvider`.

  The consequence is stated rather than hidden: **the deterministic gate does not measure renderer
  correctness.** That stays covered by the infrastructure tests and by live mode.
- **C3-6**: Recording fixtures MUST be an explicit operation, never a side effect of a failing
  deterministic run. It is a `-run TestEvalRecord` target behind `//go:build eval_live`, so the
  recorder is not compiled into the binary that runs the gate — a stronger guarantee than a flag.
- **C3-7**: Fixtures MUST be recorder-produced. A hand-authored or hand-edited fixture is a
  hand-authored expectation, which is the golden-output-text failure R1 rejects (FR-024).

---

## 4. Deterministic-mode contract

- **C4-1**: Deterministic mode MUST run as an ordinary Go test, with no build tag and no environment
  variable required (FR-005, SC-003).
- **C4-2**: It MUST fail the test suite on any regression, refusal, or unbaselined case.
- **C4-3**: It MUST complete in under 60 seconds (SC-002). If the corpus grows past that, cases are
  parallelised — the budget is not raised, because a slow gate is a skipped gate.
- **C4-4**: It MUST NOT read any provider credential, and MUST pass on a machine with no network
  access at all.
- **C4-4a**: It MUST also pass on a machine with **no PDF toolchain**: no `rendercv` binary, no
  Python, no Typst (FR-028). Verified by running the gate with those absent from `PATH`, not assumed.
  The measured render cost — ~800ms, six cases, at most three renders each — was never the binding
  constraint on SC-002; toolchain *availability* on a CI runner or a fresh clone is, and nothing
  previously measured it.
- **C4-5**: Its failure output MUST be actionable without re-running: case name, scorer, baseline,
  actual, and the direction that makes it a regression.
- **C4-6**: Every figure the run prints MUST be computed from a measurement. A field that is declared
  and printed but never assigned MUST NOT appear (FR-026). `benchmark_test.go`'s `structuralViolations`
  column is declared, printed, and never incremented — it has reported 0 for every model since it was
  written.

---

## 5. Corpus-extension contract

- **C5-1**: Cases MUST be discovered by walking `evaldata/cases/`. No Go file may enumerate them
  (FR-020).
- **C5-2**: A case directory MUST contain `case.yaml`, `master.yaml` and `vacancy.txt`. `case.yaml`
  MUST declare `page_counts`, the sequence the stubbed `countPages` returns (C3-5, data-model §2).
- **C5-3**: `case.yaml` MUST carry a non-empty `why` field stating which failure the case exists to
  catch. A case nobody can justify is a case nobody dares delete.
- **C5-4**: Every fixture MUST be synthetic. No real name, contact detail or employment history
  (FR-019, SC-011).
- **C5-5**: A test MUST assert C5-3 and MUST assert that no fixture contains an obvious real-identity
  marker, so FR-019 is enforced rather than requested.
- **C5-6**: A production failure fixed after this feature ships MUST arrive with a corpus case that
  would have caught it (FR-018, US3 scenario 3).
- **C5-7**: No fixture may contain an open-ended date — no role ending in `present` (FR-030). Asserted
  by `TestCorpusDiscipline`. `DeriveTotalExperienceYears` (`rendercv_structure.go:54`) resolves
  `present` against `time.Now().Year()`, and production still calls that wrapper at four sites, so an
  open-ended role changes the derived figure, the summary prompt and therefore the request hash on
  1 January — silently expiring every affected fixture.
- **C5-8**: The committed replay fixture count MUST carry a declared bound, asserted by test (FR-024).
  One case issues many requests: `CompleteStructured` re-prompts on invalid JSON (`port.go:302`), and
  the grounding, completeness-escalation and structure-fix ladders each add distinct requests. Fixture
  volume, not case count, is what makes this corpus expensive to maintain (research R4b).
- **C5-9**: A failure mode that only a real provider can produce MUST NOT be a deterministic case. It
  belongs in live mode, where the behaviour can actually occur. `unbounded-deliberation` was moved on
  this rule (research R7).

---

## 6. Live-mode contract

```go
//go:build eval_live
```

- **C6-1**: Live mode MUST be behind a build tag **and** an explicit environment opt-in, so it can
  never run as part of `go test ./...` (FR-012, FR-025). The build constraint is the load-bearing half:
  `benchmark_test.go` today claims `-tags benchmark` in its doc comment and has **no `//go:build` line
  at all** (line 1 is `package application`), so it compiles into the ordinary suite and skips at
  runtime on an environment variable. A test excluded by an environment variable is still in the
  suite; a test excluded by a build tag is not in the binary.
- **C6-2**: It MUST accept several candidate task keys and score every one across the whole corpus,
  not a single input (FR-013).
- **C6-3**: It MUST record cost and latency per model, per stage and per input (FR-014, SC-009),
  taking cost from the routing service's reported figure that feature 035 already captures.
- **C6-4**: Any statistic labelled a median MUST be a median (FR-022). The existing benchmark sums
  durations into `medianMs` (`benchmark_test.go:83`), divides by the attempt count (`:86`) and prints
  the result under a `Median ms` header (`:96`), which is a mean.
- **C6-4a**: Every column in the reported table MUST be computed (FR-026, C4-6). The existing
  benchmark's `structuralViolations` is declared and printed and never assigned.
- **C6-9**: Live mode MUST carry the `unbounded-deliberation` case — a thinking model spending its
  budget on reasoning and returning nothing — which cannot exist in the deterministic corpus (C5-9).
- **C6-5**: A structurally failing model MUST be scored as failing and the comparison MUST continue
  (FR-016).
- **C6-6**: A provider outage MUST mark that model's run `Incomplete`; a partial result MUST NOT be
  presented as complete (FR-017).
- **C6-7**: Results MUST be written to a durable, diffable artifact reproducible by a later reader
  (FR-015, SC-008). "Reproducible" is not satisfied by scores and cost. The artifact MUST record, for
  every stage of every case of every model: `ServedModel`, `ServedGroup`, `Substituted` and
  `Escalated` — all four already captured on `StageOutcome` (`stage_outcome.go:24-35`) and previously
  never required here — plus the temperature, token cap and response mode requested, the grounding
  level and shape config the case ran under, the corpus revision, and the run date (FR-023).

  Without these, SC-008 is unverifiable: a task key does not determine which model answered. The proxy
  selects a tier, may substitute, and may escalate. Two runs of "the same comparison" a week apart can
  be served by different models and the artifact would not say so.
- **C6-8**: Live mode MUST NOT write baselines. Baselines come from deterministic mode only —
  otherwise a lucky live run becomes the standard.

---

## 7. Replacement contract (FR-021, SC-010)

**Corrected**: C7-2 and C7-3 required migrating "the useful cases in `resume_test/`" before deleting
that directory, and asserting afterwards that its probe scripts and result files were gone.
`resume_test/` was never in this repository. It existed as untracked working files, has been deleted,
is not in git history, and is unrecoverable. Both clauses are removed rather than redirected — there
is nothing to migrate, and no substitute artifact exists. Any task, quickstart step or success
criterion resting on them is removed too.

- **C7-1**: `benchmark_test.go` MUST be rewritten into live mode, not left beside it — carrying its
  three defects with it: the mean labelled a median (C6-4), the missing build tag (C6-1), and the
  always-zero structural column (C6-4a).
- **C7-2**: After this feature, exactly one evaluation apparatus MUST exist in the repository. A test
  SHOULD assert that no live, assertionless benchmark remains — concretely, that `GENERATION_BENCHMARK`
  appears nowhere in `apps/api`.
- **C7-3**: The model-comparison table currently living as a comment in `gateway/config.yaml` MUST
  point at the harness's recorded artifact rather than remaining an independent record that drifts.
