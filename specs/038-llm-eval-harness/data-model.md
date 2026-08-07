# Phase 1 Data Model: Scored Golden-Set Evaluation Harness

**Feature**: `038-llm-eval-harness` | **Date**: 2026-08-07
**Revised**: 2026-08-07 after audit. §3 listed eight scorers, of which two do not exist, one needed a
denominator that is never exported, one was pointed at a file containing no verifier, and one had its
direction inverted. §1's layout assumed a subpackage that cannot reach the pipeline. See research.md's
corrections log.

No database change. No migration, no table, no DTO, no cross-language type. Everything below is a Go
type or a committed file.

---

## 1. Code and on-disk layout

**Corrected**: the harness is **package `application`**, in `eval_*_test.go` files beside
`service.go` — not a subpackage. Every stage of the tailoring path is unexported
(`tailorRendercvResume` `service.go:421`, `selectWithCompleteness` `:326`, `summarize` `:370`,
`fixStructureIntegrity` `:689`, `renderToPageTarget` `:581`, `renderDeps` `:518`), and the two
exported entry points, `Generate` and `GenerateAdHoc`, require Postgres and write run rows. A
subpackage could reach none of it. Test files rather than ordinary files so the harness stays out of
the API binary. Full reasoning in research R10.

```text
apps/api/internal/generation/application/
├── service.go                       # existing — the pipeline the harness drives
├── eval_corpus_test.go              # NEW: case discovery + discipline assertions
├── eval_scorer_test.go              # NEW: the six scorers, ScorerSetVersion, the delegation tripwire
├── eval_baseline_test.go            # NEW: load, compare, refuse on version mismatch, -update flag
├── eval_replay_test.go              # NEW: ReplayProvider + the request-hash tripwire
├── eval_run_test.go                 # NEW: drive one case through the pipeline with stubbed renderDeps
├── eval_test.go                     # NEW: TestEvalCorpus — the gate, ordinary suite, no tag
├── eval_live_test.go                # NEW: //go:build eval_live — live comparison + recorder
├── benchmark_test.go                # REMOVED, rewritten into eval_live_test.go
└── evaldata/
    ├── cases/
    │   ├── baseline/
    │   │   ├── case.yaml            # name, why, grounding level, shape overrides, page_counts
    │   │   ├── master.yaml          # synthetic master profile
    │   │   └── vacancy.txt          # the posting text
    │   ├── absent-skills/
    │   ├── oversized-profile/
    │   ├── nearly-empty-profile/
    │   ├── vague-vacancy/
    │   └── ambiguous-dates/
    ├── replays/
    │   └── <case>/<request-hash>.json
    └── baselines/
        └── <case>.json
```

`evaldata/` contains no `.go` files, so it is not a package; it is addressable by relative path from
both the deterministic and the live entry points, which now live in the same package.

One directory per case, one baseline file per case (research R6): a baseline change shows up in
review as a local diff on the case that moved, and unrelated cases never conflict.

### The baseline-update and record entry points

There is no `cmd/update-baseline/main.go`. A `main` package cannot import test files, and the harness
is test files. Both operations are flags on the test binary:

```bash
# update one case's baseline, deliberately, with a reason
go test ./internal/generation/application/ -run TestEvalCorpus \
  -eval.update-baseline -eval.case absent-skills -eval.reason "tightened skill-token grounding"

# record replay fixtures from a live run — build-tagged, so it cannot happen by accident
go test -tags eval_live ./internal/generation/application/ -run TestEvalRecord \
  -eval.record -eval.case absent-skills
```

Recording sitting behind a build tag and a different `-run` target is a stronger form of C3-6 than a
flag alone: a failing deterministic run has no code path to the recorder, because the recorder is not
compiled into that binary.

---

## 2. `Case`

| Field | Source | Meaning |
|---|---|---|
| `Name` | directory name | stable identifier used in baselines and failures |
| `Why` | `case.yaml` | what failure this case exists to catch — required, not decorative |
| `Master` | `master.yaml` | synthetic `domain.RendercvMaster` |
| `Vacancy` | `vacancy.txt` | posting text |
| `GroundingLevel` | `case.yaml` | which grounding level to run under |
| `ShapeConfig` | `case.yaml` | overrides on `domain.DefaultShapeConfig()` |
| `PageCounts` | `case.yaml` | the sequence the stubbed `countPages` returns, one per render round |

