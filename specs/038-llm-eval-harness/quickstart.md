# Quickstart: Scored Golden-Set Evaluation Harness

**Feature**: `038-llm-eval-harness` | **Date**: 2026-08-07
**Revised**: 2026-08-07 after audit. Every command targeted a package that cannot reach the pipeline,
step 10 checked a directory that was never in the repository, and steps 2, 4 and 5 were manual
`$EDITOR` edits that revert themselves and leave nothing a reviewer can check. See research.md's
corrections log.

**Ten** scenarios — the header previously said nine over ten. Steps 1–7 need no credentials, no
network, no money and **no PDF toolchain**. Steps 8–9 are the live comparison. Step 10 confirms the
old apparatus is gone. Run them in order.

All commands run from `apps/api/`. The harness is in package `application`, so its target is
`./internal/generation/application/` — there is no `eval` subpackage (research R10).

---

## 0. Prerequisites

```bash
cd apps/api && go build ./...
```

Nothing else. That is the point of deterministic mode — and unlike the previous version of this
document, it is now true: the renderer is stubbed through `renderDeps`, so no Python and no Typst is
needed (research R11).

---

## 1. The gate runs clean (FR-005, SC-002, SC-003)

```bash
cd apps/api
time go test ./internal/generation/application/ -run TestEvalCorpus -v
```

**Expect**:

- Every case scored, every score matching its baseline, PASS.
- Under 60 seconds (SC-002).
- No credential read, no network call, no `rendercv` invocation.

### 1a. No credentials

```bash
env -u OPENROUTER_API_KEY -u CEREBRAS_API_KEY -u GROQ_API_KEY -u COHERE_API_KEY \
    -u LITELLM_MASTER_KEY -u GATEWAY_URL \
    go test ./internal/generation/application/ -run TestEvalCorpus
```

**Expect**: still passes. A gate that needs credentials is a gate that will be skipped.

### 1b. No PDF toolchain (FR-028, C4-4a) — **new, and the one nobody measured**

```bash
PATH=/usr/bin:/bin go test ./internal/generation/application/ -run TestEvalCorpus   # rendercv not on PATH
command -v rendercv || echo "rendercv absent — good"
```

**Expect**: passes with `rendercv` unavailable. Wall clock was never the risk here — a render is about
800ms and six cases at up to three renders each fits inside 60s comfortably. The risk is a CI runner
or a fresh clone that has no Python and no Typst at all, which would make the gate unrunnable rather
than slow.

**Fails if**: the run shells out to `rendercv`. Then C3-5 has been read as its old, pre-correction
form and the gate has a hard dependency on a toolchain it was specified never to need.

---

## 2. The gate's own proof — mechanical (FR-006, FR-029, SC-001, SC-012)

**Corrected**: this step used to instruct `$EDITOR internal/generation/domain/rendercv_grounding.go`,
weaken a rule by hand, observe a failure, and `git checkout` it away. That is not evidence. It reverts
itself, leaves no artifact, and cannot be re-checked by anyone who was not in the room. It also pointed
at a failure naming `absent-skills`, a case that does not exist until Phase 4.

The proof is four committed tripwires (research R12), all in the ordinary suite:

```bash
go test ./internal/generation/application/ -v \
  -run 'TestScorerDelegationIsExact|TestScorersDetectInjectedDefects|TestReplayHashCoversEveryRequestField|TestVersionMismatchRefuses'
```

**Expect**: all four pass, and each is a test that would fail if its mechanism were removed:

| Test | What it would catch |
|---|---|
| `TestScorerDelegationIsExact` | a scorer that adds a threshold, filter or cap of its own — the C1-1 detector (FR-029) |
| `TestScorersDetectInjectedDefects` | a scorer wired to the wrong function, returning a constant, or never called |
| `TestReplayHashCoversEveryRequestField` | a hashed field left out of the key — the defect R4 originally shipped with |
| `TestVersionMismatchRefuses` | a cross-instrument delta reported as a number |

**Fails if**: any of these passes with its target mechanism deleted. Verify that once, by hand, when
implementing — that is the moment to check, and the test is the record afterwards.

### 2a. Optional human confirmation

Useful once, to convince a person. **Not** the evidence, and not a task:

```bash
$EDITOR internal/generation/domain/rendercv_grounding.go   # weaken a skill-token check
go test ./internal/generation/application/ -run TestEvalCorpus
git checkout internal/generation/domain/rendercv_grounding.go
```

