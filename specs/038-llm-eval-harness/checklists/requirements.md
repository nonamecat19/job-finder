# Requirements Quality Checklist: Scored Golden-Set Evaluation Harness

**Purpose**: Validate that the requirements in spec.md are complete, unambiguous, testable, and
consistent before implementation begins.
**Created**: 2026-08-07
**Revised**: 2026-08-07 after audit — CHK027 removed (it required migrating a directory that was never
in the repository), CHK030 rewritten, and a new section added covering the class of error found.
**Feature**: [spec.md](../spec.md)

## What is being measured

- [ ] CHK001 The spec is explicit that the golden is a **score**, not expected output text
      (research R1), so nobody implements a prose diff that gets disabled within a week
- [ ] CHK002 A requirement forbids introducing a second definition of quality (FR-002) — the harness
      must score with the checks production enforces
- [ ] CHK003 A requirement forbids an LLM judge (FR-003), with the reasoning recorded
- [ ] CHK004 Every scorer is required to be deterministic (FR-003), and nondeterministic scorers are
      excluded from the gating set rather than tolerated

## Gating versus reporting

- [ ] CHK005 The two modes are distinguished by requirement, not by convention (FR-004, FR-012)
- [ ] CHK006 Only the deterministic mode is required to gate (FR-005); no requirement makes a live,
      paid, flaky run block a change
- [ ] CHK007 Deterministic mode is required to need zero credentials and make zero external calls
      (FR-004), with a measurable bar (SC-002)
- [ ] CHK008 A speed requirement exists (SC-002) with a stated response to breaching it —
      parallelise, do not raise the budget

## The two failure modes that would make the gate a lie

- [ ] CHK009 A requirement forces a stale replay fixture to fail loudly (FR-010), not to replay an
      answer to a question no longer asked
- [ ] CHK010 A requirement forces a changed scoring definition to refuse comparison (FR-009) rather
      than report a cross-instrument delta
- [ ] CHK011 A requirement forbids baselines being auto-refreshed by a passing or failing run
      (contracts C2-5, C3-6) — otherwise any regression can be laundered
- [ ] CHK012 A requirement makes an unbaselined case a non-pass (FR-011)
- [ ] CHK013 A requirement makes an improvement fail rather than pass silently (FR-007), so baselines
      keep describing reality
- [ ] CHK014 Baseline updates are required to carry a reason (contracts C2-6)

## Corpus

- [ ] CHK015 A requirement enumerates the failure modes the corpus must cover (FR-018), drawn from
      failures that actually occurred rather than imagined ones
- [ ] CHK016 A requirement forces fixtures to be synthetic (FR-019) with a measurable bar (SC-011)
      and an automated check (contracts C5-5)
- [ ] CHK017 A requirement makes adding a case need no harness change (FR-020)
- [ ] CHK018 A requirement or contract forces each case to record **why** it exists (contracts C5-3)
- [ ] CHK019 A requirement carries the discipline forward: a production failure fixed later arrives
      with a case that would have caught it (US3 scenario 3, contracts C5-6)

## Live comparison

- [ ] CHK020 A requirement forces comparison across the whole corpus, not a single input (FR-013) —
      the specific weakness of the 035 model decision
- [ ] CHK021 A requirement makes results durable, diffable and reproducible by a later reader
      (FR-015, SC-008)
- [ ] CHK022 A requirement makes a structurally failing model a scored result rather than an aborted
      comparison (FR-016)
- [ ] CHK023 A requirement forbids presenting a partial comparison as complete (FR-017)
- [ ] CHK024 A requirement forbids live mode writing baselines (contracts C6-8)
- [ ] CHK025 The mean-labelled-as-median defect is captured as a requirement (FR-022), not left as a
      note someone may or may not act on

## Replacement, not accumulation

- [ ] CHK026 A requirement forces the existing apparatus to be replaced (FR-021) with a measurable
      bar (SC-010)
- [ ] CHK027 The independent record in `gateway/config.yaml` is required to point at the harness
      rather than drift alongside it (contracts C7-3)

## Testability

- [ ] CHK028 Every FR has an acceptance scenario or quickstart step that would fail if the
      requirement were violated
- [ ] CHK029 The gate's proof is a **committed artifact**, not a session — a test that fails if its
      mechanism is removed, not an `$EDITOR` edit that was reverted (tasks T008/T014/T015/T018,
      research R12)