`Why` is a required field on purpose. A corpus case whose reason for existing is not written down
becomes a case nobody dares delete and nobody understands.

`PageCounts` is what makes the page-fit loop deterministic *and* exercisable without a PDF toolchain
(research R11). A case declaring `[3, 2]` forces one condense round then settles; `[2]` skips the loop.
The `expand` and `condense` collaborators keep their production implementations and go through the
`ReplayProvider`, so the LLM half of page fitting is measured — only the PDF half is stubbed.

**Extension rule** (FR-020): adding a case is adding a directory. Cases are discovered by walking
`cases/`, never enumerated in Go.

### The six initial cases (research R7)

| Case | Catches | Provenance |
|---|---|---|
| `baseline` | healthy path — the checks must **not** fire on good output | `internal/generation/testdata/sample_rendercv.yaml` |
| `absent-skills` | fabricated skills in the summary | the failure that motivated 035 |
| `oversized-profile` | silent truncation of selection output | 035's economy-model truncation |
| `nearly-empty-profile` | shortfall check firing on a small profile merely for being small | 035 edge case |
| `vague-vacancy` | the completeness verifier's structural-fallback path | 035 research R7 |
| `ambiguous-dates` | derived experience figure contradicting the summary | 035 FR-008 |

**Corrected — `unbounded-deliberation` is not here.** A model burning its token budget on reasoning is
a provider-side behaviour governed by `reasoning_effort` in `gateway/config.yaml`. A `ReplayProvider`
never contacts a provider, so under replay the case would only ever assert something about a fixture
the harness itself recorded. It is a live-mode case (research R7), and the scorer it needed —
`empty_output` — does not exist (§3).

**Corrected — `sample_rendercv.yaml` is already synthetic.** It is the upstream RenderCV demo
document: Jane Doe, Princeton, `rendercv.com`. The `baseline` case is shaped from it directly. There
is no anonymisation step, because there is no real person in it.

**Corrected — dates.** Every fixture uses **closed date ranges only**; no role may end in `present`
(FR-030). `DeriveTotalExperienceYears` (`rendercv_structure.go:54`) resolves `present` against
`time.Now().Year()`, and production still calls that wrapper at four sites, so an open-ended role
makes the derived figure — and therefore the summary prompt, and therefore the request hash — change on
1 January. `DeriveTotalExperienceYearsAsOf` exists but nothing in the tailoring path takes an injected
year; closing the date ranges achieves the same determinism with no production change.
`ambiguous-dates` remains expressible through overlapping roles, gaps and missing start dates.

Every fixture is synthetic (FR-019, SC-011). The convenient fixture is the developer's own résumé;
committing a real person's employment history and contact details to test a grounding checker is not
an acceptable trade, and the checks are structural so real data buys nothing.

---

## 3. `Score` and the scorer set

### `Score`

| Field | Type | Meaning |
|---|---|---|
| `Name` | `string` | which scorer produced it |
| `Value` | `float64` | the measurement |
| `Direction` | `lower_is_better` / `higher_is_better` | so "regression" is defined per scorer, not guessed |

`Direction` exists so the comparator never has to infer whether 3 → 5 is good.

### The scorers — six, each opened and checked (FR-002, research R9)

Every row names the exact expression the scorer evaluates and the file it lives in. This table
previously listed eight scorers, three of which named nothing that exists.

| Scorer | Expression | Direction | Source |
|---|---|---|---|
| `grounding_violations` | `len(domain.VerifyRendercvGrounding(master, merged, level, analysis))` | lower | `rendercv_grounding.go:123` |
| `structural_violations` | `len(domain.VerifyStructureIntegrity(master, merged))` | lower | `rendercv_structure.go:107` |
| `highlight_drift` | `len(domain.VerifyHighlightGrounding(master, merged))` | lower | `rendercv_structure.go:231` |
| `required_skills_missing` | `len(report.RequiredMissing)` | lower | `rendercv_completeness.go:10` |
| `nice_to_have_retention` | `report.NiceToHaveRetained` | higher | `rendercv_completeness.go:11` |
| `bullet_shortfalls` | `len(report.BulletShortfalls)` | lower | `rendercv_completeness.go:13` |

