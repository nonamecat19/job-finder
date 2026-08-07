# Implementation Plan: Scored Golden-Set Evaluation Harness

**Branch**: `038-llm-eval-harness` | **Date**: 2026-08-07 | **Spec**: [spec.md](./spec.md)
**Revised**: 2026-08-07 after audit. The Source Code block put the harness in a subpackage that cannot
reach the pipeline and listed a directory that was never in the repository; the Structure Decision
justified a path choice with a claim about Go's `testdata` handling that is false. See research.md's
corrections log.

**Input**: Feature specification from `/specs/038-llm-eval-harness/spec.md`

## Summary

Features 033 and 035 built real quality machinery — grounding verification with levels, drift
detection, structural checks, a vacancy-weighted completeness verifier. Nothing runs any of it over a
fixed corpus when code changes. Quality is verified by a person generating a resume and reading the
PDF, which is how the fabricated summary that motivated 035 was found.

Turn those existing checks into scorers — **six of them, each one opened and confirmed to exist**
(research R9) — run them over six synthetic cases seeded from failures this project has actually
recorded, and compare the resulting scores against baselines committed to the repository.

The design turns on one split (research R3). **Deterministic mode** replays recorded provider
responses, runs inside `go test ./...`, costs nothing, and *fails the build* on a regression. **Live
mode** calls real providers, is explicitly opted into, and answers "which model should serve this
stage" without gating anything. Today's `benchmark_test.go` is live-only and assertionless, so it
gates nothing and runs approximately never — and it compiles into the ordinary suite despite a doc
comment claiming a build tag it does not have.

Two failure modes shape the rest. A stale replay fixture would manufacture confidence, so fixtures
are keyed by a hash of the request the pipeline actually sends and a miss fails loudly (R4). A
changed scorer would make every prior baseline a measurement from a different instrument, so scorers
are versioned and a mismatch refuses to compare rather than reporting a meaningless delta (R5).

## Technical Context

**Language/Version**: Go 1.26 (`apps/api`). No dashboard change.

**Primary Dependencies**: none added. Scoring reuses `internal/generation/domain`; replay is an
in-process `llm.Provider` implementation (the facade alias for `internal/platform/llm/domain.Provider`
— a *different* `domain` package from the generation one).

**Storage**: repository files only — corpus fixtures, replay fixtures and baselines as committed
JSON/YAML under `internal/generation/application/evaldata/`. No database, no migration.

**Testing**: `go test` — deterministic mode *is* a test and runs in the ordinary suite; live mode is
excluded by a `//go:build eval_live` constraint, not by a runtime skip. The whole harness lives in
`_test.go` files in package `application`, so none of it ships in the API binary. Deterministic mode
substitutes two things and only two: the provider (`ReplayProvider` in all five `GenerationRouters`
fields) and the PDF renderer (`renderDeps.render` / `renderDeps.countPages`). Everything between them
is the production path.

**Target Platform**: Linux server, self-hosted via Docker Compose

**Project Type**: Test/quality infrastructure inside the Go API

**Performance Goals**: deterministic mode under 60 seconds wall clock (SC-002), so it can run on every
change without anyone wanting to skip it

**Constraints**: zero external calls, zero credentials **and no PDF toolchain** in deterministic mode;
no LLM judge anywhere; no second definition of quality; every fixture synthetic; every scorer backed by
a check that already exists

