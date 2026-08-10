# Phase 0 Research: Resume Generation Workspace

Every entry resolves a question the Technical Context could not answer from the spec alone.
Sources are the tree at `85e48e0`, not recollection — where the tree and
`specs/domains/resume-generation.md` disagree, R8 records which one is true.

---

## R1 — How the AI expresses a ranking without being able to express a rewrite

**Decision.** The selection stage's response type for profile-sourced content becomes:

```go
type RankedExperience struct {
    Company string `json:"company"`
    Ranking []int  `json:"ranking"` // source indices, most relevant first
}

type RankedSelection struct {
    Experience []RankedExperience
    Projects   []RankedProject
    Skills     RankedSkills // group order + per-group entry order, also indices
}
```

No `rephrased`, no free-text field of any kind. `HighlightRef.Rephrased` is deleted from the
workspace path.

**Rationale.** FR-009 requires the displayed and exported text of a profile-sourced item to be
identical to the master's. A field whose only legal value equals its source is a field with no
purpose except to carry a violation. The codebase has made this move twice already and documents
the principle explicitly (`specs/domains/resume-generation.md` §3.1, §2a): *make violations
unrepresentable rather than detectable*. 028 deleted `SectionsToDrop`, `ExperienceOrder` and
`Drop`; 035 deleted the summary field from `TailoredSelection`. This is the same deletion applied
to the last text field that still reaches the profile-sourced group.

It also collapses three existing checks on this path. `resolveHighlight`'s `lcsCovered` drift
comparison, `ungroundedMetrics`, and `StripUngroundedHighlights` all exist to catch what a
rewording can do. Against an index they have nothing to catch, and the pre-merge/post-merge
double-check on highlights becomes a no-op by construction.

**Consequence for grounding levels.** With no rewording, `strict`/`moderate`/`aggressive` have
nothing left to govern on bullets — under all three, a profile-sourced bullet is the master's
bullet. In the workspace the level therefore governs **the summary only**, which remains written
prose (spec assumption 3). This is a real narrowing of what the control means on this route and
the UI must say so rather than implying the old behaviour. `POST /documents/tailor` keeps today's
semantics for as long as it exists.

**Alternatives considered.**

- *Keep `rephrased`, validate byte-equality against the source.* Rejected: identical outcome,
  strictly more code, and it keeps the free-text channel open for the next person who relaxes the
  check.
- *Keep `rephrased` and show a reworded bullet as a third origin ("AI edit of your bullet").*
  Rejected: the spec's whole premise is a two-way distinction the user can read at a glance
  (SC-005, 90% correct identification). A third, semantically fuzzy origin is precisely the
  collapse User Story 2's "Why this priority" warns about.

---

## R2 — What "rank up to 2N and reject an omission" means operationally

**Decision.** For an entry with `A` available master bullets and target `N`
(`cfg.ExperienceBulletsMin`, default 8):

- the prompt asks for exactly `K = min(2N, A)` distinct indices, ordered by relevance;
- `VerifyRanking` rejects the response when it contains an out-of-range index, a duplicate, or
  fewer than `K` entries;
- the first `min(N, A)` ranked items are `selected = true`; the remaining ranked items are
  `selected = false` and rendered below them;
- when `A > K`, the unranked remainder is appended in **master order**, unselected, in the same
  visible list. Nothing is hidden.