`report` is one `domain.VerifyCompleteness(master, merged, analysis, cfg)` per run, shared by the last
three so the verifier runs once.

**Four corrections against the previous table**

- **`structural_violations` named the wrong file.** "The structural checks in `rendercv_shape.go`" do
  not exist. That file holds `ApplyHardLimits` (`:143`), which *mutates* a document to fit configured
  limits and returns a report of what it trimmed. It verifies nothing. A scorer wired there would have
  measured trimming and been read as measuring structural integrity. The verifier is
  `VerifyStructureIntegrity`, in `rendercv_structure.go`.
- **`required_skill_retention` cannot be built.** It wanted a ratio. `CompletenessReport` exports
  `RequiredMissing []string` and no denominator; the required-skill token set is computed inside
  `VerifyCompleteness` and never escapes, and `matchesAnySkill` and `orderedSkillTokens` are
  unexported. Recomputing the denominator in the harness means re-implementing production's skill
  matching, which is the parallel definition of quality FR-002 forbids. Replaced by
  **`required_skills_missing`**, a count of an exported field, lower-is-better. Weaker — one missing
  skill out of two scores the same as one out of twenty — and honest.
- **`achievements_per_job_min` had its direction inverted.** It was declared `higher`. Its only
  backing quantity, `CompletenessReport.BulletShortfalls`, is populated *only* for companies that fell
  **below** `cfg.ExperienceBulletsMin` (`rendercv_completeness.go:108-132`); the map value is the
  deficient bullet count, not an achievement count. More entries is worse. Renamed
  **`bullet_shortfalls`**, direction **lower**. As originally written, the gate would have failed on
  improvement and passed on regression.
- **`json_parse_failures` and `empty_output` are cut from v1.** `CompleteStructured`
  (`port.go:299-320`) retries up to three times and discards the attempt count — only the last error
  survives, and nothing exported reports how many attempts were spent. No zero-content check exists
  anywhere in `internal/generation/domain`. Both would have to be written here, which makes the
  harness the author of a quality rule it then grades. They return as scorers once the instrumentation
  lands in production on its own justification (research R9).

**Declared overlap** (FR-027): `grounding_violations` and `highlight_drift` move on the same defect.
`VerifyRendercvGrounding` performs the drift comparison inline at `rendercv_grounding.go:145-150` —
the same `lcsCovered` check against the same per-company master bullets. One ungrounded highlight
raises both. They are kept apart because the *difference* is informative: grounding up with drift flat
means a fabricated company, section or project, a different defect with a different fix. The
comparator MUST report a co-moving pair as one defect seen twice, and MUST NOT sum scores into a
total.

Not one of these six is new logic. That is FR-002's whole point: the harness measures what production
enforces, so a green harness means something about production. A scorer that invented its own rule
would measure the harness — and FR-029 makes that detectable rather than merely forbidden.

**No LLM judge** (FR-003, research R2): every scorer above is deterministic Go.

### `ScorerSetVersion`

A single integer, bumped whenever any scorer's definition changes — including when an underlying
domain check changes behaviour.

### Delegation tripwires (FR-029, research R12)

Two committed tests, both in the ordinary suite:

- **`TestScorerDelegationIsExact`**: for each scorer, independently call the domain function it names
  and assert equality with the scorer's output, over every corpus result and a set of mutated
  documents. A scorer that adds a threshold, a filter or a cap breaks the equality.
- **`TestScorersDetectInjectedDefects`**: inject a known defect into a copy of a scored result — a
  company absent from master, a highlight sharing no words with any master bullet, highlights stripped
  below `ExperienceBulletsMin`, a required skill removed — and assert the relevant scorer moves in its
  worse direction. Catches a scorer that returns a constant, is wired to the wrong function, or is
  never called.

---

## 4. `Baseline`

```json
{
  "case": "absent-skills",
  "scorer_set_version": 3,
  "recorded_at": "2026-08-07",
  "reason": "initial baseline",
  "scores": {
    "grounding_violations": 0,
    "structural_violations": 0,
    "highlight_drift": 0,
    "required_skills_missing": 0,
    "nice_to_have_retention": 0.86,
    "bullet_shortfalls": 0
  }
}
```

