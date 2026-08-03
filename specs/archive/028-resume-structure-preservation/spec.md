> **ARCHIVED — SHIPPED.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/resume-generation.md`](../../domains/resume-generation.md) — read that first.

---
# Feature Specification: Strict Resume Structure Preservation During AI Tailoring

**Feature Branch**: `028-resume-structure-preservation`

**Created**: 2026-07-31

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "improve resume generation: 1 - resume should be strict, blocks sequence should be unchanged (name, personal info, summary, skills, experience, education); 2 - ai should not have permission to change sequence of jobs, should be same like in template; 3 - ai should not change total experience years"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Fixed Block Sequence Across AI Tailoring (Priority: P1)

A job seeker runs AI resume tailoring against a target job posting. The master resume is laid out in a fixed block order — name, personal info (contact/location/links), summary, skills, experience, education. After tailoring completes, the tailored resume presents these blocks in exactly the same order as the master; the AI is not permitted to add, remove, rename, or resequence blocks even when it judges a different order would better fit the target role. The only thing that may change within these blocks is the content of the AI's allow-listed text fields (summary, experience description bullets, skills) — never the block structure itself.

**Why this priority**: A predictable, invariant block sequence is the foundation of a trustworthy tailored resume. Recruiters and ATS systems read resumes in an expected order; an AI that silently reorders sections (e.g., moving experience above skills, or dropping education) undermines the candidate's control over how they present themselves and breaks the "constrained, faithful tailoring" promise of feature 020.

**Independent Test**: Run AI tailoring on a resume whose blocks are in the canonical order, then diff the tailored resume's block order against the master. The block sequence must be identical; only the allow-listed field contents may differ.

**Acceptance Scenarios**:

1. **Given** a master resume with blocks in the canonical order (name → personal info → summary → skills → experience → education), **When** the user invokes AI tailoring against a job posting, **Then** the tailored resume presents the same six blocks in the same order, with no blocks added, removed, renamed, or reordered.
2. **Given** the AI proposes a resequencing of blocks (e.g., moving experience before skills, or inserting a new "Projects" block), **When** the proposal is returned, **Then** that structural change is discarded by the system and the master resume's block order is preserved unchanged.
3. **Given** a master resume that is missing one or more canonical blocks (e.g., the user has no education section), **When** tailoring completes, **Then** the tailored resume contains exactly the blocks present in the master, in canonical order, and the AI does not invent the missing block.
4. **Given** a master resume whose blocks are in canonical order and the target job is a career-change role where the AI might judge skills-before-experience to be more compelling, **When** tailoring completes, **Then** the block order remains unchanged — the constraint holds regardless of the AI's opinion about optimal ordering.

---

### User Story 2 - Job Order Preservation Within the Experience Block (Priority: P1)

A job seeker has a master resume whose experience block lists jobs in a specific order (typically most-recent-first, but the user's chosen order is what matters). The AI must not reorder the job entries within the experience block during tailoring, even when the target job posting emphasizes skills relevant to an older role that the AI might want to surface first. The order of jobs in the tailored resume's experience block is byte-for-byte identical to the master resume's job order; only the description bullets under each job may change within the existing allow-list.

**Why this priority**: Job order is part of the candidate's authored presentation of their career trajectory. An AI that silently promotes an older role to the top (because it matches the target job better) misrepresents the candidate's intended narrative and can look deceptive to a recruiter who compares the tailored resume against the master. This is a structural-integrity guardrail of equal weight to the block-sequence one.

**Independent Test**: Run AI tailoring on a resume with multiple experience entries, then compare the order of experience entries in the tailored resume against the master. The order must be identical; only per-job description bullets may differ.

**Acceptance Scenarios**:

1. **Given** a master resume with experience entries ordered [Job A, Job B, Job C], **When** the user invokes AI tailoring, **Then** the tailored resume's experience block lists the jobs in the same order [Job A, Job B, Job C], with no entries added, removed, or reordered.
2. **Given** the AI proposes reordering experience entries (e.g., surfacing Job C first because it best matches the target role), **When** the proposal is returned, **Then** that reordering is discarded and the master resume's job order is preserved.
3. **Given** a master resume with experience entries where the AI proposes modifying a description bullet on Job B, **When** the proposal is applied, **Then** Job B remains in its original position within the experience block — the bullet edit does not change the entry's order.
4. **Given** the target job posting strongly emphasizes experience from an older role that is listed last in the master resume, **When** tailoring completes, **Then** the older role remains in its original (last) position; the AI may enrich its description bullets but may not move it.

---

### User Story 3 - Total Experience Years Integrity (Priority: P1)

A job seeker has a master resume that states a total years-of-experience figure (either as an explicit field or derivable from the experience entries' date ranges). The AI must not change this figure during tailoring — not to inflate it to match a seniority requirement in the job posting, nor to deflate it, nor to omit it. The total experience years in the tailored resume equal the total experience years in the master resume, exactly as authored or as derived from the same dated experience entries.

**Why this priority**: Total experience years is one of the most heavily scrutinized and most easily misrepresented facts on a resume. Inflating it to match a job's stated seniority requirement is the single highest-risk form of resume fabrication and directly violates the Grounded Generation constitution principle. Locking this figure is a non-negotiable integrity guardrail.

**Independent Test**: Run AI tailoring, then compare the total experience years (explicit field or computed from experience date ranges) in the tailored resume against the master. The figure must be identical; no proposal may alter it.

**Acceptance Scenarios**:

1. **Given** a master resume with an explicit total-experience-years statement, **When** the user invokes AI tailoring, **Then** the tailored resume carries the identical total-experience-years value in the identical location and format.
2. **Given** the AI proposes changing the total experience years (e.g., inflating "5 years" to "7 years" to match a job requiring 7+ years), **When** the proposal is returned, **Then** that change is discarded and the master resume's figure is preserved.
3. **Given** a master resume with no explicit total-experience-years field but experience entries with start/end dates, **When** tailoring completes, **Then** the total experience years derivable from the experience date ranges in the tailored resume equals the same figure derivable from the master — no experience entry's dates were altered (which would implicitly change the computed total).
4. **Given** the AI proposes adding a phrase like "over 8 years of experience" to the summary when the master resume states "5 years", **When** the proposal is returned, **Then** that inflation is suppressed by the grounding guardrail and the summary must not assert a years figure that exceeds or contradicts the master resume's total.

---

### Edge Cases

- What happens when the master resume's blocks are not in the canonical order (e.g., a user reordered them via feature 009 to put education first)? The tailored resume preserves the user's authored order; the canonical order is the default for new resumes, but the constraint is "preserve the master's order", not "force the canonical order onto an already-customized master". The AI does not reset, warn about, or "correct" a user-customized order — it only preserves it.
- What happens when the master resume contains a non-canonical block the user added (e.g., a "Projects" or "Certifications" block)? That block is preserved in its authored position; the AI does not remove or relocate it. The canonical six-block sequence defines the default layout, not a restriction on what blocks may exist.
- What happens when the master resume has no explicit total-experience-years field and the experience entries have missing or partial date ranges (e.g., a current role with a start date but no end date)? The system computes total experience from the available date ranges using the same convention as the master; the AI may not introduce a different computation or a rounded figure.
- What happens when the AI proposes a new skill group whose label references years (e.g., "10+ years of cloud experience")? Such a label is suppressed by the grounding guardrail unless the years claim is directly traceable to and consistent with the master resume's experience entries.
- What happens when the user manually reorders experience entries in the master resume between the tailoring run starting and finishing? The baseline-content-hash drift detection (feature 020) already aborts the run with a clear error; this feature relies on that existing guardrail and adds no new behavior for that case.
- What happens when the master resume's experience block is empty (no jobs)? The constraint is trivially satisfied; the tailored resume's experience block remains empty and the AI introduces no fabricated jobs.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST define a canonical resume block sequence — name, personal info (contact/location/links), summary, skills, experience, education — as the default layout for new and AI-tailored resumes, while preserving any additional user-authored blocks (e.g., projects, certifications) in their authored positions.
- **FR-002**: The system MUST NOT permit the AI to add, remove, rename, or reorder resume blocks during tailoring. The block sequence of the tailored resume MUST equal the block sequence of the master resume.
- **FR-003**: The system MUST NOT permit the AI to reorder, add, or remove experience (job) entries within the experience block during tailoring. The order and identity of experience entries in the tailored resume MUST equal the order and identity in the master resume; only the description bullets under each job may change within the existing allow-list (feature 020 FR-001).
- **FR-004**: The system MUST NOT permit the AI to alter the total years of experience stated in or derivable from the master resume. This covers both an explicit years-of-experience statement and any figure computed from the experience entries' date ranges — the tailored resume's figure MUST equal the master's figure.
- **FR-005**: When the AI produces a proposal that would violate FR-002, FR-003, or FR-004, the system MUST discard that proposal (mark it `dropped` with a structural-integrity reason) and preserve the master resume's value for the affected field, without surfacing the violating proposal to the user for accept/reject.
- **FR-006**: The system MUST extend the existing AI edit-proposal allow-list (feature 020 FR-001: summary, experience description bullets, skills/skill-groups) with explicit structural-integrity invariants: block sequence, experience-entry order, and total experience years. Structural-integrity invariants are enforced automatically (not user-accept/reject) because they are non-negotiable guardrails, not content preferences.
- **FR-007**: The system MUST detect and suppress AI-proposed summary or experience-bullet text that asserts a total-years-of-experience figure that contradicts the master resume's figure, treating such an assertion as a grounding violation (feature 020 FR-003).
- **FR-008**: The structural-integrity guardrails (FR-002, FR-003, FR-004) MUST apply to every AI tailoring run, including re-runs on an already-tailored baseline (feature 020 FR-010). A re-run may not relax or bypass the block-sequence, job-order, or total-experience constraints.
- **FR-009**: The system MUST NOT permit the AI to alter the dates (start/end) of any experience entry, because altering dates would implicitly change the derivable total experience years (FR-004). Dates remain outside the AI allow-list (feature 020 FR-002 already excludes them; this requirement makes the rationale explicit and binds it to the total-experience invariant).
- **FR-010**: When the master resume's blocks are not in the canonical order, the system MUST preserve the master's authored order in the tailored resume. The canonical sequence (FR-001) is the default for new resumes and the reference order, not a force-rewrite rule applied to already-customized masters.
- **FR-011**: The system MUST report, in the same diff/review surface used for content proposals, any AI proposals that were dropped for structural-integrity reasons (with a clear, non-technical label such as "kept original order" or "kept original experience total") so the user understands why a proposed change did not appear — without offering the user a way to override the guardrail.

### Key Entities *(include if feature involves data)*

- **Canonical Block Sequence**: The default, fixed top-level layout order for a resume — name, personal info, summary, skills, experience, education — plus any additional user-authored blocks preserved in their authored positions. The reference order against which the tailored resume's block order is validated.
- **Experience Entry Order**: The authored ordering of job entries within the experience block of the master resume. An invariant the AI may not alter during tailoring; only per-entry description bullets may change.
- **Total Experience Years**: The candidate's total years of professional experience, either stated explicitly in the master resume or computed from the experience entries' date ranges. An invariant the AI may not alter, inflate, or contradict in generated text.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of AI-tailored resumes present resume blocks in the same order as their master resume — zero block additions, removals, renames, or reorders across all tailoring runs.
- **SC-002**: 100% of AI-tailored resumes list experience (job) entries in the same order as their master resume — zero experience-entry additions, removals, or reorders across all tailoring runs.
- **SC-003**: 100% of AI-tailored resumes carry the same total years of experience as their master resume — zero changes to an explicit years figure and zero changes to experience date ranges that would alter a computed total.
- **SC-004**: 100% of AI proposals that would violate the block-sequence, job-order, or total-experience invariants are automatically dropped (not surfaced for user accept/reject) and recorded with a structural-integrity drop reason, auditable after the run.
- **SC-005**: Zero accepted AI edits introduce or imply a total-years-of-experience figure that exceeds or contradicts the master resume's figure — auditable via the traceability and grounding checks (feature 020 SC-005).
- **SC-006**: A user reviewing a tailored resume can confirm, by eye and within 30 seconds, that the block order, job order, and total experience years match their master resume — the structural invariants are visibly preserved, not silently enforced.

## Assumptions

- The master resume and its block/experience structure already exist via feature 009 (editable resume profile); this feature constrains the AI tailoring flow (feature 020), it does not change how the user authors or reorders their master resume.
- The "canonical block sequence" (name → personal info → summary → skills → experience → education) reflects the user's stated default; a master resume the user has already reordered (via feature 009's reorder capability) is preserved as-is by tailoring rather than reset to the canonical order.
- "Total experience years" is either an explicit field on the master resume or is computed by the same date-range logic the system already uses; this feature does not introduce a new computation, it locks the existing one against AI alteration.
- The structural-integrity guardrails are enforced automatically (dropped, not user-accept/reject) because they are non-negotiable integrity invariants, consistent with the Grounded Generation constitution principle — the user does not get to "opt in" to a reordered or inflated resume via the AI.
- The existing diff/review surface (feature 020 FR-011) is extended to show dropped structural-integrity proposals with a non-technical label; building a new dedicated surface is out of scope.
- These constraints apply to the AI tailoring/generation flow only; the user retains full manual control to reorder blocks and jobs in their master resume via feature 009.