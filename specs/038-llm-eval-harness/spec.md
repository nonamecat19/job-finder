# Feature Specification: Scored Golden-Set Evaluation Harness

**Feature Branch**: `038-llm-eval-harness`

**Created**: 2026-08-07

**Status**: Draft — **revised 2026-08-07 after audit.** Several requirements referenced a directory
that was never in the repository and scorers that do not exist. See research.md's corrections log.

**Input**: User description: "Eval harness — extend benchmark_test.go + testdata into a scored golden
set. Highest ROI given your grounding/drift work in 033-035."

## Clarifications

### Session 2026-08-07

- Q: Should the harness be able to fail a build? → A: Yes, but only the deterministic half. Scores
  measured against live providers vary run to run and cost money; gating a build on them would make
  the build flaky and expensive. The deterministic half — replayed responses — gates; the live half
  reports.
- Q: What is a "golden" here — the exact expected output? → A: No. Generation output is not
  reproducible text. The golden is the **score**: how a fixed input scores against the platform's own
  existing checks, recorded as a baseline that a change must not regress.
- Q: What happens when a score legitimately improves or a check changes? → A: Baselines are updated
  deliberately, in the same change, with the reason recorded. A baseline that can be refreshed
  silently is not a baseline.
- Q: Does the harness need new judging logic? → A: No, and it must not invent any. The platform
  already has grounding verification, structural checks, drift detection and completeness
  verification. The harness scores with those, so what it measures is what production enforces.
- Q: Is an LLM used as a judge? → A: No. Every scorer is deterministic code.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A change that would make resumes worse is caught before merge (Priority: P1)

Someone edits a prompt, a schema, a grounding rule or a post-processing step. Before the change
merges, a fixed set of inputs is run through the pipeline against recorded model responses, scored by
the platform's own checks, and compared with recorded baselines. A change that increases grounding
violations, drops content, or breaks structure fails, and says which input and which score moved.

**Why this priority**: This is the whole feature. Features 033 and 035 built real quality machinery —
grounding verification, drift detection, completeness thresholds — and nothing runs it against a
fixed corpus on a change. Quality is currently checked by a human generating a resume and reading the
PDF. That does not scale, does not run on every change, and did not catch the fabricated summary that
motivated 035; a person reading one output did.

**Independent Test**: Can be fully tested by deliberately weakening a grounding rule and confirming
the harness fails, naming the input and the score that moved, with no live provider involved.

**Acceptance Scenarios**:

1. **Given** a fixed corpus and recorded baselines, **When** the harness runs on unchanged code,
   **Then** every score matches its baseline and it passes.
2. **Given** a change that weakens a grounding rule, **When** the harness runs, **Then** it fails and
   names the input and the score that regressed.
3. **Given** the harness runs, **When** it does, **Then** it makes no call to any external provider,
   needs no credential, and costs nothing.
4. **Given** a change that improves a score, **When** the harness runs, **Then** it reports the
   improvement and requires the baseline to be updated deliberately rather than passing silently.

---

### User Story 2 - Model choices are made on measurement, not on one run (Priority: P1)

Deciding which model serves a stage means running the corpus against each candidate, on real
providers, and comparing scores, cost and latency across the whole set rather than a single vacancy.
The results are recorded so the decision is auditable later.

**Why this priority**: Equal to P1 because this is the decision the platform makes repeatedly and
gets wrong expensively. 035's split was chosen from a hand-run comparison on one vacancy on one day,
recorded in a YAML comment. It was a good decision, but it is not reproducible, and the next person
who wants to swap a model has to rebuild the whole apparatus from scratch. The existing
`benchmark_test.go` is that apparatus half-built: live-only, one input, no assertions, and it reports
a mean under a column headed "Median ms".

**Independent Test**: Can be tested by running the corpus against two candidates and producing a
comparison of quality scores, cost and latency across every input, reproducible by a second person
from the recorded artifact.

**Acceptance Scenarios**:

1. **Given** a set of candidate models, **When** the harness runs against real providers, **Then** it
   produces per-model quality scores, cost and latency across the whole corpus, not one input.
2. **Given** a comparison run, **When** it completes, **Then** its results are recorded in a durable,
   diffable form that a later reader can reproduce.
3. **Given** a live run, **When** it is invoked, **Then** it is explicitly opted into and never runs
   as part of the ordinary test suite.
4. **Given** a live run against a model that fails structurally, **When** it completes, **Then** the
   failure is scored and reported rather than aborting the comparison.

---

