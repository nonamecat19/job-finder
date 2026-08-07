# Phase 0 Research: Scored Golden-Set Evaluation Harness

**Feature**: `038-llm-eval-harness` | **Date**: 2026-08-07
**Revised**: 2026-08-07 after audit. R4, R7 and R8 asserted things about this repository that are not
true, and three of the eight scorers had nothing to wrap. See the corrections log at the end.

Twelve decisions. All resolved; no NEEDS CLARIFICATION remains.

---

## R1 — What is "golden": the score, not the text

**Decision**: A baseline records the **scores** a fixed input produces, not the expected output text.

**Rationale**: Generation output is not reproducible prose. Even at temperature 0.1 with a strict
schema, wording moves between provider versions, and a diff on generated text would fail constantly
for reasons nobody cares about. What must not regress is measurable: grounding violations, structural
violations, dropped content, skill retention against the vacancy.

The platform already computes some of these. **Corrected**: the first version listed "the structural
checks in `rendercv_shape.go`", which do not exist — `rendercv_shape.go` holds `ApplyHardLimits`, a
*mutator*, and no verifier at all. The real inventory, opened and checked, is R9. Four checks are
available as exported functions and two more as exported fields on a report; the rest of the original
scorer list was assumed rather than found.

The harness turns the checks that do exist from things that run once per resume into things that run
over a corpus and are compared against a recorded number.

**Alternatives rejected**:
- *Golden output text*: fails on benign rewording; would be disabled within a week.
- *Embedding-similarity to a reference output*: a threshold nobody can justify, measuring similarity
  to one arbitrary past answer rather than correctness.

---

## R2 — No LLM judge

**Decision**: Every scorer is deterministic Go. No model grades another model's output (FR-003).

**Rationale**: Three reasons, in order of weight. An LLM judge is nondeterministic, so it cannot gate
a build — the thing this feature exists to do. It costs money on every run, so it cannot run on every
change. And it would introduce a *second*, unverified definition of quality alongside the one 033 and
035 already enforce in production, which is precisely the drift FR-002 forbids: the harness must
measure what production enforces, or it measures nothing useful.

**Alternatives rejected**: an LLM judge for the subjective part (summary readability). Genuinely
beyond deterministic scoring — and genuinely out of scope. A summary that is grounded, correctly
sized and consistent with the derived experience figure is what the platform can enforce; "is it
well written" stays a human review.

---

## R3 — Two modes, and only one of them gates

**Decision**: **Deterministic mode** replays recorded responses, runs in `go test ./...`, and fails
the build on regression. **Live mode** calls real providers, is explicitly opted into, and reports
without gating.

**Rationale**: This is the load-bearing split. The existing `benchmark_test.go` is live-only, gated
behind `GENERATION_BENCHMARK=1`, and therefore runs approximately never — it has no assertions to run
anyway. A harness that requires credentials and money cannot gate a change, and a harness that does
not gate a change does not prevent the regression it was built for.

Replay makes the pipeline deterministic end to end: the same fixed input produces the same request,
matches a recorded response, and yields the same scores. That is gateable, free, and fast.

**Alternatives rejected**:
- *Live-only*: what exists today. Costs money per run, flaky on provider availability, cannot gate.
- *Deterministic-only*: cannot answer "which model should serve this stage", which is US2 and the
  reason 035 needed hand-run probes.

---

## R4 — Replay binding: match on the request, fail loudly on a miss

**Decision**: Each replay fixture is keyed by a hash of the request the pipeline sends. A request with
no matching fixture fails the run.

**Hash inputs, corrected**: the first version said "model key, messages, response format, schema".
There is no message list — `Provider.CompleteJSON(ctx, prompt string, opts *CompleteOptions)`. The
inputs are: **model key, prompt, `opts.System`, `opts.Temperature`, `opts.MaxTokens`, `opts.ResponseMode`
and `opts.JSONSchema`**. Temperature and token caps were missing, which meant raising
`selectMaxTokens` would not have invalidated a single fixture — a silent staleness of exactly the kind
this decision exists to prevent.

### R4a — The pipeline was not deterministic, and this had to be fixed first

**Status: fixed, 2026-08-07, before any harness work.**

Replay keying presumes the same input produces the same request twice. It did not. Three sites:

| Site | Defect |
|---|---|
| `rendercv_grounding.go` added-section check | violations appended in Go's randomised **map order** |
| `rendercv_grounding.go` strict project tokens | same, one violation per token |
| `rendercv_structure.go` `DeriveTotalExperienceYears` | resolves "present" against `time.Now().Year()` |

The first two matter because those violation strings are fed back into the **retry prompt**
(`service.go` → `prevViolations` → `buildSelectPrompt`), so a retry on identical input produced
differently-ordered text run to run. The third silently expires every summary fixture on 1 January.

**Landed**: both violation slices are now sorted before they enter the prompt, and
`DeriveTotalExperienceYearsAsOf(master, now int)` makes the year injectable — with
`DeriveTotalExperienceYears` retained as a wrapper so existing callers are unchanged.
`rendercv_determinism_test.go` covers all three, and each test was confirmed to fail against the
unfixed code rather than merely observed passing.

Worth noting this was a live defect independent of the harness: before the fix, a production retry
sent the model a differently-ordered list of complaints on every attempt.

### R4b — Fixture count is larger than one per case

`CompleteStructured` appends `"Your previous answer was invalid: " + lastErr` on each retry
(`port.go:305`), so attempts 2 and 3 are *different requests* needing their own fixtures. Add the
grounding retry ladder, the completeness/escalation ladder and the structure-fix ladder, and one case
can issue ten or more distinct requests.

This is the cost most likely to kill the harness in practice — recording, reviewing and refreshing
that volume by hand. It is not a reason to abandon request-keying, but the corpus must stay small and
the recording step must be scripted, never manual.

**Rationale**: FR-010. The dangerous failure of any replay harness is a stale fixture: someone edits a
prompt, the pipeline now asks a different question, and the harness cheerfully replays the answer to
the old one and reports green. That is worse than no harness, because it manufactures confidence.

Keying on the request makes a prompt change *break* the harness, which is correct: the fixture must be
re-recorded, deliberately, in the same change.

**Alternatives rejected**:
- *Key by case name and call index*: survives prompt edits, which is exactly the bug.
- *Fuzzy matching on prompt similarity*: a threshold nobody can defend, and it fails open.

---

## R5 — Scorer versioning: a definition change invalidates its baselines

**Decision**: The set of scorers carries a version. Baselines record the version that produced them.
A mismatch refuses the comparison rather than reporting a delta.

**Rationale**: FR-009. If a grounding rule is tightened, every baseline recorded under the old rule
becomes a number measured with a different instrument. Comparing across them produces a delta that
looks like a regression or an improvement and means neither. Refusing is the only honest option; the
fix is to re-record baselines in the same change that changed the scorer, with the reason in the
commit.

**Alternatives rejected**:
- *Auto-refresh baselines on scorer change*: makes the gate unfalsifiable — any regression can be
  laundered through a trivial scorer edit.
- *Ignore versioning*: silently meaningless comparisons.

---

## R6 — Baseline storage: committed JSON, one file per case

**Decision**: Baselines are JSON files under the corpus directory, one per case, committed and
reviewed like code.

**Rationale**: FR-008. Diffable is the requirement that matters — a baseline change must show up in
review as "this case's grounding violations went from 0 to 1", which is a conversation, not a
rounding error. One file per case keeps the diff local to the case that moved and avoids merge
conflicts between unrelated cases.

**Alternatives rejected**:
- *A single combined baseline file*: every change conflicts with every other change.
- *A database or external store*: unreviewable, and it puts the gate's source of truth outside the
  repository being gated.

---

## R7 — Corpus: synthetic, and seeded from what has actually broken

**Decision**: Synthetic fixtures only (FR-019), with the initial cases drawn from every failure this
project has recorded.

Initial set — **six cases, corrected down from seven**:

| Case | What it catches | Provenance |
|---|---|---|
| baseline | healthy path; the check must not fire on good output | `internal/generation/testdata/sample_rendercv.yaml` |
| absent-skills | fabricated skills in the summary | the failure that motivated 035 |
| oversized-profile | silent truncation of selection output | 035's economy-model truncation |
| nearly-empty-profile | shortfall check firing on a small profile for being small | 035 edge case |
| vague-vacancy | the completeness verifier's structural fallback path | 035 R7 |
| ambiguous-dates | derived experience figure contradicting the summary | 035 FR-008 |

