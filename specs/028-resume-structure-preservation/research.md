# Research — Feature 028: Strict Resume Structure Preservation During AI Tailoring

## R1 — Which TailoredSections fields grant the AI drop/reorder power

**Decision**: Remove `SectionsToDrop`, `ExperienceOrder`, and `Drop` from `TailoredSections` and stop honoring them in `MergeTailored`.

**Rationale**: Three fields in `apps/api/internal/generation/domain/rendercv.go` grant the AI structural mutation power that feature 028 forbids:
- `SectionsToDrop []string` (rendercv.go:84) — applied at rendercv.go:345-350; lets the AI drop any non-protected block (projects, certifications, etc.).
- `ExperienceOrder []string` (rendercv.go:85) — applied at rendercv.go:318-337; lets the AI reorder job entries.
- `Drop bool` on `TailoredExperience` (rendercv.go:73) — applied at rendercv.go:294-308 + 310-316; lets the AI drop a job entirely.

These fields must be removed from the struct (and regenerated via tygo into `packages/shared`) AND the applying code paths removed from `MergeTailored`. Removing only the fields without the code paths would still parse the LLM output; removing the code paths is what enforces the invariant.

**Alternatives considered**: Keep the fields and clamp to no-ops in `MergeTailored` (ignore the values). Rejected — leaves the struct advertising capabilities the system no longer supports, and the prompt would still be asking for them; cleaner to remove both the ask and the mechanism together.

## R2 — Does VerifyRendercvGrounding check sequence/order/years?

**Decision**: Add a new `VerifyStructureIntegrity` function; do not extend `VerifyRendercvGrounding` with the structural checks.

**Rationale**: `apps/api/internal/generation/domain/rendercv_grounding.go` (54 lines) checks only: (1) company allowlist (no fabricated companies), (2) merged sections are a subset of master sections (no added sections — but does NOT flag dropped sections or order), (3) strict-level skill token grounding. It checks none of the three new invariants (block sequence, experience order, total years). The existing verifier is about *fuzzy grounding judgments* (did the AI invent a skill/company). The new invariants are *deterministic structural facts*. Mixing them obscures both concerns, and per R5 they are enforced at the merge layer (auto-fix), not via the retry loop — so they never enter `VerifyRendercvGrounding`'s violation-return contract at all.

**Alternatives considered**: Extend `VerifyRendercvGrounding` with the new checks. Rejected — different enforcement model (auto-fix vs. retry) and different concern (structural integrity vs. content grounding).

## R3 — How block order is persisted and round-tripped

**Decision**: Block order is already preserved through tailoring by the `cv.sections["_order"]` key; feature 028 only needs to *stop dropping non-protected sections*, not change order handling.

**Rationale**: The order is carried through `SectionOrderKey = "_order"`:
- Written on parse: `domain.ParseRendercv` (rendercv_config.go) records original YAML section order under `_order`.
- Written on `ResumeToMaster`: resume_mapping.go:384-394 rebuilds sections from `resume.Sections` and writes `sections[sectionOrderKey] = order`.
- Read on `MasterToResume`: resume_mapping.go:221 via `orderedSectionKeys` (resume_mapping.go:13-43), which reads `_order` first, alpha-sorts the rest.
- Consumed on marshal: prepare_marshal.go:49-50 reads and deletes `_order` before building the ordered YAML map.

`MergeTailored` (rendercv.go:265-353) does **not** touch `sections["_order"]` — it only mutates section *contents* and `delete`s non-protected section keys. So section *order* of remaining sections is already preserved. The gap feature 028 closes is *no drops* (currently `SectionsToDrop` defeats the preserved set). For experience entries: there is no per-entry order key; `MergeTailored` rewrites `sections["experience"]` wholesale (rendercv.go:343), so `ExperienceOrder` (rendercv.go:318-337) is the only disturber. Removing `ExperienceOrder` application preserves master order automatically — the `kept` slice (rendercv.go:311-316) iterates master experience in order and only filters, so with no `Drop` filtering it is identical to master order.

**Alternatives considered**: Add an explicit order-equality assertion. Rejected — redundant with the merge-layer auto-fix (R5), which makes order drift impossible.

## R4 — Total experience years: explicit field vs. derivable

**Decision**: There is no explicit "total experience years" field anywhere; the invariant reduces to (a) keep experience dates outside the AI allow-list (already true) and (b) suppress AI-asserted numeric years claims in summary/bullet free text via a post-merge text check.

**Rationale**: `dto.Resume` (dto/resume.go:122-136) and `dto.Entry` (resume.go:53-110) carry only per-entry `Date`, `StartDate`, `EndDate` (free-text strings, not parsed years). No code computes or stores a derived total. Dates are already outside the AI allow-list: `MergeTailored` writes only `highlights` for experience (rendercv.go:300-307), `details` for skills (rendercv.go:282), and `summary` (rendercv.go:276) — never `company`, `position`, `start_date`, `end_date`, `date`, or `location`. So invariant #3's "AI may not alter dates" is *already enforced* by the existing allow-list; what's new is "AI may not assert a contradicting years figure in generated text." That is a text-content check against the LLM's free-text summary and highlights output — not a date-mutation check.

**Alternatives considered**: Introduce a parsed/derived total-experience-years field and lock it. Rejected — no such field exists today; introducing one is scope creep and changes the resume data model. The text-assertion check covers the real risk (inflated prose like "over 12 years") without a schema change.