### User Story 3 - The corpus covers what actually goes wrong (Priority: P2)

The inputs are not one profile and one vacancy. They cover the cases that have actually produced bad
output: a vacancy demanding skills the candidate lacks, a very large profile, a nearly empty one, a
vacancy with no clear requirements, and a profile whose dates make the derived experience figure
ambiguous.

**Why this priority**: A corpus of one measures one thing. Every failure this platform has recorded —
the fabricated summary, the silently truncated selection, the model that returned nothing — came from
a specific input shape, and each should be a permanent case. It is P2 because the machinery must
exist before there is anywhere to put the cases.

One historical failure does **not** belong in the deterministic corpus: the model that consumed its
token budget on reasoning and returned nothing. That is a provider-side behaviour, and a mode which
replays recorded responses cannot reproduce it — the case would measure a fixture the harness itself
wrote. It is a live-mode case (research R7).

**Independent Test**: Can be tested by confirming each known historical failure is a case in the
corpus and that the harness scores it in the way that failure would have been caught.

**Acceptance Scenarios**:

1. **Given** the corpus, **When** it is inspected, **Then** every historically-observed failure mode
   is represented by at least one case.
2. **Given** a case reproducing a past failure, **When** the harness runs against the code as it was
   when the failure occurred, **Then** the case fails.
3. **Given** a new failure found in production, **When** it is fixed, **Then** the fix includes a
   corpus case that would have caught it.

---

### User Story 4 - Cost and speed are tracked alongside quality (Priority: P3)

Every run records what the corpus cost and how long it took, per model and per stage, so a quality
improvement that quietly triples the bill is visible as a trade rather than a surprise.

**Why this priority**: 035 committed to measurable cost and latency targets. Without recorded
measurement those become claims. It is P3 because the quality regression gate is the urgent part and
this rides on the same runs.

**Independent Test**: Can be tested by running the corpus twice against models of different price and
confirming the recorded cost and latency differ correspondingly.

**Acceptance Scenarios**:

1. **Given** a live run, **When** it completes, **Then** total and per-input cost and latency are
   recorded per model and per stage.
2. **Given** two runs against different models, **When** compared, **Then** the quality-versus-cost
   trade is readable from the recorded results without re-running anything.

---

### Edge Cases

- What happens when a scorer itself changes? Every baseline it produced becomes invalid. The harness
  must detect that the scoring definition changed and refuse to compare against baselines produced by
  the old one, rather than reporting a meaningless delta.
- What happens when a live provider is down mid-comparison? That model's run is recorded as
  incomplete and the comparison continues for the others; a partial result must not be presented as
  a complete one.
- What happens when someone adds a corpus case with no baseline? The harness reports it as unbaselined
  and does not treat its absence as a pass.
- What happens when a recorded response no longer matches the request the pipeline now sends — a
  prompt changed, so the replay is stale? The harness must detect the mismatch and fail loudly rather
  than replaying a response to a question that is no longer asked.
- What happens when a score is nondeterministic even with replayed responses? It is not admissible as
  a gate. Any scorer that cannot produce the same number twice on the same input belongs in the
  reporting half, not the gating half.
- What happens when the corpus contains the real profile of a real person? It must not. Fixtures are
  synthetic, and no case may carry a real person's contact details or employment history.
- What happens when two scorers move on the same defect? They must say so. One ungrounded highlight
  raises both the grounding count and the drift count, because the grounding verifier performs the
  drift comparison inline. The failure report must present that as one defect seen by two instruments,
  not as two regressions, and the two must never be added together.
- What happens on a machine with no PDF toolchain? The gate must still run. The renderer is stubbed
  through the injection seam the pipeline already provides for exactly this, and the consequence — the
  gate does not measure the renderer — is stated rather than hidden.
- What happens when a scorer silently stops delegating — someone inlines a threshold, or wires it to a
  neighbouring function with a similar name? A test must fail. A rule that forbids something and has no
  detector is a comment.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The harness MUST run a fixed corpus of inputs through the real generation pipeline and
  score each result.
- **FR-002**: Scoring MUST reuse the platform's existing verification — grounding, structural,
  drift, and completeness checks — and MUST NOT introduce a parallel definition of quality. A measure
  with no existing check behind it is not admissible as a scorer, however desirable it is; it is a
  request for new production instrumentation, justified separately (research R9).
- **FR-003**: No scorer may use a language model to judge output. Every scorer MUST be deterministic
  code.