**Corrected — `unbounded-deliberation` is dropped from the deterministic corpus.** The failure it
describes is a thinking model spending its whole token budget on reasoning and returning nothing. That
is a *provider-side* behaviour, governed by `reasoning_effort` in `gateway/config.yaml`. A
`ReplayProvider` never contacts a provider, so under replay the case degrades into "a fixture whose
recorded content is empty" — which measures the harness's own fixture, not the pipeline. Its natural
scorer, `empty_output`, does not exist either (R9). The case belongs in **live mode**, where a real
model can actually burn its budget, and it is listed there. Reinstating it in the deterministic corpus
would require inventing an empty-output check, which FR-002 forbids.

**Corrected — the `baseline` case's provenance.** The first version had T002 authoring a fixture
"derived from `sample_rendercv.yaml` but synthetic", implying privacy work. `sample_rendercv.yaml`
**is already synthetic**: it is the upstream RenderCV demo document — Jane Doe, Princeton,
`rendercv.com`. There is no real person in it and nothing to anonymise. Copy it and shape it to the
case; do not perform an anonymisation pass that has no subject.

**Corpus date constraint (new, from R4a).** `DeriveTotalExperienceYears` still resolves `present`
against `time.Now().Year()`, and production still calls the wall-clock wrapper at four sites
(`service.go:377`, `rendercv_llm.go:369`, `rendercv_structure.go:109`, `rendercv_grounding.go:320`).
The `AsOf` variant exists but nothing in the tailoring path takes an injected year. So the harness
cannot pin the clock from outside — and rather than thread a clock through four production call sites
for a test harness's benefit, **every corpus fixture MUST use closed date ranges**: no `present`, no
open-ended role. A profile with only closed spans yields the same derived figure forever, whatever
the wall clock says, and `DeriveTotalExperienceYearsAsOf` never has to be reached. This is a corpus
rule, mechanically checkable by the discipline test, and it costs no production change. `ambiguous-
dates` stays expressible — ambiguity comes from overlapping roles, gaps and missing start dates, not
from `present`.

*Rejected alternative*: injecting a clock into `Service` and the four call sites. Correct in the
abstract, but it is production surgery motivated entirely by a test, and R4a already spent one round
of production change on determinism. Revisit if a case ever genuinely needs an open-ended role.

**Rationale**: FR-018. A corpus assembled by imagination measures imagined failures. Every case above
corresponds to something that actually produced bad output in this repository, which is the only
evidence available that a case is worth having.

**Synthetic is non-negotiable** (FR-019, SC-011): the natural fixture is the developer's own résumé,
and committing a real person's employment history and contact details to a repository to test a
grounding checker is not an acceptable trade.

**Alternatives rejected**: anonymised real profiles. Anonymisation of free-text employment history is
unreliable, and there is no need — the checks are structural and do not require real data.

---

## R8 — Replace `benchmark_test.go`, do not accumulate a second apparatus

**Decision**: The harness rewrites `internal/generation/application/benchmark_test.go` into live mode
rather than sitting beside it (FR-021).

**Corrected — `resume_test/` is gone and was never in the repository.** The first version of this
decision built a whole migration story around "the Python probe scripts and `.jsonl` result files
under `resume_test/`", which it required to be mined for cases before deletion. That directory existed
only as untracked working files, has since been deleted, is not in git history, and is unrecoverable.
Nothing can be migrated out of it, and no requirement, contract, task or quickstart step may depend on
its contents. Every reference has been removed rather than rewritten — there is no substitute artifact
and inventing one would be the same error again.

What survives from the original decision is the part that is true: `benchmark_test.go` is the one
existing apparatus, and the model-comparison table living as a comment in `gateway/config.yaml` is a
second, independent record that will drift.

**Three concrete defects in `benchmark_test.go`**, all confirmed by reading it:

| Defect | Evidence | Requirement |
|---|---|---|
| Mean reported as a median | `:83` `r.medianMs += dur`; `:86` `r.medianMs /= int64(r.attempts)`; `:96` prints it under a `Median ms` header | FR-022 |
| **No build tag** | Line 1 is `package application` with no `//go:build` constraint, yet its doc comment instructs `-tags benchmark`. It compiles into the ordinary suite and skips at *runtime* on `testing.Short()` and `GENERATION_BENCHMARK` | FR-025 |
| **A column that is always zero** | `structuralViolations` is declared in the `result` struct and printed in the table, and is never incremented anywhere in the file | FR-026 |

