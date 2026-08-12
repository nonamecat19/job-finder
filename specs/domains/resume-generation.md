# Domain: Resume Generation & Tailoring

Consolidates **020** constrained AI resume tailoring, **028** structure preservation,
**031** configurable generation shape, **032** certifications as a configurable category,
**035** split-model generation, **042** the resume generation workspace.

Implementation: `apps/api/internal/generation/`, `internal/resumeshape/`,
`apps/dashboard/src/features/generate/`. How it works:
[`docs/ai/generation.md`](../../docs/docs/ai/generation.md).

`internal/tailoring/` and `dto/tailoring.go` no longer exist — 042 retired the unwired 020
review surface rather than completing it (§ 4.1).

This domain implements Constitution II (Grounded Generation). Every rule below exists
because a resume is used in a real hiring decision, and a fabricated one damages the user in
a way they cannot easily detect or undo.

---

## 0. The staged pipeline (035)

A resume is not produced by one model call. It is produced by three, each addressed by its
own gateway task key, because the four jobs inside a tailoring pass have different
requirements and only one of them is worth a premium model.

| Stage | Task key | What it does | Verified for |
|---|---|---|---|
| Analysis | `generation-analyze` | vacancy text → required/nice-to-have skills, keywords | — |
| Selection | `generation-select` | which achievements per job; their rewording | grounding + **completeness** |
| Summary | `generation-summary` | writes the 2-4 sentence summary | grounding, independently |
| Page fit | `generation-select` | expand to hit the page target; condensing is code (`TrimHighlights`) | grounding |

**The summary is the only part that is written rather than selected**, and it is the only
part where a cheap model was measured fabricating (2026-08-07: glm-4.7 invented a summary
while producing perfectly grounded skills and bullets). Selection and page fitting run on an
economy model at roughly 1/30th the price with no measured loss in grounding.

**The split is enforced by types, not by prompt wording.** `TailoredSelection` has no summary
field and `TailoredSummary` has nothing else, so a page-fit response that tries to reword the
summary has nowhere to put it and is discarded at unmarshal. Once written, the summary is
immutable for the rest of the run — the structure re-prompt and the page fitter both carry it
through rather than rebuilding it from the master (035-FR-010).

**Completeness gate (035-FR-006/FR-007).** A cheap model's characteristic failure is not
malformed output — it is well-formed output with content quietly missing, which nothing
downstream would notice. Three of seven candidates measured did exactly this. So selection
output is checked against what the vacancy asked for before anything renders:

- every master skill matching a **required** vacancy skill must survive — exact, no tolerance
- master skills matching **nice-to-have** vacancy skills must survive at 80% or above
- per-job achievement counts must meet the configured minimum

