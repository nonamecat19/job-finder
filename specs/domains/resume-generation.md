# Domain: Resume Generation & Tailoring

Consolidates **020** constrained AI resume tailoring, **028** structure preservation,
**031** configurable generation shape, **032** certifications as a configurable category,
**035** split-model generation.

Implementation: `apps/api/internal/generation/`, `internal/tailoring/`,
`internal/resumeshape/`. How it works:
[`docs/ai/generation.md`](../../docs/docs/ai/generation.md).

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
| Selection | `generation-select` | which skills, in what order; which achievements per job; their rewording | grounding + **completeness** |
| Summary | `generation-summary` | writes the 2-4 sentence summary | grounding, independently |
| Page fit | `generation-select` | expand/condense to hit the page target | grounding |

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

Note what is *not* a shortfall: a group the model omits entirely. The merge keys skills by
index and leaves an unmentioned group at its master value, so omission is absorbed. Only
content returned in truncated form reaches the document.

**Provenance (035-FR-012/FR-017).** Each run records, per stage, which model served it,
whether the chain fell back, the duration and the measured cost — the last from the proxy's
own `usage.cost`, not an estimate. A summary served by a fallback is marked on the document
and shown on the review surface: a user who was told they get the strict model must not
silently receive the cheap one. Substitution is detected from the proxy's
`x-litellm-attempted-fallbacks` header, so the application still never learns which upstream
model was configured for a key.

**Cover letters are on demand** (035-FR-013). A tailoring run produces a resume only; the
letter is requested against a finished resume via `POST /documents/{id}/cover-letter`.

---

## 1. What the AI may change (020-FR-001, 028-FR-006, 031-FR-020)

**Allow-list.** AI edits are restricted to:

1. the professional summary,
2. the description bullets under each work-experience entry,
3. skills and skill groups,
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

**The LLM payload after 028.** `TailoredSections` carries exactly three fields: `summary` (2–3
sentences, **must not assert a numeric total-years figure** — describe seniority
descriptively); `skills` (one entry per master skill group at the same `[index]`, with
vacancy-required skills first within each group, group set and order matching master); and
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

### 4.1 The tailoring REST surface