The build-tag defect matters beyond tidiness: any plan that says "swap the tagged benchmark file for
the tagged live-mode file" is describing a file that does not exist. And the always-zero column is
worse than a missing column — the benchmark's output table has been reporting a structural-violation
count of 0 for every model, which reads as evidence of health and is an uninitialised variable.

**Alternatives rejected**: leaving `benchmark_test.go` in place. It has no assertions, no baselines,
one input, a statistic that is mislabelled and a column that is fictional.

---

## R9 — The scorer set: what actually exists, opened and checked

**Decision**: **Six** scorers in v1, not eight. Two of the original eight are cut, one is renamed to
match what it can actually measure, one had its direction inverted, and one was pointed at the wrong
file.

Every entry below was resolved by opening the function. This is the inventory the original
data-model §3 should have been built from.

### Shipping in v1

| Scorer | Backing | Direction | Where |
|---|---|---|---|
| `grounding_violations` | `len(domain.VerifyRendercvGrounding(master, merged, level, analysis))` | lower | `rendercv_grounding.go:123` |
| `structural_violations` | `len(domain.VerifyStructureIntegrity(master, merged))` | lower | `rendercv_structure.go:107` |
| `highlight_drift` | `len(domain.VerifyHighlightGrounding(master, merged))` | lower | `rendercv_structure.go:231` |
| `required_skills_missing` | `len(report.RequiredMissing)` | **lower** | `rendercv_completeness.go:10` |
| `nice_to_have_retention` | `report.NiceToHaveRetained` | higher | `rendercv_completeness.go:11` |
| `bullet_shortfalls` | `len(report.BulletShortfalls)` | **lower** | `rendercv_completeness.go:13` |

`report` is `domain.VerifyCompleteness(master, merged, analysis, cfg)`, computed once per run and
shared by the last three.

**`structural_violations` was pointed at a file with no verifier in it.** data-model §3 said "the
structural checks in `rendercv_shape.go`". `rendercv_shape.go` contains `ApplyHardLimits`
(`:143`), which *mutates* a document to fit limits and returns a `ShapeReport` of what it trimmed. It
verifies nothing. The verifier is `VerifyStructureIntegrity`, in `rendercv_structure.go`. A scorer
wired to `ApplyHardLimits` would have measured trimming, silently, and been believed.

**`required_skill_retention` does not exist and cannot be built without inventing a rule.** The
original spec wanted a retention *ratio*, higher-is-better. `CompletenessReport` exposes
`RequiredMissing []string` — a list of what went missing, with no denominator. The denominator is
computed inside `VerifyCompleteness` and never escapes; `matchesAnySkill` and `orderedSkillTokens` are
unexported. Constructing the ratio in the harness means re-deriving the required-skill token set with
the harness's own matching logic, which is precisely the parallel definition of quality FR-002 forbids
— and it would drift from production silently, because nothing would compare the two.

*Resolution*: score `len(RequiredMissing)`, lower-is-better, under the honest name
`required_skills_missing`. It reads an exported field of an existing verifier's report and adds
nothing. It is a weaker signal than a ratio — a case with one missing skill out of two scores the same
as one out of twenty — but it is a real one, and it moves when the thing it names moves.

**`achievements_per_job_min` had its direction inverted.** data-model §3 declared it `higher`. The
only backing quantity is `CompletenessReport.BulletShortfalls`, a `map[company]→bullet count` populated
only for companies that fell *below* `cfg.ExperienceBulletsMin` (`rendercv_completeness.go:108-132`).
It is a record of violations. More entries is worse. Renamed `bullet_shortfalls` and declared
**lower**. Had this shipped as written, the gate would have failed on improvement and passed on
regression — the exact inversion that makes a gate worse than no gate.

**`highlight_drift` and `grounding_violations` double-count the same defect.** `VerifyRendercvGrounding`
runs its own inline highlight-drift check at `rendercv_grounding.go:145-150` — the same `lcsCovered`
comparison against the same per-company master bullets that `VerifyHighlightGrounding` performs. One
ungrounded highlight therefore moves both scores.