- **FR-004**: The harness MUST have a deterministic mode that replays recorded model responses,
  requires no credential, makes no external call, and costs nothing.
- **FR-005**: Deterministic mode MUST run as part of the ordinary test suite and MUST fail the suite
  on a regression against a recorded baseline.
- **FR-006**: A failure MUST name the input case and the score that moved, with its baseline value and
  its new value.
- **FR-007**: A score that improves MUST be reported and MUST require a deliberate baseline update
  rather than passing silently.
- **FR-008**: Baselines MUST be stored in a durable, diffable, reviewable form in the repository.
- **FR-009**: The harness MUST detect that a scoring definition has changed and MUST refuse to compare
  against baselines produced under the previous definition.
- **FR-010**: The harness MUST detect a replayed response that no longer matches the request the
  pipeline sends, and MUST fail loudly rather than replaying a stale answer.
- **FR-011**: A corpus case with no baseline MUST be reported as unbaselined and MUST NOT count as a
  pass.
- **FR-012**: The harness MUST have a live mode that runs the corpus against real providers, and this
  mode MUST be explicitly opted into and MUST never run as part of the ordinary test suite.
- **FR-013**: Live mode MUST accept several candidate models and produce per-model scores across the
  whole corpus.
- **FR-014**: Live mode MUST record cost and latency per model, per stage and per input.
- **FR-015**: Live-mode results MUST be recorded in a durable, diffable form that a later reader can
  reproduce and audit.
- **FR-016**: A model that fails structurally MUST be scored as failing and reported, not cause the
  comparison to abort.
- **FR-017**: A provider outage mid-comparison MUST mark that model's run incomplete; a partial
  result MUST NOT be presented as complete.
- **FR-018**: The deterministic corpus MUST include at least one case for every historically-observed
  failure mode that a replayed run can reproduce: a vacancy demanding absent skills, an oversized
  profile, a nearly empty profile, a vacancy with no clear requirements, and a profile with ambiguous
  dates, plus a healthy baseline case. A failure mode that is provider-side — a model exhausting its
  token budget and returning nothing — MUST be a live-mode case and MUST NOT be represented by a
  replayed fixture, which would measure the fixture rather than the pipeline.
- **FR-019**: Every corpus fixture MUST be synthetic. No case may contain a real person's name,
  contact details or employment history.
- **FR-020**: Adding a corpus case MUST NOT require changing the harness.
- **FR-021**: The harness MUST replace `benchmark_test.go` rather than sitting beside it, so exactly
  one evaluation apparatus exists in the repository.
- **FR-022**: The existing benchmark's aggregate timing MUST be reported correctly — a statistic
  labelled as a median MUST be a median.
- **FR-023**: A live-mode result MUST record everything a second person needs to reproduce the run:
  for each stage, the served model and served group actually used, whether the call was substituted or
  escalated, the temperature, the token cap and the response mode requested, plus the grounding level,
  the shape configuration, the corpus revision and the run date. A comparison that records only scores
  and cost is not reproducible, and SC-008 would be unverifiable.
- **FR-024**: The number of committed replay fixtures MUST be bounded and the bound MUST be asserted
  by a test. One corpus case issues more than one request — the structured-output retry loop, the
  grounding retry ladder, the completeness escalation ladder and the structure-fix ladder each add
  distinct requests (research R4b) — so fixture volume grows faster than case count. Fixtures MUST be
  produced by the recorder and MUST NOT be hand-authored or hand-edited.
- **FR-025**: Live mode MUST be excluded from the ordinary test suite by a build constraint, not by a
  runtime skip. A file that compiles into the ordinary suite and returns early is still in the suite,
  and its exclusion depends on an environment variable rather than on the build.
- **FR-026**: Every figure a run reports MUST be computed. A field that is declared and printed but
  never assigned MUST NOT appear in any output — a column of zeros reads as evidence of health.
- **FR-027**: Where two scorers can move on the same underlying defect, the overlap MUST be declared,
  a single defect MUST NOT be reported as two independent regressions, and scores MUST NOT be summed
  into a total.
- **FR-028**: The deterministic gate MUST run without the PDF rendering toolchain installed. It MUST
  substitute the renderer through the pipeline's existing injection seam and MUST NOT invoke the
  `rendercv` binary, Python or Typst.
- **FR-029**: Every scorer's delegation MUST be mechanically verifiable: a test MUST fail if a
  scorer's value diverges from the existing check it names. FR-002 without a detector is a convention.
- **FR-030**: Corpus fixtures MUST use closed date ranges only. An open-ended role makes the derived
  experience figure a function of the wall clock, which silently expires every affected fixture.