- [ ] CHK030 No success criterion depends on subjective reading of generated prose

---

## Added 2026-08-07 after audit

The audit found one class of defect running through every artifact: **confident claims about this
repository's own code, written without opening the file.** The items below are aimed at that class,
not at the individual errors it produced. See research.md's corrections log.

### Claims about the codebase

- [ ] CHK031 Every named function, type, field or file in spec.md, plan.md, data-model.md,
      contracts/contracts.md and tasks.md has been **opened and confirmed to exist at that path**.
      Four separate documents said the structural verifier lives in `rendercv_shape.go`, which
      contains only `ApplyHardLimits`, a mutator
- [ ] CHK032 Every directory a requirement, contract, task or quickstart step depends on is **tracked
      in git**. `resume_test/` carried FR-021, SC-010, C7-2, C7-3, three tasks, a quickstart step and
      two research paragraphs, and was never in the repository at all
- [ ] CHK033 No claim rests on a **doc comment describing code** rather than the code.
      `benchmark_test.go`'s comment says run it with `-tags benchmark`; the file has no `//go:build`
      line and compiles into the ordinary suite
- [ ] CHK034 Every figure a document says is reported is confirmed to be **computed**. The benchmark's
      `structuralViolations` column has printed 0 for every model since it was written because nothing
      assigns it (FR-026)
- [ ] CHK035 Where two packages share a short name, the spec **disambiguates them**. `domain` means
      `internal/generation/domain` in one contract section and `internal/platform/llm/domain` in
      another; as written the code would not have compiled

### Scorers assumed to exist

- [ ] CHK036 Every scorer names the **exact expression** it evaluates and the file and line it comes
      from, not a description of a capability. Three of eight scorers named checks that do not exist
- [ ] CHK037 Every scorer's backing quantity is reachable from **exported** API. `required_skill_retention`
      needed a denominator computed inside `VerifyCompleteness` and never returned; building it in the
      harness would have been the parallel quality definition FR-002 forbids
- [ ] CHK038 Every scorer's **direction** has been checked against what its quantity actually counts.
      `achievements_per_job_min` was declared higher-is-better over a count of violations, which would
      have failed the gate on improvement and passed it on regression
- [ ] CHK039 Overlapping scorers are **declared**, and a single defect is not reported as two
      regressions (FR-027, C1-7, C2-8)
- [ ] CHK040 A requirement makes each scorer's delegation **mechanically verifiable** (FR-029, C1-1a).
      C1-1 forbade a scorer carrying its own rule and provided nothing that could detect one

### Reachability and environment

- [ ] CHK041 The code the harness must call is confirmed **reachable from where the plan puts it**.
      The plan placed the harness in a subpackage; every stage of the tailoring path is unexported and
      the exported entries need Postgres (research R10)
- [ ] CHK042 No two contracts require **incompatible** things. C3-5 required page fitting to run as in
      production; C4-4 required no network and quickstart required no toolchain; production page
      fitting shells out to Python and Typst
- [ ] CHK043 Every environmental dependency of the gate is stated and **tested**, not assumed absent.
      Toolchain availability was the real SC-002 risk and nothing measured it (FR-028, C4-4a)
- [ ] CHK044 A failure mode is only a deterministic case if a **replayed** run can reproduce it
      (C5-9). `unbounded-deliberation` is provider-side and cannot occur under replay

### Volume and drift

- [ ] CHK045 A requirement bounds the number of committed replay fixtures, and the bound is asserted
      (FR-024, C5-8). Fixture volume, not case count, is what makes this corpus expensive to maintain
      (research R4b)
- [ ] CHK046 A requirement keeps fixtures free of anything that changes with the wall clock (FR-030,
      C5-7). An open-ended date silently expires every affected fixture on 1 January
- [ ] CHK047 A requirement makes a live comparison reproducible from the artifact **alone** — served
      model and group per stage, request parameters, corpus revision, date (FR-023, C6-7). Scores and
      cost do not identify which model answered

## Notes

- Check items off as completed: `[x]`
- CHK009–CHK014 are the gate on the gate. A weakness there produces a harness that reports green for
  a pipeline it is no longer measuring, which is worse than having no harness at all — it
  manufactures confidence.
- CHK031–CHK035 are the cheapest items on this list. Every defect they describe was findable by
  grepping for a function name. None was found, in five documents, because the documents were
  checked against each other rather than against the tree.
