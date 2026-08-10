# Contract: LLM response types and their verifiers

`apps/api/internal/generation/domain/ranking.go` and `ranking_verify.go`. These are the types
`llm.CompleteStructured[T]` unmarshals into, so **the type is the contract** — a field that does
not exist cannot be returned, and a response carrying it is discarded at unmarshal rather than
checked afterwards.

---

## 1. The ranking stage — `RankedSelection`

Task key: `generation-select` (unchanged). Timeout: `selectStageTimeout` (240 s). Max tokens:
`selectMaxTokens` (16384) — a response of indices is far smaller than today's, so this is
headroom, not a constraint.

```go
type RankedExperience struct {
    Company string `json:"company" jsonschema_description:"company name copied EXACTLY from the master"`
    Ranking []int  `json:"ranking" jsonschema_description:"the K bullet [index] values for THIS entry, most relevant to the vacancy first"`
}

type RankedProject struct {
    Name    string `json:"name"`
    Ranking []int  `json:"ranking"`
}

type RankedSkills struct {
    GroupOrder []int `json:"groupOrder" jsonschema_description:"the [index] of each master skill group, most relevant first"`
}

type RankedSelection struct {
    Experience []RankedExperience `json:"experience"`
    Projects   []RankedProject    `json:"projects"`
    Skills     RankedSkills       `json:"skills"`
}
```

**What is absent, and why it is absent rather than validated:**

| Absent field | Would have permitted |
|---|---|
| `rephrased` / any `string` under an entry | A bullet that merges two originals, borrows another employer's, or attaches a metric nobody claimed (FR-009) |
| `summary` | A ranking pass silently rewriting the premium-written summary (035-FR-010, preserved) |
| `suggestions` | A fabricated bullet landing in the profile-sourced group (FR-016) |
| any `label` / `details` on skills | The silent skill deletion §2a of the domain doc records; group *contents* still come from the master untouched, ordered by the deterministic `RankSkills` |
| `drop` / `sectionsToDrop` / `experienceOrder` | The 028 structural violations, still unrepresentable |

`RankedSkills.GroupOrder` is the one genuinely new model input on skills, and it is bounded the
same way: an ordering of existing group indices. Within a group, entry order stays
`domain.RankSkills`'s deterministic computation. When `skillsMaxGroups` is 0 (the default), the
returned order still determines which groups are pre-selected versus shown unselected (FR-011,
User Story 4 AS-2) — no group is ever removed from the list.

### `VerifyRanking`

```go
func VerifyRanking(available int, target int, ranking []int) []RankingViolation
```

Where `K = min(2*target, available)`. Violations, all structural:

| Violation | Condition |
|---|---|
| `out_of_range` | an index `< 0` or `>= available` |
| `duplicate` | an index appearing twice |
| `short` | `len(ranking) < K` |

Any violation ⇒ the response is invalid for that entry. Recovery, per FR-010:

1. retry the stage once (the existing `groundingAttempts` idiom);
2. on a second failure, fall back to **master order** for that entry — `ranking = [0,1,…,K-1]` —
   and set `generation_sections.fallback_used = true`.

A lossy list never reaches the user, and SC-007 ("rejected/retried rankings occur in under 5% of
runs") is measured from the `fallback_used` column and the `ranking_violations` scorer, not
estimated.

`len(ranking) > K` is **not** a violation: extra ranked indices are accepted and the display
simply shows more ranked candidates than required. Rejecting a model that ranked *more* material
than asked would be a rejection with no user-visible defect behind it.

### Prompt shape (`buildRankPrompt`)

Reuses `buildSelectPrompt`'s numbered-bullet rendering verbatim — that numbering is already the
index space `HighlightRef.SourceIndex` addresses, and re-deriving it would create a second
numbering to keep in sync. The rules block changes to:

```text
- For each experience entry, return the K most relevant bullet [index] values, most relevant first.
- K is stated per entry below. Return exactly K distinct indices; do not return fewer.
- Return indices ONLY. There is no field for bullet text: you are ordering the candidate's own
  wording, not writing it.
- Do NOT write a summary and do NOT suggest new bullets. Separate steps do both.
- Keep every experience entry, in the EXACT order shown; do not reorder, drop, add or rename.
```

with `K = min(2N, A)` printed after each entry's bullet list.

---

## 2. The suggestion stage — `SuggestionSet`

Task key: `generation-select` (research R4 — no new gateway group). Runs concurrently with the
summary stage.

```go
type ExperienceSuggestions struct {
    Company string   `json:"company" jsonschema_description:"company name copied EXACTLY from the list below"`
    Bullets []string `json:"bullets" jsonschema_description:"achievement bullets the vacancy calls for that this candidate's profile does not contain"`
}

type SuggestionSet struct {
    Experience []ExperienceSuggestions `json:"experience"`
    Skills     []string                `json:"skills" jsonschema_description:"skills the vacancy asks for that the profile does not list"`
}
```

**What is absent:** any index field. A suggestion cannot claim to be one of the user's bullets,
which is the mirror image of §1 — together the two types make the profile/AI distinction a
property of the wire format rather than of a label the UI applies afterwards.

**Input:** the `VacancyAnalysis`, the master's company names and skill *group labels*. **Not** the
master's bullet text or skill tokens (R4): a model not shown the material cannot paraphrase it,
which reduces the FR-017 duplicate case at the source.

**Post-processing, deterministic and in the domain layer:**

1. an entry whose `company` does not match a master company (via `norm`) is dropped;
2. `SuppressDuplicateSuggestions` removes a bullet whose normalised form matches a master bullet
   for that entry, or whose word-set containment against one is ≥0.9 (R6);
3. a suggested skill whose normalised token matches any master skill token is removed;
4. survivors become `generation_items` with `origin='ai'`, `selected=false`, ranked after every
   profile item in their section.

**No grounding check runs on a `SuggestionSet`** — FR-016 says so explicitly, and running one
would be incoherent: the content is *defined* as material the profile does not contain. What
protects the user is that it is unselected, badged unverified, and absent from an untouched
export (SC-004). Zero suggestions is a normal outcome and renders an empty state, not an error.

---

## 3. The summary stage — unchanged

`TailoredSummary` and `SummaryBrief` are untouched. The brief's `Highlights` field is now
populated from **the selected profile items** rather than from a `TailoredSelection` — the same
data by a different route, so `SelectedHighlights` is replaced by an equivalent reading over the
run's items. The summary remains the one written part, keeps its grounding checks, keeps its 034
model choice, and stays immutable for the rest of the run.

---

## 4. What the eval corpus asserts about these types

Per research R7, `ranking_violations` joins the six scorers, delegating to `VerifyRanking`
exactly — asserted by extending the existing `TestScorerDelegationIsExact`, and its
wrong-direction movement asserted by extending `TestScorersDetectInjectedDefects`.

Two new cases, both synthetic with closed date ranges:

- `ranked-oversized-entry` — an entry with far more than 2N bullets. Baseline asserts K distinct
  in-range indices and zero ranking violations.
- `suggestion-duplicates-profile` — a vacancy whose obvious suggestions restate master bullets.
  Baseline asserts the suppression fires.

`ScorerSetVersion` bumps, so every baseline is re-recorded with a stated reason. The harness
refusing to compare across scorer sets is the designed behaviour.