### Key Entities *(include if data involved)*

- **Corpus Case**: one named, synthetic input — a master profile, a vacancy, and the configuration to
  run them under — plus what makes it interesting.
- **Score**: one deterministic measurement of one result, produced by an existing platform check.
- **Baseline**: the recorded scores for a case under a stated scoring definition, stored in the
  repository and changed only deliberately.
- **Replay Fixture**: a recorded model response bound to the request that produced it, so a stale
  replay is detectable.
- **Comparison Run**: a live execution of the corpus against one or more candidate models, recording
  scores, cost and latency.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A deliberately weakened grounding rule is caught by the harness in 100% of runs, naming
  the case and the score.
- **SC-002**: Deterministic mode completes in under 60 seconds, makes zero external calls, requires
  zero credentials, and passes on a machine where the `rendercv` binary, Python and Typst are absent.
  Wall clock was never the binding constraint — a render is roughly 800ms, and six cases at up to
  three renders each would fit the budget with the toolchain present. Toolchain *availability* is the
  constraint, and it is the one nothing previously measured.
- **SC-003**: The harness runs on every change through the ordinary test suite, with zero added cost.
- **SC-004**: 100% of corpus cases have a baseline or are reported as unbaselined; zero unbaselined
  cases pass silently.
- **SC-005**: A changed scoring definition is detected in 100% of runs, and no meaningless
  cross-definition delta is ever reported.
- **SC-006**: A stale replay fixture is detected in 100% of runs rather than producing a score.
- **SC-007**: Every historically-observed failure mode is represented by at least one corpus case — in
  the deterministic corpus where a replayed run can reproduce it, in live mode where only a real
  provider can — and each deterministic case fails when run against the code as it was when that
  failure occurred, with the commit checked recorded in the case.
- **SC-008**: A model comparison across the whole corpus is reproducible by a second person from the
  recorded artifact alone — the artifact names the served model and group per stage, the request
  parameters, the grounding level, the shape config, the corpus revision and the date (FR-023).
- **SC-009**: Cost and latency per model and per stage are recorded for every live run, so 035's cost
  and speed targets are verified from measurement rather than asserted.
- **SC-010**: Exactly one evaluation apparatus exists in the repository: `benchmark_test.go` is gone,
  rewritten into live mode, and no assertionless live benchmark remains beside the harness.
- **SC-011**: Zero corpus fixtures contain a real person's identifying data.
- **SC-012**: Every scorer in the gating set produces a value identical to the existing check it
  names, asserted by test over the corpus and over mutated documents; zero scorers carry a rule of
  their own.
- **SC-013**: The committed replay fixture count stays under its declared bound, asserted by test, and
  100% of fixtures are recorder-produced.
- **SC-014**: Every figure in every reported table is computed from a measurement; zero always-zero
  columns.

## Assumptions

- The existing verification built by features 033 and 035 — grounding levels, drift detection,
  structural checks, completeness thresholds — is the definition of quality. This feature measures
  with it and does not redefine it.
- The generation pipeline is the first and primary subject. Other AI tasks (matching, ghost-job
  detection, salary inference) are out of scope for this feature but the harness should not be shaped
  so as to exclude them later.
- Replay fixtures are recorded from real provider responses once, committed, and refreshed
  deliberately — the same discipline as the baselines.
- Cost figures come from the routing service's reported cost, already captured by feature 035. If
  feature 036 has shipped, its recorded traces are a cross-check rather than a second source.
- The existing `benchmark_test.go` is the starting point, not a thing to preserve: it has no
  assertions, no baselines, one input, reports a mean while calling it a median, has no build tag
  despite a doc comment claiming one, and prints a structural-violation column that is never assigned.
- The platform stays single-user and self-hosted, so the corpus lives in the repository rather than
  in an external evaluation service.
- The harness lives in package `application` alongside `service.go`, in test files, because the entire
  tailoring path is unexported and the exported entry points require Postgres (research R10). This is
  a constraint discovered from the code, not a preference.
- The deterministic gate stubs the PDF renderer through the pipeline's existing `renderDeps` seam and
  therefore does not measure renderer correctness. That gap is accepted and stays covered by the
  infrastructure tests and by live mode (research R11).
- Only checks that already exist as exported functions or exported report fields can be scored. Two
  desirable measures — how many structured-output retries a call consumed, and whether a response was
  empty — have nothing to delegate to and are out of scope until the instrumentation exists (R9).