**Expect**: FAIL, naming the case, the scorer (`grounding_violations`), the baseline and the actual
value — and, if the weakened rule was the inline highlight-drift check at
`rendercv_grounding.go:145-150`, reporting `highlight_drift` as the **same defect seen twice**, not as
a second independent regression (FR-027, C2-8).

---

## 3. An improvement does not pass silently (FR-007)

```bash
# tighten a rule so a case scores better than its baseline
go test ./internal/generation/application/ -run TestEvalCorpus
```

**Expect**: FAIL with "baseline needs updating", reporting the improvement. Not a silent pass.

Then update it deliberately. **Corrected**: this is a flag on the test binary, not
`go run ./…/cmd/update-baseline` — `main` cannot import test files, and the harness is test files
(research R10).

```bash
go test ./internal/generation/application/ -run TestEvalCorpus \
  -eval.update-baseline -eval.case absent-skills -eval.reason "tightened skill-token grounding"
git diff internal/generation/application/evaldata/baselines/absent-skills.json
```

**Expect**: a reviewable one-case diff with a non-empty `reason` (C2-6). A baseline that can move
without a reason is not a baseline.

---

## 4. A stale replay fixture fails loudly (FR-010, SC-006) — **the second gate**

The failure that would manufacture green. Proved mechanically by
`TestReplayHashCoversEveryRequestField` (step 2), which perturbs **each hashed field in turn** — model
key, prompt, `opts.System`, `opts.Temperature`, `opts.MaxTokens`, `opts.ResponseMode`,
`opts.JSONSchema` — and asserts every perturbation misses.

That coverage matters more than a single hand edit: the original key omitted temperature and token
caps, so raising `selectMaxTokens` would have invalidated no fixture at all, and a hand-edited prompt
would still have looked fine.

Optional human confirmation:

```bash
$EDITOR internal/generation/application/rendercv_llm.go   # change a prompt
go test ./internal/generation/application/ -run TestEvalCorpus
git checkout internal/generation/application/rendercv_llm.go
```

**Expect**: FAIL naming the case and the unmatched request summary — model key, prompt length,
temperature, token cap, response mode.

---

## 5. A changed scorer refuses to compare (FR-009, SC-005)

Proved mechanically by `TestVersionMismatchRefuses` (step 2), which constructs a baseline at
`ScorerSetVersion-1` and asserts a refusal containing no delta.

Optional human confirmation: bump `ScorerSetVersion` in `eval_scorer_test.go` without re-recording,
then run the gate.

**Expect**: a **refusal**, not a delta — "baselines recorded under scorer set version N, current is
N+1". No number is reported, because a cross-instrument number would be fiction.

---

## 6. An unbaselined case does not pass (FR-011, SC-004)

```bash
cp -r internal/generation/application/evaldata/cases/baseline \
      internal/generation/application/evaldata/cases/scratch
go test ./internal/generation/application/ -run TestEvalCorpus
```

**Expect**: FAIL reporting `scratch` as unbaselined. Absence of a baseline is not evidence of health.

```bash
rm -rf internal/generation/application/evaldata/cases/scratch
```

---

## 7. Corpus discipline (FR-018, FR-019, FR-020, FR-024, FR-030, SC-011, SC-013)

```bash
go test ./internal/generation/application/ -run TestCorpusDiscipline -v
```

**Expect**:

- Every case discovered by walking `evaldata/cases/`, with no case name appearing in a Go file (C5-1).
- Every `case.yaml` has a non-empty `why` and a `page_counts` sequence (C5-2, C5-3).
- No fixture contains a real-identity marker (C5-4, C5-5).
- **No fixture contains an open-ended date** — no role ending in `present` (C5-7, FR-030). An
  open-ended role makes the derived experience figure a function of the wall clock, which changes the
  summary prompt and therefore the request hash on 1 January, silently expiring every affected
  fixture.
- **The committed replay fixture count is under its declared bound** (C5-8, FR-024). Fixture volume,
  not case count, is what makes this corpus expensive: `CompleteStructured` re-prompts on invalid JSON
  (`port.go:302`) and the grounding, escalation and structure-fix ladders each add distinct requests,
  so one case can issue ten or more.

Then confirm coverage by hand against the six cases in data-model §2 — `baseline`, `absent-skills`,
`oversized-profile`, `nearly-empty-profile`, `vague-vacancy`, `ambiguous-dates` — each tracing to a
failure this repository actually recorded. `unbounded-deliberation` is deliberately **not** here: a
model burning its token budget is provider-side and a replay harness cannot reproduce it (C5-9). It is
a live-mode case, checked in step 8.