*Resolution*: keep both, and say so everywhere it matters. They are kept because they separate: a
`grounding_violations` rise with `highlight_drift` flat means a fabricated company, section or project,
which is a different defect with a different fix. But the comparator must not present a single
ungrounded highlight as **two independent regressions** in its failure output, and the pair must never
be summed into a total. Contract C1-7 and FR-027 carry this.

*Rejected alternative*: drop `highlight_drift`. It loses the ability to tell drift from fabrication,
which is the distinction 033 and 035 were built around.

### Cut from v1 — nothing to wrap

| Scorer | Why it is not buildable now |
|---|---|
| `json_parse_failures` | `CompleteStructured` (`port.go:299-320`) loops up to three attempts and **discards the count**; only the last error survives into the returned value. Nothing exported reports how many attempts were spent. Measuring it needs new instrumentation on the retry loop in `internal/platform/llm/domain/port.go`. |
| `empty_output` | No zero-content check exists anywhere in `internal/generation/domain`. There is no function to delegate to; the scorer would *be* the rule. |

**Recommendation: cut both from v1, and do not add the instrumentation as part of this feature.**
The reason is FR-002's dependency direction. This feature's entire claim is that a green harness means
something about production *because it measures production's own rules*. A harness that ships new
production code so that it has something to measure has inverted that: it becomes the author of a
quality signal it then grades. Both are worth having, and both are legitimate small features against
`port.go` and the domain package — with their own justification, their own tests, and their own
review — after which they become scorers here for the cost of a version bump and a re-baseline.

Concretely, when they are picked up: `json_parse_failures` needs `CompleteStructured` to return an
attempt count or record it on a context-carried counter, and `empty_output` needs a
`VerifyNonEmpty(merged) []StructureViolation` in `internal/generation/domain`. Neither is in this
feature's scope.

---

## R10 — Package placement: the harness cannot reach the pipeline from a subpackage

**Decision**: The harness lives in **package `application`**, as files alongside `service.go` in
`apps/api/internal/generation/application/`. Not in a subpackage.

**Corrected**: plan.md put it in `internal/generation/application/eval/`. Nothing there can call the
pipeline. The whole tailoring path is unexported:

| Entry | Location |
|---|---|
| `tailorRendercvResume` | `service.go:421` |
| `selectWithCompleteness` | `service.go:326` |
| `summarize` | `service.go:370` |
| `fixStructureIntegrity` | `service.go:689` |
| `renderToPageTarget` | `service.go:581` |
| `renderDeps` | `service.go:518` |

The only exported entries are `Generate` (`service.go:745`) and `GenerateAdHoc` (`service.go:169`),
and both need a live Postgres `domain.Repository` and write run rows — which would make the "zero
credentials, zero external calls" gate need a database. A subpackage harness would have had exactly
two options: export the pipeline for a test's benefit, or drive it through Postgres. Both are worse
than putting the harness where the code is.

The provider *is* injectable, which is what makes replay work at all: `NewService` (`service.go:93`)
takes a `GenerationRouters` of five `llm.Provider` values (`service.go:71-79`), and every stage helper
takes an `lc llm.Provider` parameter. Substituting a `ReplayProvider` needs no production change.

**Second consequence: the harness core lives in `_test.go` files.** Non-test files in package
`application` ship in the API binary; a corpus loader, a fixture recorder and a baseline comparator do
not belong in a production build. Putting the harness in `eval_*_test.go` files in package
`application` gives it access to the unexported pipeline *and* keeps it out of the binary.

That forces one design change: the baseline-update command and the fixture recorder cannot be a
`cmd/update-baseline/main.go`, because `main` cannot import test files. They become **flags on the
test binary**:

```bash
go test ./internal/generation/application/ -run TestEvalCorpus -eval.update-baseline -eval.reason "..."
go test -tags eval_live ./internal/generation/application/ -run TestEvalRecord -eval.record -eval.case absent-skills
```

This is strictly better for C3-6 as well: recording is a different `-run` target under a build tag,
so it is structurally impossible for a failing deterministic run to record what it just saw.

**Naming**: files are prefixed `eval_` (`eval_corpus_test.go`, `eval_scorer_test.go`, …) so the
harness is visible as a unit inside a package it shares with the service.

**Alternatives rejected**:
- *Subpackage + export the pipeline*: widens production API to serve a test.
- *Subpackage + drive `Generate` through Postgres*: breaks FR-004, SC-002 and Principle V's
  no-credentials property in one move.