Rejection retries the stage once, then falls back to **master order** for that entry rather than
failing the run (FR-010's own wording: "retrying or falls back to master order").

**Rationale.** FR-007 says "present up to 2N" and FR-010 says an omitting ranking is invalid.
Read together with a full-permutation requirement they conflict for any entry with more than 2N
bullets — the model would be asked to rank 40 bullets to display 16. `K = min(2N, A)` makes the
completeness rule *exact and cheap*: the response is invalid iff it fails to name K distinct
in-range indices, which is checkable in O(K) with no reference to relevance. The tail appearing
in master order satisfies the visible-omission rule (FR-011's counterpart for achievements, and
the "no master skill is missing from the list entirely" clause of FR-011/AS-1) without paying for
a ranking nobody reads.

Defaults make this concrete: `experienceBulletsMin` is 8, so K is 16 and the top 8 are
pre-selected — the spec's "target 4 → present 8" example scaled by the shipped config, which
spec assumption 4 explicitly authorises.

**Alternatives considered.**

- *Require a full permutation of all A bullets.* Rejected: superlinear prompt and response cost
  on the exact profiles that need the feature most (`oversized-profile` is already a corpus case),
  for ordering information below the fold.
- *Let the model choose how many to return.* Rejected: it makes "omitted" unfalsifiable, which is
  the failure mode 035's completeness gate was built for after three of seven measured candidates
  returned well-formed output with content quietly missing.

---

## R3 — Where the target bullet count `N` comes from

**Decision.** `N = cfg.ExperienceBulletsMin` and the hard cap stays
`cfg.ExperienceBulletsMax`, both read from the existing `resume_shape_setting` singleton via the
`ShapeProvider` port, resolved **once at the top of the run** exactly as `shapeConfig(ctx)` and
`summaryOption(ctx)` already are.

**Rationale.** Spec assumption 5 says page-fit limits and target counts come from existing resume
shape settings and this feature introduces no new layout configuration. The once-at-the-top rule
is the established discipline in `service.go:177` and `:120`, and it matters more here than for a
one-shot run: a workspace run persists and is revisited, so a settings change between the run and
the export must not silently change what the user approved. The run therefore **snapshots** its
resolved `ShapeConfig` into `generation_runs.shape_config` (031-FR-006 already requires each
generated resume to record the configuration it was produced with).

**Alternatives considered.** A per-run target on the request — rejected as new layout
configuration, which assumption 5 rules out.

---

## R4 — How AI suggestions are produced without weakening the ranking

**Decision.** A **separate LLM call** returning a separate type:

```go
type SuggestionSet struct {
    Experience []ExperienceSuggestions // {company, bullets []string}
    Skills     []string
}
```

routed through the **existing `generation-select` task key** — no new gateway model group. It
runs concurrently with the summary stage, after analysis, and takes the vacancy analysis plus the
master's entry identities (company names, skill group labels) — not the master's bullet text.

**Rationale.**

- *Separate type, separate call* is the 035 pattern verbatim: `TailoredSelection` has no summary
  field so a page-fit response cannot reword the summary. Here, `RankedSelection` has no text
  field, so a ranking response cannot smuggle a suggestion into the profile-sourced group, and a
  `SuggestionSet` has no index field, so a suggestion cannot claim to be the user's bullet. The
  two-way distinction the whole feature rests on is enforced at unmarshal.
- *Existing task key* keeps Constitution V trivially satisfied: `generation-select` already has a
  fallback chain terminating at local in `gateway/config.yaml`, and adding a key would require
  the `gateway_config_test.go` `requestedGenerationGroups` update the 034 notes describe. A
  suggestion is explicitly unverified and off by default — the stage where a premium model earns
  its price is the summary (035's measurement), not this one. If measurement later justifies a
  dedicated key, adding one is the three-step change 034 documents and changes nothing here.
- *Not given the master's bullet text* because a suggestion that paraphrases a real bullet is the
  FR-017 duplicate case, and the cheapest way to reduce those is not to show the model the
  material it would paraphrase. Remaining duplicates are suppressed deterministically (R6).

**Alternatives considered.**

- *One call returning both ranking and suggestions.* Rejected: it reopens the exact channel R1
  closes — a model that can emit text in the same response as indices can attach text to an index.
- *Suggestions from the summary (premium) stage.* Rejected: pays premium rates for content that
  is unverified by definition and unselected by default; SC-004 expects most of it never to be
  used.

---

## R5 — What "export exactly what I approved" means against the real render path

**Decision.** The workspace export path is a **render-once** path:

1. `domain.Assemble(master, selectionState)` builds a `RendercvMaster` from the selected items in
   the displayed order — pure data, no model call.
2. `ApplyFontSize`, then `RenderCvRenderer.Render`, then `CountPages`.
3. Over the page target → apply `CompactDesign` (a *layout* change: font metrics and spacing, no
   content mutation) and re-render **once**.
4. Still over → return `status: "blocked"` with an overflow report: how many pages over, and the
   lowest-ranked **selected** items as named drop candidates. Nothing is dropped or reworded.

`expandContent` (an LLM call) and `TrimHighlights` (silent content dropping) are **not** on this
path. `padHighlights` and the `ApplyHardLimits` truncation are likewise not applied to the
assembled document — the selection *is* the shape.

**Rationale.** FR-018 forbids post-selection rewording, condensing or re-ranking; FR-019 requires
the overflow to be *reported with candidates*, not resolved. The existing `renderToPageTarget`
loop does all three forbidden things — it calls `expand`, it calls `TrimHighlights` via
`condense`, and it records a `conflict` when section lengths were forced down. It is therefore
unusable here and is left untouched for the legacy `/documents/tailor` path.

`CompactDesign` is retained because it changes typography, not content: the exported words are
still exactly the approved words in the approved order, which is what FR-018 protects.

Ranking the drop candidates is free: `generation_items.rank` already orders each section by
relevance, so "the lowest-ranked candidates for removal" is a query, not a heuristic — a strictly
better answer than the measure-by-removal search §7.1 of the domain doc describes for a fitter
that does not exist.

**Alternatives considered.**

- *Build the chromedp density-ladder fitter §7.1 describes.* Rejected: it is an unbuilt feature
  from a different spec, and 042 needs page *measurement*, which `CountPages` already provides.
- *Auto-deselect the lowest-ranked items until it fits.* Rejected: FR-019 explicitly forbids
  resolving overflow silently, and SC-001's guarantee is about what the user approved.

---

## R6 — Deduplicating a suggestion against a profile item (FR-017)

**Decision.** Deterministic, post-response, in the domain layer. A suggested bullet is suppressed
when its normalised form (casefold, punctuation stripped, whitespace collapsed) matches a master
bullet for the **same entry**, or when word-set containment against any master bullet for that
entry is ≥0.9. Skills reuse the existing `tokens()` normalisation from `rendercv.go` and are
suppressed on exact normalised-token match against any master skill token.

**Rationale.** FR-017 is a hard rule, and a rule enforced by prompt wording is a rule with no
detector. `norm()` and `tokens()` already exist and are already the comparison basis everywhere
else in this pipeline, so the same input produces the same suppression on every machine — which
the 038 deterministic corpus requires.

The edge case in the spec ("AI returns a suggestion identical to an existing profile bullet: it
is deduplicated and shown only once, in the profile-sourced group") falls out: the profile item
is never touched, only the suggestion disappears.

**Alternatives considered.** An embedding-similarity threshold via pgvector — rejected:
non-deterministic across model versions, breaks the corpus gate, and buys nothing for a check
whose false-negative cost is one redundant suggestion the user can ignore.

---

## R7 — What the 038 eval corpus must gain

**Decision.** Three additions, all following the standing rule ("a production failure fixed from
now on arrives with a corpus case that would have caught it"):

1. A seventh scorer, `ranking_violations`, delegating to `domain.VerifyRanking` — the same
   delegation-not-reimplementation rule the other six follow, covered by the existing
   `TestScorerDelegationIsExact` and `TestScorersDetectInjectedDefects` tripwires.
2. A new case `ranked-oversized-entry`: one entry with far more than 2N bullets, whose baseline
   asserts K distinct in-range indices and zero ranking violations.
3. A new case `suggestion-duplicates-profile`: a vacancy whose obvious suggestions restate master
   bullets, asserting the suppression in R6 fires.

Because the scorer set changes, `ScorerSetVersion` bumps and **every** baseline is re-recorded —
the harness refuses to compare across scorer sets, and that refusal is the correct behaviour, not
an obstacle to work around.

**Rationale.** The ranking contract is the feature's central claim (SC-001, SC-003, SC-007) and
today nothing outside a unit test would notice it regressing. Both cases must use synthetic
fixtures with closed date ranges, per the corpus rules.

---

## R8 — Two documented components that do not exist

Recorded because the plan would otherwise claim reuse of code that is not in the tree, and
because 024-FR-015 requires every statement in a context document to be true of the repository.

| Documented at | Claim | Reality at `85e48e0` |
|---|---|---|
| `specs/domains/resume-generation.md` §4.1 | A `/api/tailoring` draft/proposal REST surface with eight endpoints, `SELECT … FOR UPDATE` write paths, and worker dispatch on `GeneratePayload.tailoringDraftId` | `internal/tailoring/` contains `doc.go` (one line) and `proposals.go` (validators only). No handler, no service, no route registration. `TailoringDraftID` is declared in `queue.go:63` and read nowhere. Migration `00036` created `tailored_drafts` and `edit_proposals`; sqlc generated 13 query methods; **no caller exists**. `apps/dashboard/src/features/tailoring/index.ts` is `export {};`. |
| `specs/domains/resume-generation.md` §7.1 | A chromedp density-ladder page fitter at `internal/generation/singlepage` with a 9-step ladder and ranked blocked-feedback | The package contains only `doc.go`. The real path is `RenderCvRenderer` (Typst) driven by `renderToPageTarget` in `service.go:744`, whose over-target strategy is `CompactDesign` then `TrimHighlights`. |

**Decision.** 042 supersedes the 020 review model rather than completing it: the accept/reject
diff surface and the ranked-item workspace are two answers to the same question, and shipping
both would leave the repository with two review surfaces and one user. Migration `00043` drops
`tailored_drafts` and `edit_proposals`, `internal/tailoring/` and `queue.GeneratePayload.
TailoringDraftID` are removed, and `dto/tailoring.go` goes with them (its `TailorResumeRequestDto`
has no reader either — `POST /documents/tailor` uses its own request shape in
`interfaces/http/documents.go`).

**This is a schema drop and is called out for explicit approval rather than performed quietly.**
It is safe on the evidence — no code writes these tables, so no deployment holds rows that were
not hand-inserted — but "safe" is a claim about the tree, and the user owns the decision. If they
prefer to keep them, the migration is dropped from the task list and everything else in this plan
is unchanged; the cost is two dead tables and a documentation footnote.

Correcting §4.1 and §7.1 in `specs/domains/resume-generation.md` happens at ship time, when 042's
durable requirements are folded in and `specs/042-…/` is removed.
