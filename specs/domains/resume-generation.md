# Domain: Resume Generation & Tailoring

Consolidates **020** constrained AI resume tailoring, **028** structure preservation,
**031** configurable generation shape, **032** certifications as a configurable category.

Implementation: `apps/api/internal/generation/`, `internal/tailoring/`,
`internal/resumeshape/`. How it works:
[`docs/ai/generation.md`](../../docs/docs/ai/generation.md).

This domain implements Constitution II (Grounded Generation). Every rule below exists
because a resume is used in a real hiring decision, and a fabricated one damages the user in
a way they cannot easily detect or undo.

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
| 028-FR-001 | A canonical block sequence is defined: name → personal info → summary → skills → experience → education. |
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
section-order preservation. Section order for projects, certifications, publications and
links is pinned.

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
| `experienceBulletsMin` | 8 | 1–20 | Target floor of bullets per job | 031-FR-010 |
| `experienceBulletsMax` | 10 | 1–20 | Hard cap of bullets per job | 031-FR-010 |
| `targetPages` | 2 | 1–3 | Page count the render loop aims for | 031-FR-011 |
| `projectsEnabled` | true | — | `false` removes the projects section entirely | 031-FR-012 |
| `projectsMin` | 0 | 0–20 | Target floor of projects; `0` = no minimum | 031-FR-012 |
| `projectsMax` | 0 | 0–20 | Hard cap on projects; `0` includes all | 031-FR-012 |
| `projectBulletsMax` | 0 | 0–10 | Hard cap of bullets per project; `0` keeps all | 031-FR-013 |
| `certificationsEnabled` | true | — | `false` removes the certifications section entirely | 032-FR-001, 032-FR-004 |
| `certificationsMin` | 0 | 0–20 | Target floor of certifications; `0` = no minimum | 032-FR-003 |
| `certificationsMax` | 0 | 0–20 | Hard cap on certifications; `0` includes all | 032-FR-002, 032-FR-005 |

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