## R5 — Retry, hard-fail, or auto-fix for structural invariants?

**Decision**: Auto-fix in the merge layer for block sequence / experience order / dropped jobs / dropped user blocks (deterministic, no LLM cooperation needed); single targeted re-prompt for text-asserted years, with strip-and-flag fallback.

**Rationale**: The existing grounding loop (service.go:204-224, `groundingAttempts = 2`) retries because `VerifyRendercvGrounding`'s violations are *fuzzy skill-grounding judgments* where a second LLM attempt plausibly helps. Structural violations are *deterministic*: if the LLM is told "reorder most-relevant first" (rendercv_llm.go:136) and obeys, it will obey on retry too. Retrying an LLM that was *asked* to violate, then *ignoring* its answer, wastes a round-trip. Therefore:

1. **Block sequence / experience order / dropped jobs / dropped user blocks** — enforce by *removing the capability* from `MergeTailored` (R1) and removing the prompt instructions that ask for it (R7). The merge layer never honors `SectionsToDrop`/`ExperienceOrder`/`Drop`, so the LLM's output on those fields is silently ignored. No retry, no surfacing — the invariant is structurally impossible to violate after merge.
2. **Text-asserted years** — not auto-fixable safely (can't rewrite the LLM's summary to remove a number without re-prompting). This is a *content* check on `merged.cv.sections.summary` and each experience `highlights[]`. On first detection, do a single targeted re-prompt (feed the violation back: "the summary asserts '12 years' but the master's experience spans 5 years; remove any numeric years claim"). If it recurs, strip the offending sentence/clause and flag it in the activity log — do not emit a resume with a wrong figure.

**Alternatives considered**:
- (a) Retry all structural violations via the existing loop. Rejected — futile for deterministic violations (the LLM was asked to violate).
- (b) Hard-fail the whole tailoring run on any structural violation. Rejected for block/order (auto-fix is strictly better — the run still produces a valid tailored resume). Considered for text-years, but a single re-prompt + strip fallback produces a better UX than failing the whole run.

## R6 — Surfacing structural drops: reuse edit_proposals or separate?

**Decision**: Do not surface silent structural enforcements in the diff UI. Log them internally on the activity row only. The `edit_proposals`/`tailored_drafts` tables from feature 020 are *specified but not implemented* (no migration, no `internal/dto/tailoring.go`, no wired code — `service.go` uses the direct-tailor path straight to `generated_documents`). Feature 028 stays on that direct path.

**Rationale**: A structural "proposal" the user can't accept or reject ("we refused to drop your projects") is the wrong shape for `edit_proposals`' accept/reject lifecycle. Surfacing it as a reviewable item implies user control where there is none — the invariant is non-negotiable. The spec's FR-011 ("report dropped structural proposals in the diff surface") is satisfied by the diff view reflecting the *result* (blocks in master order, no dropped jobs, no inflated years) rather than by emitting rows the user must act on. Internal audit logging on the activity covers traceability without polluting the review surface.

If, when feature 020's draft/proposal flow is later implemented, the team wants visible "we kept your original order" annotations, that belongs as a *separate* lightweight mechanism (new `field_type` values + migration), not by overloading `edit_proposals`' CHECK-constrained accept/reject lifecycle. That decision is deferred to feature 020's implementation.

**Alternatives considered**: Reuse `edit_proposals` with new `field_type` values (`block_sequence`, `experience_order`, `total_experience_years`). Rejected — (1) tables don't exist yet, coupling 028 to 020's migration; (2) the accept/reject lifecycle is semantically wrong for non-negotiable invariants; (3) the spec (FR-005) explicitly says structural invariants are "enforced automatically (not user-accept/reject)".

## R7 — Does buildSelectPrompt ask the LLM to reorder/drop?

**Decision**: Edit `buildSelectPrompt` (rendercv_llm.go:131-143) to stop asking for reorder/drop/section-drop, and add a "no numeric years claim" instruction to the summary guidance.

**Rationale**: The prompt currently instructs the LLM to violate all three invariants:
- Line 135: "Set drop: true only for entries with score below 3" — violates invariant #2 (no job drops).
- Line 136: "Reorder experience: most relevant company first" — violates invariant #2 (experience keeps master order).
- Lines 139-143: "Decide which sections to drop... NEVER drop: summary, experience, education, skills... Drop academic sections... Drop projects" — violates invariant #1 (block sequence immutable; AI may not drop/rename/reorder blocks; user-authored blocks preserved).

The prompt edit alone is insufficient (the LLM may still emit the fields), so it is paired with the merge-layer removal (R1). Both layers must change: prompt stops asking; merge stops honoring.

New/edited instructions:
- Replace line 135: "Keep every experience entry; never set drop: true. Do not omit any job."
- Replace line 136: "Keep experience entries in the EXACT order shown in the master; do not reorder."
- Replace lines 139-143: "Do not drop, add, rename, or reorder any resume section. Keep the master's section set and order exactly as given. Do not populate sectionsToDrop."
- Add to summary guidance (after rendercv_llm.go:144-147): "Do not state a total number of years of experience (e.g. 'over 8 years'); describe seniority descriptively without a numeric claim."

**Alternatives considered**: Leave the prompt asking and rely only on merge-layer clamping. Rejected — wastes LLM effort on instructions that are then ignored, and a well-behaved LLM following "reorder most relevant first" produces a `payload.ExperienceOrder` that is silently discarded, which is wasteful and confusing when debugging. The prompt and the merge layer should agree.