| Field | Purpose |
|---|---|
| `case` | which case |
| `scorer_set_version` | the instrument this was measured with (FR-009) |
| `recorded_at` | when |
| `reason` | why it was recorded or changed — required on every update (FR-007) |
| `scores` | scorer name → value |

**Comparison rules**

- Baseline version ≠ current version → **refuse to compare**, report the mismatch. Never a delta
  across instruments (FR-009, research R5).
- A score worse than baseline in its `Direction` → **fail**, naming case, scorer, baseline and actual
  (FR-006).
- A score better than baseline → **report**, and fail with a "baseline needs updating" message
  (FR-007). Silent acceptance of improvement is how a baseline quietly stops describing reality.
- A case with no baseline file → **unbaselined**, reported, **not a pass** (FR-011).
- Two scorers moving on the same defect → reported as **one defect seen by two instruments**, never as
  two independent regressions, and never summed (FR-027, §3's declared overlap).
- Baselines are never auto-written by a passing run. Updating is an explicit `-eval.update-baseline`
  invocation with a reason, producing a reviewable diff (§1).

---

## 5. `ReplayFixture`

```json
{
  "request_hash": "sha256:...",
  "request_summary": {
    "model_key": "generation-select",
    "prompt_prefix": "You are tailoring a resume…",
    "prompt_len": 4812,
    "system_len": 320,
    "temperature": 0.1,
    "max_tokens": 8000,
    "response_mode": "strict",
    "schema_len": 1904
  },
  "response": { "content": "...", "usage": { "cost": 0, "prompt_tokens": 0, "completion_tokens": 0 } },
  "recorded_at": "2026-08-07",
  "recorded_from": "openrouter/google/gemini-2.5-flash-lite"
}
```

**Key** (research R4, corrected): `request_hash` covers **model key, prompt, `opts.System`,
`opts.Temperature`, `opts.MaxTokens`, `opts.ResponseMode` and `opts.JSONSchema`**. There is no message
list — the interface is `CompleteJSON(ctx, prompt string, opts *CompleteOptions)`
(`internal/platform/llm/domain/port.go:152`). The original key omitted temperature and token caps,
which meant raising `selectMaxTokens` would have invalidated no fixture at all.

- A request with **no** matching fixture → **fail loudly** (FR-010). Never fall through to a live
  call, never return a default, never pick the nearest fixture.
- `request_summary` is human-readable context for the failure message. It is not part of the key.
- Fixtures are recorded from a live run under `-tags eval_live` and committed. Refreshing is
  deliberate, in the same change that changed the prompt. Fixtures are never hand-edited (FR-024).

**Volume** (research R4b, FR-024): one case is not one fixture. `CompleteStructured` appends
`"Your previous answer was invalid: " + lastErr` on retry (`port.go:302`), so attempts 2 and 3 are
distinct requests with distinct hashes; the grounding retry ladder, the completeness escalation ladder
and the structure-fix ladder each add more. A single case can issue ten or more. The corpus therefore
stays at six cases, the fixture count carries an asserted bound, and recording is scripted.

This is the design's second sharp edge. A replay harness that tolerates a stale fixture reports green
for a pipeline that has changed underneath it — worse than no harness, because it manufactures
confidence. Keying on the request makes a prompt edit *break* the harness, which is the correct and
intended outcome.

### `ReplayProvider`

Implements `llm.Provider` — the alias in `internal/platform/llm` for
`internal/platform/llm/domain.Provider`, which today is
`ModelName() string`, `Complete`, `CompleteJSON` and `Embed` (`port.go:149-154`). Note this is a
*different* `domain` package from `internal/generation/domain`, which is where `RendercvMaster` and
the verifiers live; inside package `application` the existing import convention already keeps them
apart — `domain` is the generation domain, `llm` is the provider facade.

Injection point: `NewService` (`service.go:93`) takes `GenerationRouters` (`service.go:71-79`), five
`llm.Provider` fields — `Analyze`, `Select`, `Premium`, `Summary`, `Cover`. The harness constructs a
`Service` with all five set to the same `ReplayProvider`. Every stage helper also takes an
`lc llm.Provider` parameter directly, so no production change is needed to substitute the provider.

- Zero external calls, zero credentials (FR-004, SC-002).
- `Embed` returns a fixed vector; the tailoring path does not use embeddings.
- It must implement whatever the interface is at the time. Feature 037 has **not** landed, so there is
  no `CompleteChat` today; if it lands, this is a compile error, which is the correct way to find out.

### Renderer stubbing (FR-028, research R11)

`renderToPageTarget` takes a `renderDeps` (`service.go:518`) whose own comment says it is "injected so
the page-target loop can be tested without a Typst toolchain or a live LLM". The harness supplies:

| Dep | Deterministic mode |
|---|---|
| `render` | returns a fixed path; produces no PDF, invokes no binary |
| `countPages` | returns the next value from the case's `page_counts` |
| `expand` | **production** `expandContent`, through the `ReplayProvider` |
| `condense` | **production** `condenseContent`, through the `ReplayProvider` |

**Corrected**: the previous C3-5 required page fitting to "run as in production", which contradicted
C4-4's no-network requirement and quickstart's `go build`-only prerequisite — production page fitting
shells out to the `rendercv` binary (`internal/generation/infrastructure/rendercv_renderer.go:58`), a
Python + Typst toolchain. C3-5 is now narrowed to "everything except the PDF render". The cost is
stated in the spec's Assumptions: the deterministic gate does not measure renderer correctness.

---

## 6. `ComparisonRun` (live mode)

| Field | Meaning |
|---|---|
| `Models` | candidate task keys compared |
| `Cases` | which cases ran — including the live-only `unbounded-deliberation` case |
| `Results` | per model, per case: scores, cost, latency **per stage** |
| `Provenance` | per model, per case, **per stage**: `ServedModel`, `ServedGroup`, `Substituted`, `Escalated` (FR-023) |
| `RequestParams` | per stage: temperature, max tokens, response mode; per case: grounding level and shape config |
| `CorpusRevision` | the git revision of `evaldata/` the run used |
| `Incomplete` | per model: true when a provider outage cut the run short (FR-017) |
| `StartedAt` / `Duration` | when and how long |

**`Provenance`, `RequestParams` and `CorpusRevision` are what make SC-008 true rather than claimed.**
The previous C6-7 required only "a durable, diffable artifact". A second person cannot reproduce a
comparison from scores and cost alone: the task key does not determine the served model — the proxy
picks a tier, may substitute and may escalate — and the temperature, token cap and date all move the
result. `StageOutcome` (`stage_outcome.go:24-35`) already captures `ServedModel`, `ServedGroup`,
`Substituted` and `Escalated` on every stage; the artifact simply has to write them down, and now must.

Written to a durable, diffable artifact (FR-015, SC-008) so a later reader can reproduce the decision
— unlike 035's model choice, which survives only as a comment in `gateway/config.yaml`.

**Aggregate statistics** (FR-022): a field named `median` holds a median. The existing benchmark
accumulates durations into `medianMs` (`benchmark_test.go:83`), divides by the attempt count (`:86`)
and prints the result under a `Median ms` header (`:96`). That is a mean — and a mean over three
attempts is precisely the statistic one slow outlier distorts, which is the case a latency measure
most needs to catch.

**Every reported figure is computed** (FR-026, SC-014): the existing benchmark declares a
`structuralViolations` field, prints it in its table, and never increments it anywhere in the file. It
has been reporting 0 structural violations for every model since it was written. A column of zeros
reads as evidence of health, which makes an unassigned field worse than a missing one. No field may
appear in an output table unless something writes to it.

**Partial results** (FR-016, FR-017): a model that fails structurally is *scored as failing* and the
comparison continues. A model whose provider went down is marked `Incomplete` and must never be
presented as a complete result.

---

## 7. What is deliberately not modelled

- No expected output text. The golden is the score (research R1).
- No LLM-judged quality score.
- No database, no external evaluation service — the gate's source of truth stays inside the repository
  it gates.
- No per-user or per-tenant dimension; the platform is single-user.
- No scoring for matching, ghost-job detection or salary inference yet. Out of scope, but `Case` and
  `Score` are not shaped to exclude them: a future case would carry a different input and a different
  scorer set.