- *Non-test files in package `application`*: ships eval machinery in the API binary.

---

## R11 — Page fitting: stub the renderer through the seam that exists for it

**Decision**: In deterministic mode, `renderToPageTarget` runs with a **stubbed `renderDeps.render`
and `renderDeps.countPages`**. Everything else — prompts, schemas, merge, grounding, completeness,
structure fixing, and the expand/condense loop — runs as in production.

**Corrected — C3-5 contradicted C4-4.** C3-5 said "page fitting MUST run as in production". Production
page fitting shells out to the `rendercv` binary
(`internal/generation/infrastructure/rendercv_renderer.go:58`), a Python + Typst toolchain. C4-4 says
the gate must pass with no network, and quickstart's only prerequisite is `go build`. Both cannot hold.

The seam already exists and its own comment says why: `renderDeps` (`service.go:518`) is documented as
"injected so the page-target loop can be tested without a Typst toolchain or a live LLM". This is the
intended use.

**What the stub does**: `render` returns a fixed path without producing a PDF; `countPages` returns
the next value from a `page_counts` sequence declared in the case's `case.yaml`. The sequence is what
makes the expand/condense loop deterministic *and* exercisable — a case can declare `[3, 2]` to force
one condense round, or `[2]` to skip the loop. `expand` and `condense` keep their production
implementations and go through the `ReplayProvider`, so the LLM half of page fitting is genuinely
measured; only the PDF half is stubbed.

**What this costs, stated plainly**: the deterministic gate does not measure the renderer. A change
that breaks Typst output, or that produces a document `rendercv` rejects, passes this gate. That is a
real hole and it is accepted, because closing it would put a Python toolchain on the critical path of
every `go test ./...` run, which is how gates get skipped. Renderer correctness stays covered by the
existing infrastructure tests and by live mode.

**Measured budget check for SC-002**: `rendercv render` is ~800ms. Six cases at up to three renders
each would be ~14s — the 60s budget would have survived. Wall clock was never the problem. The problem
is *availability*: a CI runner or a fresh clone without Python and Typst cannot run the gate at all,
and nothing in the original spec measured that. FR-028 and quickstart step 1b now do: the gate must
pass with `PATH` stripped of the `rendercv` binary.

---

## R12 — Proving the gate can fail, durably

**Decision**: The gate's failure paths are proved by **committed tests that fail if the mechanism is
removed**, not by a session in which someone edited a file and reverted it.

**Corrected**: T023, T024 and T025 instructed an implementer to open `$EDITOR`, weaken a rule, observe
a failure, and `git checkout` the change away. Tasks.md then declared these "the only evidence the gate
works". They are not evidence of anything after the session ends: they revert themselves, they leave
no artifact, no reviewer can check them, and a re-run a month later re-tests nothing. A hard-sequencing
rule that says "must have been seen to fail" and produces no record is a rule that will be reported as
satisfied.

Four mechanical replacements, each a committed test in the ordinary suite:

**(a) `TestScorerDelegationIsExact` — the C1-1 tripwire.** For each scorer, the test independently
calls the domain function the scorer claims to wrap and asserts *equality* with the scorer's output,
over the corpus results plus a set of mutated documents. A scorer that adds a threshold, a filter, a
cap or any rule of its own breaks the equality. This is the only thing that makes C1-1 — "every scorer
MUST delegate to an existing check" — enforceable rather than aspirational; as written it was a rule
with no detector.

**(b) `TestScorersDetectInjectedDefects` — the wiring tripwire.** Take each case's scored result, inject
a defect into a copy (an experience entry for a company absent from master; a highlight sharing no
words with any master bullet; highlights stripped below `ExperienceBulletsMin`; a required skill
removed), and assert the relevant scorer moves in its worse direction. This catches a scorer that
returns a constant, is wired to the wrong function, or is never called. It is the mechanical form of
old T023 and it does not touch production code.

**(c) `TestReplayHashCoversEveryRequestField` — the R4 tripwire.** Take a committed fixture, perturb
each hashed field in turn — model key, prompt, `opts.System`, `opts.Temperature`, `opts.MaxTokens`,
`opts.ResponseMode`, `opts.JSONSchema` — and assert every single perturbation misses. This is the
mechanical form of old T024, and it is strictly stronger: it would have caught the original R4 defect,
where temperature and token caps were outside the hash and raising `selectMaxTokens` invalidated no
fixture at all.