`internal/tailoring/`, mounted under `/api`. Auth, CORS and requestId middleware are inherited
from `httpapi.NewRouter`; errors use the standard `{error, path?, message?}` shape; dates are
ISO-8601 UTC.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/tailoring` | Enqueue a run — creates the draft, enqueues the `generate` job, returns `202 {draftId, activityId}` |
| `GET` | `/api/tailoring/{draftId}` | The draft: state, baseline summary, all proposals (the UI groups by status). **`dropped` proposals are not included in this list.** |
| `POST` | `/api/tailoring/{draftId}/proposals/{proposalId}` | `{action: "accept" \| "reject"}` |
| `POST` | `/api/tailoring/{draftId}/finalize` | `200 {state: "finalized"}`; **precondition: zero `pending` proposals.** The handler never auto-finalizes — the user must click. |
| `POST` | `/api/tailoring/{draftId}/export-pdf` | Starts the page fitter |
| `GET` | `/api/tailoring/{draftId}/export-status` | Idempotent short-poll |
| `POST` | `/api/tailoring/{draftId}/rerun` | `202` with a new draft seeded from the current baseline |
| `DELETE` | `/api/tailoring/{draftId}` | `204`, draft `abandoned` |

The request body carries either a `jobId` or an ad-hoc `vacancy` (`{company, title, text}`)
alongside `profileId`.

Status codes that encode real rules:

- `POST /api/tailoring` → **400** when the profile has no master content, or when a draft is
  already `review`/`finalized` for the same `(profile, job)` — **the caller must use `/rerun`
  instead of starting a second draft.** → **409** when the master's `rendercv_config` checksum
  no longer matches the existing draft's, meaning the master was edited mid-review; the caller
  abandons and starts fresh.
- Accept/reject → **409** when the proposal is already terminal or the draft is
  `finalized`/`abandoned`; → **422** when the proposal is `dropped` (grounding- or
  structure-suppressed). **A dropped proposal can never be accepted.**
- `/export-pdf` → **409** when the draft is not `finalized`, still has pending proposals, or
  already produced a `fit` (the caller fetches the existing `documentId`).

**Accepting mutates the baseline in the same transaction**, so a subsequent poll already
reflects it. Rejecting restores the baseline value for that field and changes no baseline.
Every write endpoint takes a row-level `SELECT … FOR UPDATE` on the draft first
(`GetDraftForUpdate`), and accept/reject is idempotent on terminal rows — a client may retry
after a network flake without double-applying.

`/rerun` leaves the prior draft `finalized` and seeds the new draft's baseline from the
prior's *current* baseline, so **already-accepted edits are not re-surfaced as fresh
proposals.**

**Worker wiring, reused rather than new.** `POST /api/tailoring` enqueues the existing asynq
`TypeGenerate` task with a `tailoringDraftID` field added to `GeneratePayload` — wire-nullable,
so old callers leave it nil and the merged-resume path is unchanged.
`generation.Handler.ProcessTask` sees the field and dispatches to
`tailoring.Service.RunProposals`. The existing `GET /api/activity` routes already carry the
queued/running state the dashboard polls for 020-FR-013's progress indicator, so no new
endpoint was needed. `generated_documents` rows from `/export-pdf` are indistinguishable from
legacy-path rows, so `GET /api/documents` and the PDF download work unchanged. **The legacy
`POST /api/documents/tailor` merge-and-render endpoint was not removed** — 020's flow is
additive, and two export paths coexist by design.

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

### 7.1 The page fitter

`internal/generation/singlepage`. Takes a finalized `dto.Resume` (the 009 structured shape),
a draft id for artifact naming and audit, an output directory (default `/data/documents`) and
an optional `storage.Store` for MinIO upload. Returns a `FitResult` whose `Status` is `fit`,
`blocked` or `error`, carrying the document id and file path on a fit, ranked `feedback` when
blocked, and — for telemetry — the measured content height and the density step that won.

**The density ladder is deterministic and searched cheapest-first**, largest settings to
smallest; the first step that fits wins:

| Step | Body pt | Margin mm | Line height | Bullet gap px |
|---|---|---|---|---|
| 1 | 11.0 | 14 | 1.4 | 4 |
| 2 | 11.0 | 14 | 1.3 | 4 |
| 3 | 11.0 | 14 | 1.3 | 2 |
| 4 | 10.5 | 12 | 1.3 | 2 |
| 5 | 10.5 | 10 | 1.2 | 2 |
| 6 | 10.0 | 10 | 1.2 | 1 |
| 7 | 10.0 | 8 | 1.2 | 1 |
| 8 | 9.5 | 8 | 1.1 | 0 |
| 9 | 9.0 | 8 | 1.1 | 0 — **minimum bound** |

Page size is A4 (210 × 297 mm), fixed; a configurable page size is out of scope. **Anything
below the minimum bound is rejected as `blocked` rather than rendered**, because tighter than
that stops being ATS-readable.

Measurement per step, via chromedp: render the template with the density knobs exposed as CSS
custom properties on `:root`, `SetDocumentContent`, evaluate
`document.documentElement.scrollHeight`/`scrollWidth`, convert px to mm at 96 dpi
(1 px = 0.2645 mm), and fit when the measured height is within `297 − 2×margin` **and** the
width does not overflow. Exhausting the ladder produces `blocked` with computed feedback.

There is a **5 s synchronous budget** per export. Beyond it the remaining steps run as a
background task and `POST /export-pdf` returns `200 {status: "pending"}` for the dashboard to
poll through `/export-status`.

**Blocked feedback is ranked by space saved**, not listed arbitrarily: longest bullets first
(identified as `experience:Acme:3`), then skill-group removals (`skill_group:Cloud`), then the
summary. Each candidate's gain is measured at the minimum density step by removing it and
re-measuring, and the returned array is the **shortest set whose removal achieves fit**.

**The text-PDF guarantee** (020-FR-007) is structural, not incidental: `PrintToPDF` with
`WithPrintBackground(true)`; no `<canvas>`, no image-of-text, no `user-select: none`; real
`<p>` and `<ul>` elements throughout. An integration test opens the produced PDF and asserts
the page count plus successful text extraction of the name, summary and first bullet.

The template renders from `dto.Resume` alone and **does not import the legacy JSON-Resume
`resume_view` struct**. It is ATS-clean by construction: single document flow, no columns, no
flexbox a paper print cannot reproduce, with margins and skill order stable across every
density step. It renders a header, then each `Section` in declared order by `entryType`
(`experience`/`education`/`publication` structured entries; `normal`/`text`/`bullet`/
`numbered`/`reversed_numbered`/`one_line` content entries; and `unrecognized` entries rendered
from their raw fields per the 009 data model), with skill groups as label plus
comma-separated details, one row per group and never stripping tokens.

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