Adding a case must need no harness change:

```bash
mkdir -p internal/generation/application/evaldata/cases/new-case
# add case.yaml, master.yaml, vacancy.txt — touch no Go file
go test ./internal/generation/application/ -run TestEvalCorpus
```

**Expect**: the new case is discovered and reported as unbaselined. Then remove it.

---

## 8. A live model comparison (FR-012–FR-017, FR-023, SC-008, SC-009)

Real providers. Costs money. Explicitly opted into.

```bash
make up
EVAL_LIVE=1 go test -tags eval_live ./internal/generation/application/ \
  -run TestLiveComparison -timeout 60m -v \
  -eval.models generation-select,generation-select-premium
```

**Expect**:

- Every case run against both candidates, not one input — including `unbounded-deliberation`, which
  exists only here (C6-9).
- Per-model, per-stage, per-input cost and latency recorded (C6-3).
- **Per stage: the served model, served group, and whether the call was substituted or escalated**,
  plus temperature, token cap, response mode, grounding level, shape config, corpus revision and run
  date (C6-7, FR-023). Without these the artifact is not reproducible and SC-008 cannot be checked: a
  task key does not determine which model answered — the proxy picks a tier and may substitute or
  escalate.
- Any statistic labelled a median **is** a median (C6-4).
- **No always-zero column** (C6-4a, FR-026). The benchmark this replaces printed a
  `structuralViolations` column that was declared, printed, and never assigned.
- A durable artifact written, reproducible by a second reader (C6-7, SC-008).

Confirm it cannot run by accident:

```bash
go test ./... 2>&1 | grep -i "TestLiveComparison"   # expect no output
grep -c '^//go:build eval_live' internal/generation/application/eval_live_test.go   # expect 1
```

**Expect**: nothing from the first, `1` from the second. The build constraint is the load-bearing
half — `benchmark_test.go` claimed `-tags benchmark` in its doc comment and had **no `//go:build` line
at all**, so it compiled into the ordinary suite and skipped at runtime on an environment variable
(C6-1, FR-025).

---

## 9. Live mode degrades honestly (FR-016, FR-017)

```bash
# point one candidate at an unreachable endpoint, then rerun step 8
```

**Expect**:

- The failing candidate is **scored as failing** and the comparison continues for the other (C6-5).
- Its result is marked `Incomplete` and is not presented as a complete comparison (C6-6).
- No baseline is written by live mode under any circumstance (C6-8).

---

## 10. Exactly one apparatus remains (FR-021, SC-010)

**Corrected**: this step used to run `ls resume_test/` and expect "not found". `resume_test/` was never
in this repository — it existed as untracked working files, has been deleted, and is not in git
history. There is nothing to check for and nothing that could have been migrated out of it. The check
that remains is the one that was always real:

```bash
ls internal/generation/application/benchmark_test.go 2>/dev/null   # expect: not found
grep -rn "GENERATION_BENCHMARK" apps/api                           # expect: no output
```

**Expect**: `benchmark_test.go` is gone, rewritten into `eval_live_test.go` (C7-1), and no live
assertionless benchmark remains beside the harness (C7-2). `gateway/config.yaml`'s model-comparison
comment now points at the harness artifact rather than carrying its own record (C7-3).

---

## Success summary

| Step | Requirement | Criterion |
|---|---|---|
| 1 | FR-004, FR-005 | SC-002, SC-003 — under 60s, zero calls, zero credentials |
| 1b | FR-028 | SC-002 — passes with no PDF toolchain present |
| 2 | FR-006, FR-029 | SC-001, SC-012 — four committed tripwires, each failing if its mechanism is removed |
| 3 | FR-007 | improvement reported, baseline update deliberate and reasoned |
| 4 | FR-010 | SC-006 — every hashed field proved to invalidate a fixture |
| 5 | FR-009 | SC-005 — refusal, never a cross-instrument delta |
| 6 | FR-011 | SC-004 — unbaselined never passes |
| 7 | FR-018–FR-020, FR-024, FR-030 | SC-011, SC-013 — discovered, justified, synthetic, closed-dated, bounded |
| 8 | FR-012–FR-015, FR-023, FR-025, FR-026 | SC-008, SC-009, SC-014 — reproducible comparison, build-tagged, no fictional column |
| 9 | FR-016, FR-017 | failing scored, incomplete marked, no baseline written |
| 10 | FR-021 | SC-010 — one apparatus |