**(d) `TestVersionMismatchRefuses` — the R5 tripwire.** Construct a baseline at `ScorerSetVersion-1`
and assert the comparator returns a refusal with no delta in it. Mechanical form of old T025.

The `$EDITOR` walkthroughs survive in quickstart as **optional human confirmation**, clearly marked as
such. They are useful once, to convince a person. They are not the evidence.

**Alternative considered**: build-tagged mutants of the domain package, compiled under
`//go:build evalmutate`, so a real weakened rule can be run against the real gate. Rejected for v1: it
requires mutation hooks inside production files, which is production surgery for a test's benefit, and
tripwires (a) and (b) cover the same failure without touching production code. Worth revisiting if the
scorer set grows past what (a) can compare exactly.

---

## Resolved unknowns summary

| Unknown | Resolution |
|---|---|
| What is "golden" | The scores, not the text (R1) |
| Judge | Deterministic code only, never an LLM (R2) |
| How it can gate a build | Two modes; only the replay mode gates (R3) |
| Stale fixtures | Keyed by request hash over model key, prompt, system, temperature, max tokens, response mode and schema; fail loudly on miss (R4) |
| Pipeline determinism | Fixed in production before harness work; corpus must also avoid `present` dates (R4a, R7) |
| Fixture volume | Ten-plus requests per case from the retry ladders; corpus stays small, recording is scripted, count is bounded and asserted (R4b, FR-024) |
| Scorer changes | Versioned; mismatch refuses comparison (R5) |
| Baseline storage | Committed JSON, one file per case (R6) |
| Corpus contents | Six synthetic cases, seeded from actual past failures; `unbounded-deliberation` moved to live mode (R7) |
| Existing apparatus | `benchmark_test.go` rewritten; mean-as-median, missing build tag and always-zero column all fixed (R8) |
| Which scorers are real | Six, all opened and checked; two cut for having nothing to wrap (R9) |
| Where the harness lives | Package `application`, in `eval_*_test.go` files — a subpackage cannot reach the pipeline (R10) |
| Page fitting under replay | Renderer stubbed through the existing `renderDeps` seam; C3-5 narrowed (R11) |
| Proving the gate fails | Four committed tripwire tests, not a reverted `$EDITOR` session (R12) |

## Corrections log

This research was revised on 2026-08-07 after an audit checked its claims against the tree. Seven were
materially wrong, and one class of error produced most of them.

1. **R4** hashed "model key, messages, response format, schema". There is no message list, and
   temperature and token caps were outside the hash — raising `selectMaxTokens` would have invalidated
   no fixture. Corrected in place.
2. **R7 and R8** were built around `resume_test/`, a directory that was never in the repository. It was
   untracked, is deleted, and is not recoverable. Every dependent requirement, contract, task and
   quickstart step has been removed rather than redirected.
3. **R7** kept `unbounded-deliberation` in a corpus that replays responses, where the provider-side
   failure it describes cannot occur. Moved to live mode.
4. **R1 and data-model §3** located the structural verifier in `rendercv_shape.go`, which has no
   verifier — only `ApplyHardLimits`, a mutator. A scorer wired there would have measured trimming.
5. **data-model §3** listed eight scorers. Two (`json_parse_failures`, `empty_output`) have nothing to
   wrap; one (`required_skill_retention`) needed a denominator that never leaves `VerifyCompleteness`;
   one (`achievements_per_job_min`) was declared higher-is-better over a count of violations, which
   would have failed the gate on improvement and passed it on regression.
6. **plan.md** put the harness in a subpackage that cannot reach the pipeline — every stage entry is
   unexported and the exported ones need Postgres.
7. **C3-5** required page fitting to "run as in production" while C4-4 required no network and
   quickstart required no toolchain. Production page fitting shells out to Python and Typst.

The class of error, stated once because it explains all seven: **confident claims about this
repository's own code, written without opening the file.** Every corrected item was checkable in
under a minute by grepping for the function name. The audit's other finding is the same shape in a
different register — `benchmark_test.go`'s doc comment says `-tags benchmark` and the file has no
build tag, and its `structuralViolations` column has been printing 0 for every model since it was
written, because the field is never incremented. Documentation about code is not evidence about code.