**Scale/Scope**: **six** deterministic corpus cases plus one live-only case, one baseline file each,
one replay fixture *set* per case — a set is ten or more files, because the retry ladders make each
attempt a distinct request (research R4b). **Six** scorers, not eight; two of the originally listed
eight have nothing to delegate to (research R9). The generation pipeline is the subject; other AI
tasks are out of scope but must not be designed out.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Assessment |
|---|---|
| **I. No Auto-Apply** | Untouched. The harness reads fixtures and scores output; it has no outbound path and submits nothing. **PASS** |
| **II. Grounded Generation** | **The principle this feature exists to serve.** Grounding is currently enforced per-run and verified by a human reading a PDF. This makes it a measured, regression-gated property of the codebase. FR-002 forbids inventing a competing definition, so what the harness measures is exactly what production enforces. **PASS** |
| **III. Typed Contracts** | No cross-language boundary. Corpus, baselines and replay fixtures are Go-side test data; nothing reaches TypeScript, so no tygo regeneration and no `packages/shared` change. **PASS** |
| **IV. Test Discipline** | Directly reinforces it. Deterministic mode runs in `go test` on every change (FR-005), which is what makes this a quality gate rather than a report. Live mode is excluded by a build constraint, not a runtime skip (FR-025) — the file this replaces claimed a build tag in its doc comment and had none, so it compiled into the ordinary suite and skipped on an environment variable. **PASS with a stated gap**: the gate stubs the PDF renderer, so it does not test renderer correctness. See Complexity Tracking. |
| **V. Local-First** | Deterministic mode makes zero external calls, needs zero credentials, **and needs no Python or Typst toolchain** (FR-004, FR-028, SC-002), so the gate works on a machine with no provider access and no rendering stack at all. This is stronger than the pre-audit design, which required the `rendercv` binary via C3-5 while simultaneously claiming `go build` was the only prerequisite. Live mode reaches providers only through the existing gateway and only when opted into. **PASS** |

**Post-Phase-1 re-check**: two things changed after the audit and both were re-checked against the
principles.

The harness moved from a subpackage into package `application` as test files. Principle III is
unaffected — still no cross-language boundary. Principle IV is *strengthened*: because the harness is
test-only, none of it can leak into the API binary, which an `application/eval/` package of ordinary
Go files would have done.

The renderer is now stubbed. That is the one place the design knowingly measures less than production,
and it buys Principle V: the alternative was a gate that could not run without a Python toolchain,
which contradicts both FR-004's spirit and the "runs on every change" property the whole feature rests
on. Recorded in Complexity Tracking rather than waved through.

The design commits fixtures and baselines to the repository, which grows it. The alternative — an
external evaluation service — would put the gate's source of truth outside the repository being gated
and break the local-first property. Still **PASS**.

## Project Structure

### Documentation (this feature)

```text
specs/038-llm-eval-harness/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── contracts.md     # Phase 1 output
├── checklists/
│   └── requirements.md
└── spec.md
```

### Source Code (repository root)

**Corrected**: this block previously put the harness in a subpackage, `application/eval/`, and listed
a `resume_test/` directory that was never in the repository. Neither is right. The subpackage cannot
reach the pipeline (research R10), and `resume_test/` does not exist and never did in git — every
reference to it has been removed from this feature rather than redirected.

```text
apps/api/internal/generation/application/
├── service.go                  # existing — the pipeline the harness drives
├── eval_corpus_test.go         # NEW: case discovery + discipline assertions (FR-020, C5-*)
├── eval_scorer_test.go         # NEW: the six scorers, ScorerSetVersion, delegation tripwire (FR-029)
├── eval_baseline_test.go       # NEW: load, compare, refuse on version mismatch, -eval.update-baseline
├── eval_replay_test.go         # NEW: ReplayProvider by request hash + hash-coverage tripwire
├── eval_run_test.go            # NEW: drive one case, with renderDeps stubbed (FR-028)
├── eval_test.go                # NEW: TestEvalCorpus — the gate, ordinary suite, no build tag (FR-005)
├── eval_live_test.go           # NEW: //go:build eval_live — comparison + fixture recorder (FR-012)
├── evaldata/
│   ├── cases/                  # NEW: six synthetic case dirs
│   ├── replays/                # NEW: recorded responses keyed by request hash
│   └── baselines/              # NEW: one committed JSON baseline per case (FR-008)
└── benchmark_test.go           # DELETED, rewritten into eval_live_test.go (FR-021, FR-022, FR-025, FR-026)
```

**Structure Decision**: the harness is **package `application`**, in test files beside `service.go`.

The subpackage was impossible. Every stage of the tailoring path is unexported —
`tailorRendercvResume` (`service.go:421`), `selectWithCompleteness` (`:326`), `summarize` (`:370`),
`fixStructureIntegrity` (`:689`), `renderToPageTarget` (`:581`), and the `renderDeps` seam (`:518`).
The only exported entries, `Generate` (`:745`) and `GenerateAdHoc` (`:169`), require a live Postgres
repository and write run rows, which would make a credential-free gate need a database. A subpackage
had exactly two escapes — export the pipeline for a test's benefit, or drive the gate through Postgres
— and both are worse than moving the harness to the code.