A shortfall retries once on the economy model; a second shortfall escalates that stage to the
premium model and marks the run. A shortfall that survives escalation fails the run rather
than rendering a hollowed-out resume. When the analysis lists no required skills at all the
weighted checks would be vacuously true, so the gate falls back to a structural check (group
count equals the master's) and records that it did.

Note what is *not* a shortfall: anything about skills. A selection response has no skills
field at all — skill order is computed by `RankSkills`, and the master's own groups carry
through the merge untouched — so a model cannot truncate them whether it omits them or not.
The gate now bites on the material the model does return: the per-job achievement counts.

**Provenance (035-FR-012/FR-017).** Each run records, per stage, which model served it,
whether the chain fell back, the duration and the measured cost — the last from the proxy's
own `usage.cost`, not an estimate. A summary served by a fallback is marked on the document
and shown on the review surface: a user who was told they get the strict model must not
silently receive the cheap one. Substitution is detected from the proxy's
`x-litellm-attempted-fallbacks` header, so the application still never learns which upstream
model was configured for a key.

**Condensing is code, not a fourth model call.** Over-target documents get the
compact design first, then `domain.TrimHighlights` drops bullets from the end of each entry
toward ~60% of the configured maximum. The selection stage already ordered them by relevance,
so the tail is the least relevant material; the wording that survives is wording that was
already verified. Page fitting is a layout problem, and it no longer buys a third opportunity
to reword the document. When there is nothing left to trim the loop stops rather than
re-rendering an identical file. Expansion is still a model call: choosing *which* further
bullets earn a place is a judgement, not arithmetic.

**Cover letters are on demand** (035-FR-013). A tailoring run produces a resume only; the
letter is requested against a finished resume via `POST /documents/{id}/cover-letter`.

---

## 1. What the AI may change (020-FR-001, 028-FR-006, 031-FR-020)

**Allow-list.** AI edits are restricted to:

1. the professional summary,
2. which description bullets appear under each work-experience entry, and an optional
   rewording of a bullet held against that bullet alone (see §2a),
3. skill *proposals* the user reviews — never applied skill text; see §2a,
4. *(028)* the additional fields 028 admits to the list,
5. *(031, 032)* section presence/length, only as driven by the saved shape config.

**Everything else is off-limits** (020-FR-002): contact information, name, job titles,
employer names, dates, education, credentials. Not "discouraged" — the proposal is discarded.

020-SC-001 is the measurable form: 100% of AI-tailored resumes differ from the master only
within the allow-list.

## 2. Grounding (020-FR-003, 032-FR-007)

Generated text is derived from and traceable to the user's master profile and the target job
posting. No invented experience, skills, employers, dates, degrees or metrics.

- 020-SC-005: zero accepted edits contain claims absent from the master resume — auditable
  through the activity trail.
- 032-FR-007 / 032-SC-003: 100% of certifications in a generated resume trace to a
  certification in the master profile.
- 031-FR-017 / 031-SC-008, 032-FR-006: **a configured minimum is never a licence to
  fabricate.** When the profile holds less content than the floor asks for, generation uses
  what exists and records the shortfall. Grounding checks pass at the same rate regardless of
  configuration.

### 2a. What the model is not allowed to decide

The failures above were enforced by *checking* the model's free text — and a check can only
catch what it can see. Each rule below either stops asking for the text, or looks at the one
thing the old check was blind to.

**Skill order is code, not a model decision.** The select prompt used to ask for a rewritten
`details` string per group ("vacancy-required skills first"). In that response a reorder and
a silent deletion are indistinguishable, and only the completeness gate stood between a
dropped skill and the page. `TailoredSelection` no longer carries skills; `MergeTailored`
carries every group over from the master untouched; and `domain.RankSkills` orders them from
the vacancy analysis:

- within a group, entries sort by relevance — a match against a **required** vacancy skill
  outranks a **nice-to-have**, which outranks the rest; ties keep the master's authored order,
- an entry's score is the best match any of its tokens makes, so a slash-joined entry cannot
  outrank a single required skill by listing more words,
- the ordering is a permutation: nothing is added, reworded or dropped,
- groups keep the master's order unless `skillsMaxGroups` forces a choice, in which case they
  sort by how much of the vacancy they cover so the cap keeps the relevant ones,
- pinned groups (`Spoken Languages`) are left exactly as authored,
- a group's authored `skills_level` (`all` / `medium` / `relevant`) bounds how much of it a
  tailored resume renders, applied **after** ordering: `medium` keeps the top half (ceil, so a
  single-skill group never empties) and `relevant` keeps only entries the vacancy asks for —
  a `relevant` group with nothing matching is dropped from the rendered document, while the
  profile and workspace still hold it in full. The trim runs on master-fresh skills only; the
  workspace export path never applies it (the selection is the shape, FR-018),
- the same inputs always produce the same output.

**Achievements are chosen by reference, not written.** `TailoredExperience.Highlights` and
`TailoredProject.Highlights` are `[]HighlightRef` — `{sourceIndex, rephrased}` — where
`sourceIndex` names one of the numbered bullets the prompt listed for that entry. The prompt
numbers them; `MergeTailored` resolves them via `ResolveHighlights`:

- an index outside the entry's bullet list, or a repeat of one already used, is dropped,
- an omitted `rephrased` yields the master's bullet verbatim,
- a `rephrased` is checked against **the one bullet it names** — word overlap and metrics —
  and a failure resolves to the original rather than failing the run,
- under **strict** grounding `rephrased` is not consulted at all.

A bullet that merges two originals, borrows from another employer, or carries a number its
source never had is now unrepresentable rather than detectable. This is also what lets the
pre-merge check be exact: it compares a rewording against its named source, where it used to
compare against every bullet the company ever had.

**Metrics must trace to the bullet that carries them.** The highlight-drift check
(`lcsCovered`) counts word overlap, and its word set drops tokens shorter than three
characters — so `40%` reduced to `40` and was discarded before the comparison. A model could
take a real bullet, attach a metric the candidate never claimed, and pass. Every number a
highlight asserts is now checked against the master bullets it may draw from, at **every**
grounding level: an invented number is not a stylistic liberty a looser level tolerates.
Digits inside an identifier (`S3`, `p95`, `EC2`) name a technology rather than assert a
quantity and are skipped. The check runs pre-merge on the selection payload and post-merge on
the document, and covers project highlights as well as experience.

### 2b. Ranking, not rewording — the workspace path (042)

042 finished the move §2a started. On the workspace path the model does not write a bullet at
all: it returns **indices**. `RankedSelection` (`generation/domain/ranking.go`) is what
`llm.CompleteStructured[T]` unmarshals into, and it has no text field anywhere:

```go
type RankedExperience struct {
    Company string `json:"company"`   // copied EXACTLY from the master
    Ranking []int  `json:"ranking"`   // the K bullet indices, most relevant first
}
type RankedSkills struct { GroupOrder []int }   // between-group order only
type RankedSelection struct {
    Experience []RankedExperience
    Projects   []RankedProject
    Skills     RankedSkills
}
```

| Absent field | Would have permitted |
|---|---|
| `rephrased` / any string under an entry | A bullet that merges two originals, borrows another employer's, or attaches a metric nobody claimed (042-FR-009) |
| `summary` | A ranking pass silently rewriting the premium-written summary (035-FR-010, preserved) |
| `suggestions` | A fabricated bullet landing in the profile-sourced group (042-FR-016) |
| any `label`/`details` on skills | The silent skill deletion §2a records; group *contents* still come from the master, ordered by `RankSkills` |
| `drop`/`sectionsToDrop`/`experienceOrder` | The 028 structural violations, still unrepresentable |

This is the third application of the same principle — 028 deleted `SectionsToDrop`,
`ExperienceOrder` and `Drop`; 035 deleted the summary field from `TailoredSelection`; 042
deleted `rephrased`, the last free-text channel that still reached the profile-sourced group.
**Make the violation unrepresentable rather than detectable.** Three checks collapse as a
consequence: `lcsCovered` drift, `ungroundedMetrics` and `StripUngroundedHighlights` have
nothing to catch against an index.

**K, and what makes a ranking invalid.** For an entry with `A` available master bullets and
target `N` (`cfg.ExperienceBulletsMin`, default 8), the prompt asks for exactly
`K = min(2N, A)` distinct indices. `VerifyRanking(available, target, ranking)` returns
structural violations only — `out_of_range`, `duplicate`, `short` — checkable in O(K) with no
reference to what "relevant" means, which is what makes it a verifier rather than a judge.
`len(ranking) > K` is deliberately **not** a violation: rejecting a model that ranked *more*
material than asked would be a rejection with no user-visible defect behind it.

`K = min(2N, A)` is what makes 042-FR-007 ("present up to 2N") and 042-FR-010 ("an omitting
ranking is invalid") consistent. A full permutation of all `A` bullets would cost superlinearly
on exactly the profiles that need the feature most, to order material below the fold; letting
the model choose how many to return would make "omitted" unfalsifiable — the failure 035's
completeness gate exists for.

**Recovery is bounded (042-FR-010).** A violation retries the stage once; a second failure
falls back to **master order** for that entry (`MasterOrderRanking`) and sets
`generation_sections.fallback_used = true`. A lossy list never reaches the user, and 042-SC-007
("rejected rankings in under 5% of runs") is measured from that column, not estimated.

**Skill groups use the same verifier at a different K.** `VerifySkillGroupOrder(groupCount,
order)` calls `VerifyRanking(groupCount, groupCount, order)`, which collapses `min(2*target,
available)` to `available` — so `short` means "omitted a group" here while it means "ranked
fewer than 2N candidates" on an achievement ranking. A group is never dropped over a bad
ranking: order and membership are separate concerns, and the seeding emits every group whatever
the order says.

**What a grounding level still governs here.** With no rewording, `strict`/`moderate`/
`aggressive` have nothing left to say about bullets — under all three, a profile-sourced bullet
*is* the master's bullet. On the workspace route the level therefore governs **the summary
only**, which remains written prose. The UI must say so rather than implying the old behaviour.
`POST /api/documents/tailor` keeps today's semantics for as long as it exists.

## 3. Structural invariants (028)

028 answers a failure the 020 allow-list did not close: the model can leave every field
individually plausible while rearranging the document into a different résumé.

| # | Invariant |
|---|---|
| 028-FR-001 | A canonical block sequence is defined: name → personal info → summary → experience → skills → education. |
| 028-FR-002 | Blocks are never added, removed, renamed or reordered by tailoring. The tailored block sequence equals the master's. |
| 028-FR-010 | When the master's blocks are **not** in canonical order, the master's authored order wins. The canonical sequence is the default, not an override. |
| 028-FR-003 | Experience entries are never reordered, added or removed within the experience block. Order and identity are the master's. |
| 028-FR-009 | Experience entry dates are never altered — changing a date implicitly changes derivable total experience. |
| 028-FR-004 | Total years of experience — stated explicitly *or* derivable from the entries — is never altered. |
| 028-FR-007 | Summary or bullet text asserting a total-years figure that contradicts the master is detected and suppressed. |

**Enforcement, not review** (028-FR-005): a proposal violating FR-002, FR-003 or FR-004 is
**discarded** — marked `dropped` with a structural reason — and is never surfaced to the user
for acceptance (028-SC-004: 100% dropped automatically). 028-FR-011 requires dropped
proposals to appear in the same diff/review surface, so the user sees that something was
rejected and why.

028-FR-008: these guardrails apply to **every** run, including re-runs on an already-tailored
baseline. See § 4.

031-FR-019: the projects section sits where the master profile places it, consistent with
section-order preservation. Section order for projects, certifications and publications is
pinned. There is no links section; a résumé's contact URL lives in personal info, not a
separate block.

### 3.1 How the invariants are enforced

**The design principle: make violations unrepresentable rather than detectable.** For the
structural invariants, 028 did not add a checker — it deleted the fields through which the
model could have expressed a violation, and the merge code paths that would have applied one.

| Removed | Was used for |
|---|---|
| `TailoredSections.SectionsToDrop` | Deleting section keys during merge |
| `TailoredSections.ExperienceOrder` | Reordering experience entries |
| `TailoredExperience.Drop` | Dropping individual jobs |

`MergeTailored` now mutates section *contents* only — summary text, skill `details`,
experience `highlights` — and never writes or removes a section key or the
`cv.sections["_order"]` key. `sections["experience"]` is rewritten from the master-order
slice with only per-entry `highlights` changed; `company`, `position`, `start_date`,
`end_date`, `date`, `location` and `summary` pass through verbatim from the deep-cloned
master. **Invariants 1 and 2 are therefore impossible to violate after merge**: no field
expresses the mutation and no code path applies it.

Because dates were already outside the allow-list and pass through untouched, the *derivable*
total years of experience is unchanged by construction. The one genuinely new check is the
**text-asserted years** case: `VerifyStructureIntegrity` scans the merged summary and every
experience highlight for numeric years-of-experience assertions ("over N years", "N+ years",
"N years of experience") via a bounded regex set, parses N, and compares it against the
master's derivable total — the sum of per-entry `endYear − startYear`, with "Present"
resolving to the current year and an unparseable date conservatively contributing 0.

Escalation on a contradiction:

1. **First detection** — one targeted re-prompt feeding the violation back in ("the summary
   asserts '12 years' but the master's experience spans 5 years; remove any numeric years
   claim"). This is a single extra LLM call, distinct from the two-attempt grounding loop.
2. **Recurrence after the re-prompt** — strip the offending sentence or clause and log the
   intervention on the activity row. **A resume with a contradicting figure is never emitted.**
3. **No figure asserted** — no violation. The check flags a *contradiction*, never the
   absence of a claim; "senior backend engineer" with no number is fine.

**The LLM payload after 028.** `TailoredSections` carries `summary` (2–3
sentences, **must not assert a numeric total-years figure** — describe seniority
descriptively) and
`experience` (one entry per master entry, keyed by exact `company` name, with only
`highlights` — the top 3–5 most relevant, rephrased — changeable). The prompt's hard rules in
`buildSelectPrompt` say so explicitly: keep every experience entry, keep them in the exact
master order, and do not drop, add, rename or reorder any section.

**Surfacing — how 028-FR-011 is actually satisfied.** Structural enforcements are *not*
surfaced as accept/reject proposals, because offering a non-negotiable invariant as a choice
would contradict 028-FR-005. Block-sequence, experience-order and job-drop enforcement is
silent — nothing is "dropped" because nothing the merge would honour was ever attempted.
Text-asserted-years interventions are logged on the activity row for audit, not shown as
choices. FR-011 is met by the **diff view reflecting the result** — blocks in master order,
no dropped jobs, no inflated years — rather than by emitting rows the user must act on. A
visible "we kept your original order" annotation, if ever wanted, is a separate lightweight
mechanism and must not overload the proposal accept/reject lifecycle.

## 4. Review and acceptance (020-FR-004..006, 010, 011)

- **No AI edit is applied without explicit user accept/reject** (020-FR-004). Nothing is
  committed silently.
- Granularity is field-level (020-FR-005): the summary as a whole, each individual
  work-experience bullet, each skill group.
- Rejecting an edit restores that field to its value in the current baseline — master plus
  previously accepted edits (020-FR-006).
- A diff view against the baseline shows exactly what changed before acceptance
  (020-FR-011). Target: review and decide in under 90 seconds for a one-page resume
  (020-SC-004).
- Re-running tailoring on an already-tailored resume treats the current state — master +
  accepted edits + manual edits — as the new baseline (020-FR-010).

020-FR-009 restates Constitution I in this context: a tailoring run updates a local draft and
nothing else. It never submits an application or contacts an employer.

### 4.1 The review surface, as it actually shipped (042)

The `/api/tailoring` draft/proposal surface described in earlier revisions of this section was
never built: `internal/tailoring/` held only field validators (`doc.go`, `proposals.go`), with
no handler, no service and no route registration, and `GeneratePayload.TailoringDraftID` was
declared and read nowhere. Migration `00036`'s `tailored_drafts`/`edit_proposals` tables were
never written to by any caller and were dropped in `00043`. 042 (the resume generation
workspace) supersedes that design with a different review model — a ranked-item list rather
than an accept/reject diff. Shipping both would have left the repository with two review
surfaces and one user, so `internal/tailoring/`, `dto/tailoring.go` and
`queue.GeneratePayload.TailoringDraftID` were removed with it.

`internal/generation/interfaces/http/generations.go`, mounted under `/v1` (and unversioned)
through the same `httpapi.NewRouter` registration every other feature uses. Auth, CORS and
requestId middleware are inherited; errors use the standard `{error, path?, message?}` shape;
dates are ISO-8601 UTC.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/generations` | Start a run — persists the run row, seeds items in master order, enqueues the background pipeline, returns `202 {runId, activityId}` |
| `GET` | `/v1/generations/{runId}` | The whole run: sections and items, in `position` order |
| `GET` | `/v1/generations?jobId=&limit=` | Recent runs, newest first |
| `PATCH` | `/v1/generations/{runId}/items/{itemId}` | Toggle `selected`, move `position`, or edit `text` (AI items only) |
| `PATCH` | `/v1/generations/{runId}/sections/{sectionId}/order` | Whole-section reorder in one call |
| `POST` | `/v1/generations/{runId}/rerun` | `202`; replaces the named sections' items (or the whole run) in place, on the same run id |
| `POST` | `/v1/generations/{runId}/export` | Render-once export: `202` rendering, or `200` with an overflow report |
| `GET` | `/v1/generations/{runId}/export` | Idempotent short-poll for export status |
| `DELETE` | `/v1/generations/{runId}` | `204` |

There is no accept/reject proposal loop: an item is either selected or not, and its position
is either where the ranking put it or where the user dragged it. A profile-sourced item's text
is never a proposal to accept — it is byte-identical to the master bullet at its `sourceIndex`
by construction (§2a), so there is nothing to review about its wording, only whether to
include it.

Status codes that encode real rules:

- `POST /v1/generations` → **400** when neither `jobId` nor `vacancy.text` is given, or the
  profile has no master content. **No 409 for an existing run** — unlike the never-built
  `/api/tailoring` design, concurrent runs against the same vacancy are legal; the workspace
  opens the newest by default.
- `PATCH .../items/{itemId}` → **403** when `text` is sent for an `origin="profile"` item
  (FR-009 at the API boundary); **409** when the run is `running` or the item is `unavailable`.
- `POST .../rerun` → **409** when the run is already `running`.
- `POST .../export` → **409** when the run is `running`, or every section has zero selected
  items.

Every write endpoint takes a row-level `SELECT … FOR UPDATE` on the run first
(`GetRunForUpdate`), matching the discipline 020 specified for drafts. `PATCH .../items/{id}`
is idempotent: re-sending the same body is a no-op landing on the same row.

**Worker wiring.** `POST /v1/generations` enqueues the existing asynq `TypeGenerate` task with
`GenerationRunID` set on `GeneratePayload` — wire-nullable, so old callers leave it nil and the
merged-resume path is unchanged. `generation.Handler.ProcessTask` sees the field and dispatches
to `application.Service.StartRun`, or to `RerunRun` when the payload also carries `IsRerun`.
`generated_documents` rows from a workspace export are indistinguishable from legacy-path rows,
so `GET /api/documents` and the PDF download work unchanged. **The legacy
`POST /api/documents/tailor` merge-and-render endpoint was not removed** — it keeps its own
request shape and today's grounding semantics, and the two paths coexist by design.

**Three response rules the client depends on**, guaranteed by the handler rather than
reconstructed by the UI:

1. Items come back in `position` order, profile-origin and AI-origin **interleaved and tagged**,
   never pre-grouped — an included, repositioned AI bullet must be able to sit between two
   profile bullets.
2. Every master bullet for a rendered entry appears exactly once, selected or not. A client can
   assert this without consulting the profile.
3. `text` for `origin="profile"` is byte-identical to the master's bullet. This is the assertion
   042-SC-001 is measured with.

**Route and layout** (042-FR-001..005): `/generate` in the dashboard, `routeLayoutModes` `fit`
(the two-pane workspace owns the viewport and scrolls its panes independently, like `/tracker`),
wrapped in `RequireProfileConfig` — a run without master content is a 400, and the guard is how
every profile-dependent route avoids that. Entry points are a nav item and a "Tailor for this
job" action on the job detail view. The left pane groups items into Summary, Work Experience
(one block per master entry) and Skills; every item is individually selectable, deselectable and
reorderable **without re-running generation**; and every item shows its origin as a badge.

**Client wiring.** `api.generations` mirrors the existing `settings` group; query keys follow the
established convention (`['generations']`, `['generations', id]`) and mutations invalidate the
run's key. **There is no new polling mechanism**: while `state === 'running'` the run poll reuses
the activity-polling interval every other feature uses.

### 4.2 What the workspace persists, and why (042)

Three tables (migration `00042`), not five: **Section** and **Work Entry Block** are the same
thing at two granularities, so they collapse into one `generation_sections` row per section with
a nullable `entry_key`; **Selection State** is not a table at all but the `selected` + `position`
columns on `generation_items`, because a selection that lives anywhere other than on the item it
selects can drift from it.

| Table | Holds | Rules it carries |
|---|---|---|
| `generation_runs` | vacancy, `master_snapshot`, `master_content_hash`, resolved `shape_config`, grounding level, summary option, analysis, `state`, export status/report | `state='ready'` requires every section ready; `partial` = at least one ready and one failed. **No uniqueness on `(profile_id, job_id)`** — concurrent runs against one vacancy are a comparison, not a conflict |
| `generation_sections` | `kind` (`summary`/`experience`/`skills`), `entry_key`, `position`, `target_count`, `state`, `error`, `fallback_used` | `position` is master order, never model-chosen (028-FR-003); `CHECK (kind <> 'experience' OR entry_key IS NOT NULL)`; 042-SC-007 is measured from `fallback_used` |
| `generation_items` | `origin`, `source_index`, `source_text`, `edited_text`, `rank`, `position`, `selected`, `unavailable` | `CHECK (origin <> 'profile' OR edited_text IS NULL)` — **042-FR-009 as a schema fact**; `UNIQUE (section_id, origin, source_index)` — no master bullet twice |

`entry_key` is the master **company name**, not a synthetic row id: the master is an opaque
`RendercvMaster` map with no stable per-entry identity, `MergeTailored` already keys experience
by `norm(company)`, and the ranking prompt addresses entries the same way. A second identity
scheme for the same thing is a second thing to keep in sync.

Effective text is `COALESCE(edited_text, source_text)`, computed in the domain layer and
**never stored** — a stored copy is a second source of truth that can drift from the edit that
produced it.

**The master is snapshotted, not referenced.** `master_snapshot` holds the whole
`RendercvMaster` the run ranked and `source_text` is copied onto each item at creation. Without
that, 042-FR-022 is unimplementable: the workspace would have no way to render an item whose
source the user just deleted, and would have to drop it silently — the exact behaviour FR-022
forbids. With it, staleness is a hash comparison (`master_content_hash` vs the profile's
current), unavailability is per-item (`MarkItemsUnavailable`), and an export from a stale run
still produces the document the user approved rather than one assembled from a profile they have
since changed. The cost is a jsonb copy per run.

**Rerun replaces ordering, not decisions** (042-FR-021). A rerun deletes and recreates the named
sections' items on **the same run id** — forking would detach the user's selections on the
sections they did not rerun. Explicit decisions are re-applied where the underlying item still
exists: a profile item matched by `source_index`, an AI item by normalised `source_text`, and a
match keeps its `selected`, `position` and `edited_text`. Anything unmatched is gone, which is
what "re-running replaces the AI's ordering for that section" means. The client owns the warning;
the server preserves matches regardless of whether it was shown.

042-FR-023 (auditable provenance) needs no new table: `generation_items.origin` on the run
reachable through `generation_runs.export_document_id` *is* the per-exported-item record, and the
stage/model provenance columns from `00038` still record which model served each stage.

### 4.3 Suggestions are a separate channel, not a looser mode (042)

The AI may propose material the profile does not contain — that is the escape hatch that makes
strict grounding livable — but it arrives through its own type, its own call, and its own badge.

```go
type ExperienceSuggestions struct { Company string; Bullets []string }
type SuggestionSet        struct { Experience []ExperienceSuggestions; Skills []string }
```

**What is absent is the point**: no index field. A suggestion cannot claim to be one of the
user's bullets, which is the mirror image of `RankedSelection`'s missing text field. Together the
two types make the profile/AI distinction a property of the wire format rather than of a label
the UI applies afterwards.

| # | Rule |
|---|---|
| 042-FR-012 | Suggested bullets per work entry and suggested skills, derived from the vacancy, in groups separate from profile-sourced items. |
| 042-FR-013 | **Unselected by default**, always. Inclusion requires an explicit user action. |
| 042-FR-014 | Marked AI-written and unverified wherever they appear — **including after inclusion**. |
| 042-FR-015 | An included suggestion's text is editable before export; the edit is what exports. |
| 042-FR-016 | Suggestions are **not** subject to the grounding rules that bind profile-sourced items — running a grounding check on content defined as absent from the profile would be incoherent — but they are never presented as the user's own material. |
| 042-FR-017 | A suggestion duplicating an existing profile item is suppressed from the suggestion group. The profile item is untouched; only the suggestion disappears. |

The suggestion stage runs on the **existing `generation-select` task key**, concurrently with the
summary stage. No new gateway model group: a suggestion is unverified by definition and
unselected by default, so the stage where a premium model earns its price is the summary, not
this one.

It is **not shown the master's bullet text or skill tokens** — only the vacancy analysis, company
names and skill group labels. A model not shown the material cannot paraphrase it, which reduces
the FR-017 duplicate case at its source rather than catching it afterwards. What survives is
suppressed deterministically in the domain layer (`domain/suggestions.go`): a bullet whose
normalised form matches a master bullet for the same entry, or whose word-set containment against
one is ≥0.9, is dropped; a suggested skill whose normalised token matches any master skill token
is dropped. `norm()` and `tokens()` are the comparison basis everywhere else in this pipeline, so
the same input suppresses the same way on every machine — which the 038 corpus requires. An
embedding-similarity threshold was rejected for exactly that reason: non-deterministic across
model versions, for a check whose false-negative cost is one redundant suggestion.

Survivors become `generation_items` with `origin='ai'`, `selected=false`, ranked after every
profile item in their section. **Zero suggestions is a normal outcome** and renders an empty
state, not an error.

The summary is the one AI-written item that is *not* badged "unverified suggestion": it is
grounded output subject to the existing summary grounding checks, selected by default and
editable. Different thing, different badge.

## 5. Configurable shape (031, 032)

`Settings → Resume shape`, `PUT /v1/settings/resume-shape`, DTO
`dto.ResumeShapeConfigDto` (`apps/api/internal/dto/settings.go`).

**Governing rules**

| # | Rule |
|---|---|
| 031-FR-001/002 | Readable and updatable without a code change or restart; persists across restarts; applies to generations **started after** the change. A run already in flight finishes with the settings it started with. |
| 031-FR-003, 032-FR-010 | Every value has a documented default, and the defaults reproduce pre-settings behaviour exactly (031-SC-002, 032-SC-004). Leaving the card alone changes nothing. |
| 031-FR-004, 032-FR-008 | Validation is **all-or-nothing**: an out-of-range value rejects the whole update and stores none of it. 032-FR-009 additionally rejects a minimum greater than a configured maximum. |
| 031-FR-005 | Reset to documented defaults in one action. |
| 031-FR-006 | Each generated resume records the configuration it was produced with, so a past result stays explainable after settings change. |
| 031-SC-009, 032-FR-012 | Every configurable value, its current setting and its allowed range are discoverable from one place, through one interface. |

**Settings**

`0` means *unlimited / no limit* for `skillsMaxGroups`, `projectsMin`, `projectsMax`,
`projectBulletsMax`, `certificationsMin`, `certificationsMax`.

| Setting | Default | Range | Effect | Spec |
|---|---|---|---|---|
| `summaryLines` | 4 | 1–12 | Approximate summary length in sentences | 031-FR-007 |
| `skillsEnabled` | true | — | `false` removes the skills section entirely | 031-FR-009 |
| `skillsMaxGroups` | 0 | 0–20 | Skill groups kept; `0` keeps all | 031-FR-008 |
| `experienceBulletsMin` | 8 | 1–10 | Target floor of bullets per job | 031-FR-010 |
| `experienceBulletsMax` | 10 | 1–10 | Hard cap of bullets per job | 031-FR-010 |
| `targetPages` | 2 | 1–3 | Page count the render loop aims for | 031-FR-011 |
| `projectsEnabled` | true | — | `false` removes the projects section entirely | 031-FR-012 |
| `projectsMin` | 0 | 0–20 | Target floor of projects; `0` = no minimum | 031-FR-012 |
| `projectsMax` | 0 | 0–20 | Hard cap on projects; `0` includes all | 031-FR-012 |
| `projectBulletsMax` | 0 | 0–10 | Hard cap of bullets per project; `0` keeps all | 031-FR-013 |
| `certificationsEnabled` | true | — | `false` removes the certifications section entirely | 032-FR-001, 032-FR-004 |
| `certificationsMin` | 0 | 0–20 | Target floor of certifications; `0` = no minimum | 032-FR-003 |
| `certificationsMax` | 0 | 0–20 | Hard cap on certifications; `0` includes all | 032-FR-002, 032-FR-005 |
| `fontSize` | 10 | 8–14 | Body text size in points; name scales to 3x body, headline and connections match body | — |

**Minima are targets; maxima are guarantees.**

- 031-FR-014: the model is *steered* toward configured lengths — approximate where an
  approximate match reads better. Bars: summaries within ±1 line 80% of the time (031-SC-004),
  ≥90% of experience entries inside the bullet range when the profile has the content
  (031-SC-003).
- Maxima are enforced **deterministically after the model responds**, so they always hold
  (031-SC-007, 032-SC-002). 032-FR-015: when a certifications maximum truncates, the retained
  set is chosen by the rule 032 specifies, not arbitrarily.

**Page-target loop**

- 031-FR-015: the lengthen/condense loop drives toward the configured page count, not a
  fixed one.
- 031-FR-016: when the page target conflicts with configured section lengths, **the page
  target wins**, and the run records that it did.
- 031-FR-021: when adjustment attempts are exhausted, return the best result achieved rather
  than failing, and report the final page count and reason (031-SC-005: page target hit ≥80%
  of the time, every miss explained).

**Disabling a section** (031-FR-020, 032-FR-004) is not a structural or grounding violation.
All other structure and grounding checks continue to apply unchanged. 031-SC-006: disabling
removes the section from 100% of subsequent generations with no other section's content
lost.

**Section positions are fixed** (032-FR-013): certifications keep their established position
in the enforced order regardless of configuration.

**Explicitly not in scope** (032-FR-016): no per-certification detail-line cap.

### 5.1 The endpoint

Mounted by `internal/resumeshape/interfaces/http/resume_shape.go` beside the existing
`/v1/settings/ai-features` routes. `ResumeShapeConfigDto` is the body for **both** request and
response on every method, tygo-generated into `packages/shared/src/generated.ts` and never
hand-edited (Constitution III).

| Method | Behaviour |
|---|---|
| `GET /v1/settings/resume-shape` | `200` with the current config, served from the in-memory cache. **Never 404s** — the singleton row is seeded by migration `00034`, and the service falls back to `DefaultShapeConfig()` if the row is somehow absent. |
| `PUT /v1/settings/resume-shape` | Replaces the **whole** config — a full-payload replacement, not a patch, because 031-FR-004's all-or-nothing validation requires every field to be present. `200` with the persisted values; `400` naming the offending field and its range; `500` on a persistence failure. |
| `DELETE /v1/settings/resume-shape` | `200` with the documented defaults. Idempotent — deleting an already-default config returns the same body. |

**Ordering guarantee**: validation runs before any write. On a `400`, nothing is stored *and
the in-memory cache is untouched*, so a following `GET` returns the pre-request values. The
new config applies to every generation **started after** the response; a generation already in
flight completes with the config it resolved at its start.

Validation runs twice by design — in the handler, so a bad body produces a `400` rather than a
`500`, and in the service, so the write is atomic. Both call the same
`domain.ShapeConfig.Validate()`, so a new rule takes effect in both places from one edit.

Error messages are the contract, since clients surface `error` verbatim:

| Condition | Message |
|---|---|
| `targetPages` outside 1–3 | `targetPages must be between 1 and 3` |
| `experienceBulletsMin > experienceBulletsMax` | `experienceBulletsMin must be <= experienceBulletsMax` |
| `projectsMin > 0` with `projectsEnabled: false` | `projectsMin > 0 requires projectsEnabled` |
| `certificationsMin` / `certificationsMax` outside 0–20 | `certificationsMin must be between 0 and 20` (likewise `certificationsMax`) |
| `certificationsMin > certificationsMax` (when max > 0) | `certificationsMin must be <= certificationsMax` |
| `certificationsMin > 0` with `certificationsEnabled: false` | `certificationsMin > 0 requires certificationsEnabled` |

The certifications messages mirror the projects forms exactly, so 032 needed no client change.

Dashboard client: `settings.getResumeShape()`, `settings.putResumeShape(body)`,
`settings.resetResumeShape()` in `lib/api.ts`; query keys `resumeShape.all = ['resumeShape']`
and `resumeShape.get = ['resumeShape', 'get']`, with mutations invalidating `resumeShape.all`,
matching the `aiFeatures` pattern.

> ### ⚠ The whole-config PUT hazard
>
> Go decodes a missing boolean as `false`. An external or scripted client that PUTs a
> hand-written body omitting `certificationsEnabled` therefore **silently disables the
> certifications section** — and the same is true of `skillsEnabled` and `projectsEnabled`.
>
> This was considered and accepted during 032: the dashboard always round-trips the full
> config it received from `GET`, so it is unaffected, and making certifications behave
> differently from the other two toggles on the same endpoint would be worse than the hazard.
> The alternative — decoding into a pointer-field struct and treating an absent boolean as
> "keep current" — remains available if a scripted client ever gets burned, but it must then
> be applied to all three toggles at once.

Changing the DTO is a four-step sequence: edit `internal/dto/settings.go` → `make
tygo-generate` → `pnpm --filter @job-finder/shared build` → dashboard typechecks. `make
tygo-check` fails CI if the committed generated file drifts.

## 6. Projects section (031-FR-018)

With projects enabled, each project reproduces its name, link and dates from the master
profile. 031-SC-007: 100% of generated resumes contain a project count inside the configured
range, or all available projects when fewer exist.

## 7. PDF output (020-FR-007, 008, 013, 014)

- 020-FR-007: exports render as **searchable/selectable text**, never rasterised, within a
  bounded density range. The 020 spec required exactly one page; 031-FR-011 generalised this
  to a configurable `targetPages` (1–3) and 031-FR-021 replaced hard failure with
  best-effort-plus-report. **Where 020 and 031 disagree, 031 governs.**
- 020-FR-008: content that cannot fit even at minimum density blocks the export with an
  actionable message — never a truncated or silently multi-page document (020-SC-006).
- 020-FR-012: a job posting with insufficient signal degrades gracefully (e.g. summary-only
  polish) and says so, rather than inventing relevance.
- 020-FR-013: a tailoring run completes in under 60 s on average against the local model
  (020-SC-007: ≥90% of runs), with an indeterminate progress indicator meanwhile.
- 020-FR-014: an unreachable local model, a timeout, or malformed model output surfaces a
  clear error — never a partial or corrupted resume.

### 7.1 The page fitter, as it actually shipped

The chromedp density-ladder fitter earlier revisions of this section described —
`internal/generation/singlepage`, a 9-step ladder over CSS custom properties, ranked
blocked-feedback by re-measuring each candidate's removal — was never built. The package
contains only `doc.go`. Page measurement and fitting are `RenderCvRenderer`
(`internal/generation/infrastructure`), which shells out to the Typst-based `rendercv`
binary (`RENDERCV_BIN`) rather than driving a headless Chrome instance, and `CountPages`
measures the rendered PDF directly instead of estimating content height in CSS pixels.

Two fitting strategies exist side by side, because two different rules govern what each may
change:

- **The legacy job-scoped and ad-hoc paths** (`/api/jobs/{id}/generate`,
  `POST /api/documents/tailor`) use `Service.renderToPageTarget` (`service.go`): render, and if
  over target, `ApplyFontSize` down a step, then `CompactDesign` (layout only), then
  `expandContent` (a model call, when under target) or `TrimHighlights`/`condense` (silently
  drops bullets from the end of each entry) when still over. This path may reword and may
  drop content to make the page count.
- **The 042 workspace export path** (`POST /v1/generations/{runId}/export`,
  `internal/generation/application/workspace_export.go`) is render-once and never
  reword-or-drop — **042-FR-018** forbids post-selection rewording, condensing or re-ranking,
  and **042-FR-019** requires overflow to be *reported with candidates* rather than resolved: `domain.Assemble` builds a `RendercvMaster` from
  exactly the selected items (no model call), `ApplyFontSize` then `RenderCvRenderer.Render`
  then `CountPages`; over target, one `CompactDesign` re-render (typography only — the exported
  words are unchanged); still over, the export returns `status: "blocked"` with an overflow
  report — pages rendered vs. target, and the lowest-ranked **selected** items as named drop
  candidates the user acts on. `expandContent`, `TrimHighlights`, `padHighlights` and the
  `ApplyHardLimits` truncation never run on this path; `generation_items.rank` already orders
  each section by relevance, so "worst-ranked first" is a query over persisted rank, not a
  re-measured search.

Every export, on either path, produces searchable/selectable text (020-FR-007): RenderCV's
Typst output has no rasterised text and no image-of-text. Page size and margins come from the
resolved `ShapeConfig` (§5), not a fixed density ladder.

Three deliberate non-goals: it does **not** render the user's RenderCV Typst theme — the point
of a tailored resume is to deviate from the master theme; it does **not** fall back to
`RenderCvRenderer`; and it does **not** accept the master's opaque `RendercvMaster` map, only
the structured `dto.Resume`.

## 8. Measurable bars

**Structure preservation (028)** — all four structural criteria are 100% bars, because the
enforcement is mechanical rather than statistical:

- 028-SC-001: 100% of tailored resumes present blocks in master order — zero additions,
  removals, renames or reorders across all runs.
- 028-SC-002: 100% list experience entries in master order — zero additions, removals or
  reorders.
- 028-SC-003: 100% carry the same total years as their master — zero changes to an explicit
  figure and zero changes to date ranges that would alter a computed total.
- 028-SC-005: zero accepted edits introduce or imply a total-years figure that exceeds or
  contradicts the master's, auditable through the same grounding trail as 020-SC-005.
- 028-SC-006 is the human-facing counterpart: a user can confirm **by eye, within 30 seconds**,
  that block order, job order and total experience match their master. The invariants must be
  *visibly* preserved, not only silently enforced.

**Configurable shape (031, 032)**

- 032-SC-001: disabling certifications removes them from the next generated resume **with no
  other section's content or order changed.**
- 032-SC-002: with a maximum of N configured and more than N in the profile, 100% of generated
  resumes contain exactly N.
- 032-SC-004: with default settings, generated resumes are identical to those produced before
  the feature, for the same profile, vacancy and model output.
- 032-SC-005: any setting can be changed, saved and confirmed persisted in under a minute.
- 032-SC-006: 100% of out-of-range or contradictory updates are rejected **without altering any
  stored setting.**
- 032-SC-007: a minimum the profile cannot meet produces a resume **plus a recorded shortfall**
  in 100% of such runs — never a failed generation, and never an invented certification.

**The workspace (042)** — the first four are structural, so they are 100%/0% bars rather than
targets:

- 042-SC-001: 100% of profile-sourced items in an exported resume match the master text
  exactly. Nothing fabricated can appear without an explicit user inclusion.
- 042-SC-003: for an entry with N target bullets and ≥2N available, at least 2N ranked
  candidates are shown, of which exactly N are pre-selected.
- 042-SC-004: **0%** of AI-suggested items appear in an export where the user took no action on
  them.
- 042-SC-007: rejected/retried rankings occur in under 5% of runs — measured from
  `generation_sections.fallback_used` and the `ranking_violations` scorer — and no such run
  reaches the user with a lossy list.
- 042-SC-002: pasted vacancy → approved, exported resume in under 3 minutes including review.
- 042-SC-005: a user identifies any item's origin (theirs vs AI-written) without opening a
  detail view, at 90% correct identification.
- 042-SC-006: adjusting a selection updates the preview with **no additional model run** and
  under 1 second of delay. This is a property of the design, not of tuning: selection state
  lives on the item rows and export assembles from them, so nothing about a toggle can reach a
  model.

## The evaluation harness (038)

Resume quality had no regression gate: every check in the pipeline verified one run against
itself, and nothing compared today's output to what the same input produced last week. The
harness is that comparison, and it runs on every change.

### Two modes, and only one of them gates

**Deterministic mode** is the gate. `TestEvalCorpus` runs in the ordinary suite — no build tag,
no environment variable, no credentials, no network, no database, and no PDF toolchain. Every
model response comes from a committed fixture keyed by a hash over the request, so the same
input produces the same scores on any machine. It fails the build on a regression, on an
improvement, and on a case with no baseline.

**Live mode** is behind `//go:build eval_live` and reports rather than gates. It runs the corpus
against several candidate task keys and writes a durable comparison artifact — per model, per
case, per stage, with cost, latency, served model, substitution and escalation flags, the
request parameters and the corpus revision. That artifact is what a model swap is decided on;
`gateway/config.yaml` points at it rather than carrying figures that drift.

Live mode never writes a baseline. A live run's scores depend on which upstream answered that
day, and gating future changes on that would make the weather a build failure.

### What the gate does not measure

**The PDF renderer.** `render` and `countPages` are stubbed: a case declares a `page_counts`
sequence and the stub returns it in order, which is what makes the page-fit loop deterministic
and runnable with no Python and no Typst installed. The *LLM* half of page fitting is measured —
`expand` and `condense` keep their production implementations and go through the replay
provider — but nothing here proves a PDF comes out. That stays covered by the infrastructure
tests and by live mode.

Two measures specified for the harness are also absent, deliberately: `json_parse_failures`
(the structured retry loop discards its attempt count) and `empty_output` (no zero-content
check exists in the domain). Adding either would mean writing the production instrumentation
here, which would make the harness the author of a quality rule it then grades. They return as
scorers once that instrumentation lands in production on its own justification.

### The seven scorers

Every scorer delegates to a check production already enforces, so a green harness means
something about production rather than about the harness. There is no LLM judge.

| Scorer | Delegates to | Direction |
|---|---|---|
| `grounding_violations` | `VerifyRendercvGrounding` | lower |
| `structural_violations` | `VerifyStructureIntegrity` | lower |
| `highlight_drift` | `VerifyHighlightGrounding` | lower |
| `required_skills_missing` | `CompletenessReport.RequiredMissing` | lower |
| `nice_to_have_retention` | `CompletenessReport.NiceToHaveRetained` | higher |
| `bullet_shortfalls` | `CompletenessReport.BulletShortfalls` | lower |
| `ranking_violations` (042) | `VerifyRanking` | lower |

`grounding_violations` and `highlight_drift` move on the same defect — the grounding verifier
performs the drift comparison inline — so the comparator reports a co-moving pair as **one
defect seen by two instruments** and never sums scores.

Two committed tripwires keep this honest: `TestScorerDelegationIsExact` calls each named domain
function independently and asserts equality, and `TestScorersDetectInjectedDefects` injects a
known defect and asserts the relevant scorer moves the wrong way. Without them, "scorers must
delegate" is a rule with no detector.

042 added the seventh scorer and two cases with it, per the standing rule below:
`ranked-oversized-entry` (an entry with far more than 2N bullets — the baseline asserts K
distinct in-range indices and zero violations) and `suggestion-duplicates-profile` (a vacancy
whose obvious suggestions restate master bullets — the baseline asserts the suppression fires).
Adding a scorer changes the scorer set, so `ScorerSetVersion` bumped and **every** baseline was
re-recorded with a stated reason. The harness refusing to compare across scorer sets is the
designed behaviour, not an obstacle: a delta measured across two instruments is not a quality
signal.

### Adding a case

Add a directory under `internal/generation/application/evaldata/cases/` with `case.yaml`,
`master.yaml` and `vacancy.txt`. Cases are discovered by walking that directory — no case name
appears in any Go file — so a case cannot be added without the gate running it, or quietly
dropped from a list.

`case.yaml` requires a `why` naming a concrete failure mode, and a `page_counts` sequence. Every
fixture must be **synthetic** and must use **closed date ranges only**: a role ending in
`present` makes the derived experience figure, and therefore every replay fixture for that case,
change on 1 January.

Then record the fixtures and the baseline:

```bash
go test -tags eval_live ./internal/generation/application/ \
  -run TestEvalRecord -eval.record -eval.case <name>
go test ./internal/generation/application/ -run TestEvalCorpus \
  -eval.update-baseline -eval.case <name> -eval.reason "initial baseline"
```

Fixtures are recorder-produced. Hand-editing one turns the corpus from a record of what a model
did into a record of what somebody wished it had done.

### Updating a baseline

Never automatically, and never for the whole corpus at once:

```bash
go test ./internal/generation/application/ -run TestEvalCorpus \
  -eval.update-baseline -eval.case <name> -eval.reason "<what changed and why>"
```

The reason is required and is written into the baseline file, so the diff a reviewer sees says
what moved and why. A baseline recorded under a different `ScorerSetVersion` is refused rather
than compared — a delta measured across two instruments is not a quality signal.

### The standing rule

**A production failure fixed from now on arrives with a corpus case that would have caught it,
in the same change.** A fix without a case is a fix that can regress silently, and the corpus
exists precisely so that the second occurrence of a failure is impossible rather than merely
unlikely. This is the rule the corpus is built on: every case in it names a failure this
repository actually recorded.

## The summary model choice (034)

The user picks who writes the professional summary. **Only** the summary — not the whole pipeline,
and not the other four stages.

That restraint is the design. 035 measured the economy model doing the mechanical stages (analyze,
select) as well as the premium one at a fraction of the price, and failing at the summary. Offering
a choice over the mechanical stages would sell the user a way to spend more money for no
improvement; offering it over the summary hands them the one lever that changes what they get. The
original 034 spec asked for a choice of "the model that writes my resume", which stopped meaning
anything the moment 035 split the pipeline — see the standing note at the top of that spec.

**The catalogue** lives in `internal/generation/domain/summary_option.go`: `standard`, `premium`,
`fast` and `local`. Four entries, one of them self-hosted and always available. It is Go rather than
a database table because an option is a gateway task key plus prose, and the task key has to exist
in `gateway/config.yaml` — a deployment artifact reviewed in code. A row in a table could name a key
the gateway has never heard of, and that failure is silent: no such group, the call falls through,
and a user who deliberately picked "Premium" gets whatever the terminal tier is while the UI happily
reports success. `apps/api/internal/summarycatalogue_test.go` is what stops that, asserting every
option's key is a declared and chained model group.

**The default routes to the pre-034 task key**, `generation-summary`, unchanged. A user who never
opens the selector therefore sends exactly the request they sent before the feature existed — true
by construction rather than by vigilance. Giving the default its own key would have made that
property depend on two chains being kept in sync forever.

**Adding an option** is three steps: an entry in the catalogue, a task key with a fallback chain
terminating at `local` in `gateway/config.yaml`, and that key added to `requestedGenerationGroups`
in `gateway_config_test.go`. The routers wire themselves from the catalogue.

**How the choice is resolved.** Once, at the top of a run, beside the shape config and for the same
reason: a settings change while a run is in flight must not swap the model writing the summary
halfway through and produce a document nobody chose. A per-run choice on the request wins over the
stored default, and choosing also persists — picking and remembering are one action, not two.

**An unconfigured or unknown option degrades, it does not fail.** An option is a routing preference,
and a preference that can fail a resume run is a liability. The write path is stricter than the read
path on purpose: an unknown id sent to `PUT /v1/settings/summary-model` is a 400, because the client
picked it from a menu this same API served, whereas an unknown id *read back* from storage is an
option retired between releases and resolves to the default.

**What this does not do is measure the options.** The catalogue's cost indicators are relative words
— "moderate", "highest" — not prices, because a figure written here would be wrong within a month
and could not be reproduced by the next reader. Deciding whether `premium` is worth its cost is
038's job, and 038 answers it with a durable artifact:

```sh
go test -tags eval_live ./internal/generation/application/ \
  -run TestLiveComparison -eval.models generation-summary,generation-summary-premium
```
