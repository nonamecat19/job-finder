# Contract: Structural-Integrity Invariants for AI Resume Tailoring

**Feature**: 028 — Strict Resume Structure Preservation During AI Tailoring
**Scope**: the AI tailoring pipeline (`apps/api/internal/generation`), invoked by the `/generate` (job-scoped) and ad-hoc generation paths. This contract binds the LLM output payload (`TailoredSections`), the merge step (`MergeTailored`), and the post-merge verifier (`VerifyStructureIntegrity`).

These are **non-negotiable invariants enforced automatically**. They are not user-accept/reject proposals. A violating AI output is silently ignored (for structural fields) or re-prompted/stripped (for text-asserted years), never surfaced as a choice.

## Invariant 1 — Block sequence is immutable

**Statement**: The AI may not add, remove, rename, or reorder resume blocks (top-level `cv.sections` keys) during tailoring. The tailored resume's block set and order equal the master resume's block set and order exactly.

**Canonical default (for NEW resumes)**: name → personal info (contact/location/links) → summary → skills → experience → education.

**For already-authored masters**: the AI preserves whatever order the user authored via feature 009. The canonical sequence is the default for new resumes, not a force-rewrite rule applied to already-customized masters. Additional user-authored blocks (e.g., projects, certifications, patents) are preserved in their authored positions; the AI may not drop them.

**Enforcement**:
- `TailoredSections.SectionsToDrop` is removed from the struct; `MergeTailored` no longer deletes section keys.
- `MergeTailored` only mutates section *contents* (summary text, skill `details`, experience `highlights`) and never writes/removes a section key or the `cv.sections["_order"]` key.
- Block order is already preserved through tailoring by the untouched `_order` key; this invariant adds *no drops* (previously `SectionsToDrop` defeated the preserved set) and *no add/rename/reorder* (no code path exists for these; the contract makes the absence explicit).

**Violation handling**: impossible to violate after merge — the LLM has no field to express a block mutation and the merge layer has no code path to apply one.

## Invariant 2 — Experience (job) order and identity are preserved

**Statement**: The order and identity of experience entries within the experience block of the tailored resume equal the order and identity in the master resume. The AI may not reorder, add, or drop job entries. Only the description bullets (`highlights`) under each job may change, within feature 020's existing allow-list.

**Enforcement**:
- `TailoredSections.ExperienceOrder` and `TailoredExperience.Drop` are removed from the struct.
- `MergeTailored` no longer applies the reorder block (rendercv.go:318-337) or the `_drop` filter (rendercv.go:310-316). The `kept` slice iterates the master's experience entries in order and, with no `Drop` markers, is identical to master order.
- `sections["experience"]` is rewritten from the master-order experience slice with only per-entry `highlights` changed; `company`, `position`, `start_date`, `end_date`, `date`, `location`, `summary` pass through verbatim from the deep-cloned master.

**Violation handling**: impossible to violate after merge — the LLM has no field to express a reorder or drop and the merge layer has no code path to apply one.

## Invariant 3 — Total years of experience is locked

**Statement**: The AI may not alter the total years of experience stated in or derivable from the master resume. This covers (a) an explicit years-of-experience statement and (b) any figure computed from experience entries' date ranges — the tailored resume's figure equals the master's figure. The AI also may not assert a numeric years-of-experience figure in generated summary or bullet text that contradicts the master's figure.

**Enforcement**:
- **Dates are already outside the AI allow-list**: `MergeTailored` never writes `start_date`/`end_date`/`date`/`company`/`position`/`location` — they pass through verbatim from the deep-cloned master. The derivable total is therefore unchanged by construction. Feature 028 makes this rationale explicit in code comments and the plan.
- **Text-asserted years** (new check): `VerifyStructureIntegrity` scans the merged `cv.sections.summary[0]` and each `cv.sections.experience[].highlights[]` for numeric years-of-experience assertions (e.g. "over N years", "N+ years", "N years of experience") via a bounded regex set, parses the asserted N, and compares against the master's derivable total (sum of per-entry `endYear − startYear`, "Present" → current year, unparseable → 0, conservative).

**Violation handling** (text-asserted years only):
1. **First detection**: one targeted re-prompt, feeding the violation back into the prompt ("the summary asserts '12 years' but the master's experience spans 5 years; remove any numeric years claim"). This is a single additional LLM call, not the existing 2-attempt grounding loop.
2. **Recurrence after re-prompt**: strip the offending sentence/clause from the text and log the intervention on the activity row. Do not emit a resume with a contradicting figure.
3. **No figure asserted**: no violation. The check only flags a *contradiction* with the master's derivable total, not the absence of a claim. A summary that says "senior backend engineer" without a number is fine.

## LLM output payload contract (post-feature-028)

The LLM is asked to produce a `TailoredSections` with exactly these fields:

| Field | Type | Constraint |
|-------|------|-----------|
| `summary` | string | 2-3 sentence professional summary. **Must not assert a numeric total years-of-experience figure** (e.g. "over 8 years"). Describe seniority descriptively. |
| `skills` | `[]TailoredSkillGroup` | One entry per master skill group, same `[index]`, vacancy-required skills first within each group. Group set and order match master. |
| `experience` | `[]TailoredExperience` | One entry per master experience entry, keyed by exact `company` name. Only `highlights` (top 3-5 most relevant, rephrased) may change. **No `drop`, no reordering** — entries appear in master order. |

Removed fields (no longer accepted):
- `sectionsToDrop` — ignored; do not emit.
- `experienceOrder` — ignored; do not emit.
- `experience[].drop` — ignored; do not emit.

## Prompt contract (post-feature-028)

`buildSelectPrompt` (`rendercv_llm.go`) HARD RULES are revised to:

- Keep every experience entry; never set `drop`. Do not omit any job.
- Keep experience entries in the EXACT order shown in the master; do not reorder.
- Do not drop, add, rename, or reorder any resume section. Keep the master's section set and order exactly as given. Do not populate `sectionsToDrop`.
- (Retained) Return skills as one entry per group, using the same `[index]`.
- (Retained) Reorder skills within each group: vacancy-required skills first.
- (Retained) Keep highlights concise, one achievement each, no fabricated numbers.

Summary guidance gains:
- Do not state a total number of years of experience (e.g. "over 8 years"); describe seniority descriptively without a numeric claim.

## Surfacing (audit, not user choice)

Structural enforcements are **not** surfaced as user-accept/reject proposals:
- Block-sequence, experience-order, and job-drop enforcements are silent (the LLM has no field to express them; nothing is "dropped" because nothing was attempted that the merge honored).
- Text-asserted-years interventions (re-prompt, strip) are logged on the activity row for audit, not shown in the diff/review surface as choices.

The spec's FR-011 ("report dropped structural proposals in the diff surface with a non-technical label") is satisfied by the diff view reflecting the *result* (blocks in master order, no dropped jobs, no inflated years) rather than by emitting rows the user must act on. The invariant is non-negotiable; offering it as accept/reject would contradict FR-005.

If feature 020's draft/proposal flow is later implemented and the team wants visible "we kept your original order" annotations, that belongs as a separate lightweight mechanism, not by overloading the `edit_proposals` accept/reject lifecycle. Deferred to feature 020's implementation.