What *is* injectable is the provider, which is the only thing replay needs: `NewService` (`:93`) takes
`GenerationRouters` (`:71-79`), five `llm.Provider` fields, and every stage helper takes an
`lc llm.Provider`. No production change is required.

**Test files, not ordinary files**: a corpus loader, a fixture recorder and a baseline comparator have
no business in the API binary. The cost is that `main` cannot import them, so the baseline-update and
record commands are flags on the test binary (`-eval.update-baseline`, `-eval.record`) rather than a
`cmd/`. That also strengthens C3-6: the recorder lives behind `//go:build eval_live`, so it is not
compiled into the binary that runs the gate.

**Fixture directory name — the previous justification was false.** This plan said fixtures sit in
`evaldata/` rather than `testdata/` "so they are addressable from both the deterministic and the live
entry points without Go's `testdata` tooling conventions getting in the way". There is no such
obstacle. `testdata` is excluded from *package matching* — `./...` does not descend into it looking for
packages — and nothing else. Files inside it are read normally, from ordinary and build-tagged tests
alike, and `go test` sets the working directory to the package directory either way. `testdata/` would
work identically.

The real reason is a naming preference, stated as one: `testdata/` conventionally holds inputs to a
particular test, and this corpus is a committed gate artifact reviewed like source — baselines change
in review, cases are argued about, the fixture count is bounded by requirement. A distinct name says
that. If a reviewer prefers `testdata/eval/`, nothing in this design resists the change; only the paths
in data-model §1, quickstart and tasks move.

## Phase 0: Research

See [research.md](./research.md). Twelve decisions, all resolved, four added after an audit found that
several were confident claims about this repository made without opening the file. The load-bearing
ones:

- **R1**: the golden is the *score*, not the output text. Generated prose is not reproducible; a text
  diff would be disabled within a week.
- **R2**: no LLM judge — nondeterministic, costly, and it would create the second definition of
  quality FR-002 forbids.
- **R3**: two modes, and only replay gates. This is why the harness will actually run, unlike the
  existing benchmark.
- **R4**: fixtures keyed by request hash over **model key, prompt, `opts.System`, `opts.Temperature`,
  `opts.MaxTokens`, `opts.ResponseMode` and `opts.JSONSchema`**, because a stale replay manufacturing
  green is worse than no harness at all. The original key listed a message list that does not exist and
  omitted temperature and token caps, so raising `selectMaxTokens` would have invalidated no fixture.
- **R4a**: **the pipeline was not deterministic, and this was fixed in production before any harness
  work.** Two violation slices were appended in Go's randomised map order and fed back into the retry
  prompt, so a retry on identical input sent differently-ordered text run to run; and
  `DeriveTotalExperienceYears` resolved `present` against `time.Now().Year()`. Both are landed:
  the slices are sorted, `DeriveTotalExperienceYearsAsOf(master, now int)` exists with the original
  retained as a wrapper, and `internal/generation/domain/rendercv_determinism_test.go` covers all
  three sites. The wall-clock wrapper is still what production calls, so the corpus closes the
  remaining gap by forbidding open-ended dates in fixtures (FR-030) rather than threading a clock
  through four production call sites.
- **R4b**: **fixture volume, not case count, is the maintenance cost.** `CompleteStructured` re-prompts
  on invalid JSON, so attempts 2 and 3 are distinct requests with distinct hashes; the grounding retry
  ladder, the completeness escalation ladder and the structure-fix ladder each add more. One case can
  issue ten or more. Hence six cases, a bounded and asserted fixture count (FR-024), and a scripted
  recorder — never a hand-authored fixture.
- **R5**: versioned scorers, because a cross-definition delta means nothing and auto-refreshing
  baselines would make the gate unfalsifiable.
- **R7**: six synthetic cases, each traceable to a failure this repository actually recorded, all with
  closed date ranges. `unbounded-deliberation` moved to live mode: a replay harness cannot reproduce a
  provider burning its own token budget.
- **R8**: replace `benchmark_test.go` — mean labelled a median, no build tag despite its doc comment,
  and a structural-violation column that is never assigned.
- **R9**: six scorers, not eight. Two have nothing to delegate to, one needed a denominator that never
  leaves `VerifyCompleteness`, one was pointed at a file with no verifier in it, and one had its
  direction inverted so the gate would have failed on improvement.
- **R10**: package `application`, not a subpackage — the pipeline is entirely unexported.
- **R11**: the renderer is stubbed through the `renderDeps` seam; C3-5 could not have coexisted with
  C4-4.
- **R12**: the gate's failure paths are proved by committed tripwire tests, not by an `$EDITOR`
  session that reverts itself and leaves no artifact.

## Phase 1: Design

- [data-model.md](./data-model.md) — `Case`, `Score`, `Baseline`, `ReplayFixture`, `ComparisonRun`,
  the scorer set and its version, and the on-disk layout.
- [contracts/contracts.md](./contracts/contracts.md) — the scorer contract, the baseline comparison
  contract, the replay provider contract, the corpus-extension contract, and the live-mode contract.
- [quickstart.md](./quickstart.md) — runnable validation: run the gate clean, run it with no
  credentials and no PDF toolchain, exercise the four tripwires, stale a fixture, bump a scorer
  version, add an unbaselined case, and run a live two-model comparison.

## Complexity Tracking

The pre-audit plan said "No violations. Complexity Tracking omitted." Three decisions taken since do
carry real cost, and each was chosen over a simpler-looking alternative that turned out to be worse.
Recording them here so a reviewer can disagree with the trade rather than discover it.

| Decision | Cost accepted | Simpler alternative, and why it was rejected |
|---|---|---|
| **The harness is `_test.go` files in package `application`** | Unusual shape: several hundred lines of machinery in test files, and `main` cannot import them — so the baseline-update and record entry points are flags on the test binary rather than a `cmd/`. Test files also carry no godoc and are invisible to consumers. | *A subpackage `application/eval/`*: **impossible**, not merely worse. Every stage of the tailoring path is unexported and the two exported entries need Postgres (research R10). Escaping that meant either exporting the pipeline to serve a test — widening production API permanently for a harness — or driving the gate through a database, which destroys FR-004's zero-credential property. *Ordinary `.go` files in package `application`*: possible, and it would give back the `cmd/`; rejected because a corpus loader, a fixture recorder and a baseline comparator would then ship inside the API binary. |
| **The PDF renderer is stubbed** | **The gate does not measure renderer correctness.** A change that produces a document `rendercv` rejects, or that breaks Typst output, passes this gate. Real hole, stated in spec Assumptions and in the domain documentation (T056). | *Running the real renderer*, as pre-audit C3-5 required: puts a Python + Typst toolchain on the critical path of every `go test ./...`. Wall clock was not the objection — ~800ms per render, six cases, well inside 60s. Availability was: a CI runner or fresh clone without the toolchain could not run the gate at all, and a gate that cannot run is not a gate. Coverage stays with the infrastructure tests and live mode. |
| **Two scorers move on the same defect** | `grounding_violations` and `highlight_drift` both rise on one ungrounded highlight, because `VerifyRendercvGrounding` runs the drift comparison inline (`rendercv_grounding.go:145-150`). The comparator needs extra logic to report co-movement as one defect and to refuse to sum scores (FR-027, C1-7, C2-8). | *Dropping `highlight_drift`*: removes the logic and the confusion, but loses the ability to distinguish drift from fabrication — grounding up with drift flat means an invented company, section or project, a different defect with a different fix. That distinction is what 033 and 035 were built around. *Deduplicating inside the scorers*: would mean the harness reinterpreting production's violation lists, which is the parallel quality definition FR-002 forbids. |

Two further costs are deliberate rather than complex, and are not trade-offs a reviewer needs to
re-open: the corpus and its fixtures are committed to the repository (the alternative puts the gate's
source of truth outside the thing being gated), and two desirable scorers are cut rather than built
(research R9 — a harness that ships production instrumentation so it has something to grade has
inverted its own justification).
