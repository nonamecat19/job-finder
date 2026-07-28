# Feature Specification: Constrained AI Resume Tailoring with Single-Page PDF Output

**Feature Branch**: `020-ai-resume-tailoring`

**Created**: 2026-07-28

**Status**: Draft

**Input**: User description: "lets improve resume generation. ai can ONLY change the descriptions of each company, change summary, change skills (can also add or remove group of skills). also its required that resume must be single page pdf"

## User Scenarios & Testing *(mandatory)*

<!--
  IMPORTANT: User stories should be PRIORITIZED as user journeys ordered by importance.
  Each user story/journey must be INDEPENDENTLY TESTABLE - meaning if you implement just ONE of them,
  you should still have a viable MVP (Minimum Viable Product) that delivers value.

  Assign priorities (P1, P2, P3, etc.) to each user story, where P1 is the most critical.
  Think of each story as a standalone slice of functionality that can be:
  - Developed independently
  - Tested independently
  - Deployed independently
  - Demonstrated to users independently
-->

### User Story 1 - Tailor Resume for a Specific Job Listing (Priority: P1)

A job seeker has a master resume and an active job posting they want to apply to. They request an AI-tailored version of their resume. The system rewrites only the professional summary, the description bullets under each company/work experience entry, and the skills section (which may include adding or removing whole skill groups), while leaving everything else — contact info, job titles, employers, dates, education, certifications, links — exactly as in the master resume. The result is presented to the user for review and editing before any export.

**Why this priority**: This is the core value promise of the feature — a constrained, trustworthy AI pass that improves fit for a target role without inventing or misrepresenting the candidate's history. It embodies the Grounded Generation constitution principle by restricting AI edits to a fixed allow-list of text fields derived from the user's existing profile.

**Independent Test**: Can be fully tested by selecting a saved job posting, triggering "Tailor resume", and verifying that only the allowed fields (summary, work-experience descriptions, skills) changed while all other resume fields are byte-for-byte identical to the master resume.

**Acceptance Scenarios**:

1. **Given** a user has a saved master resume and a saved target job posting, **When** they invoke "Tailor resume for this job", **Then** the system returns a tailored resume where the summary, each company's description bullets, and the skills section (including skill groups that may be added or removed) are the only fields that differ from the master resume.
2. **Given** a tailored resume has been produced, **When** the user reviews the diff against their master resume, **Then** every changed field is one of: summary, work-experience description (per company), or skill/skill-group membership — and no job title, employer, date, education entry, certification, contact detail, or link has been altered.
3. **Given** the AI proposes changes to a field outside the allow-list (e.g., rewording a job title), **When** the result is returned, **Then** that proposed change is discarded by the system and the master resume's value is preserved for that field.
4. **Given** the user is reviewing the tailored result, **When** they reject a specific AI edit (a bullet, the summary, or a skills-group change), **Then** the corresponding field reverts to the master resume value and the rest of the tailored content is preserved.

---

### User Story 2 - Export Tailored Resume as a Single-Page PDF (Priority: P2)

After reviewing and finalizing the tailored resume, the user wants a downloadable PDF that fits entirely on a single page. The system renders the resume to PDF, and if the content would overflow one page, the system automatically applies density controls (margins, font sizing, spacing) — or surfaces actionable feedback to the user — so the final artifact is always exactly one page.

**Why this priority**: A one-page resume is a hard constraint stated by the user and a common requirement for ATS / recruiter screening; without it the tailored resume cannot be used in its intended workflow.

**Independent Test**: Trigger PDF export of a tailored resume and assert the resulting PDF has exactly one page, regardless of input content length.

**Acceptance Scenarios**:

1. **Given** a finalized tailored resume with content that fits within standard single-page bounds, **When** the user clicks "Download PDF", **Then** the system produces a PDF with exactly one page that contains the entire resume.
2. **Given** a finalized tailored resume whose content would naturally exceed one page (long descriptions, many skill groups, many roles), **When** the user clicks "Download PDF", **Then** the system applies automatic density controls (reduced margins, smaller font, tighter spacing within a bounded range) and produces exactly one page, without truncating required content.
3. **Given** a resume that cannot fit on one page even at the minimum-density bound (e.g., far too many roles to list), **When** the user attempts to export, **Then** the system blocks the one-page export and shows the user a clear, actionable message indicating which content must be shortened or removed to fit on a single page.
4. **Given** a successfully exported single-page PDF, **When** a reviewer (or ATS) opens it, **Then** all text is selectable/searchable (true text PDF, not a rasterized image) and the layout preserves a readable, professional appearance.

---

### User Story 3 - Manage Skill Groups During Tailoring (Priority: P3)

A user wants granular control over the skills section during tailoring: the AI may suggest adding a new skill group (e.g., "Cloud" if the job emphasizes it) or removing an existing group (e.g., "Legacy Systems" if the job doesn't mention it), and the user can accept, reject, or modify these group-level changes alongside individual skill additions/removals.

**Why this priority**: Skill groups are the most fluid part of resume tailoring and give the user control over how their skill set is framed; it's a refinement of the core tailoring story rather than an independent MVP slice.

**Independent Test**: Trigger tailoring on a job that emphasizes a skill domain the user has but hasn't grouped, and assert the AI proposes a new skill group the user can accept or reject.

**Acceptance Scenarios**:

1. **Given** the user's master resume has skill groups A, B, C and the target job emphasizes skills in domain D that the user has tagged in their profile, **When** the AI tailors the resume, **Then** it proposes adding a new skill group D populated from the user's existing skill tags — not from fabricated skills.
2. **Given** the user's master resume has a skill group that is irrelevant to the target job, **When** the AI tailors the resume, **Then** it may propose removing that skill group, and the user can accept or reject the removal independently of other edits.
3. **Given** the AI has proposed skill-group changes, **When** the user accepts the new group but rejects the removal, **Then** the final resume contains the original groups plus the new group, with no deletions applied.
4. **Given** the user has accepted an AI-proposed new skill group, **When** they then remove an individual skill from within that newly added group, **Then** the system applies that per-skill edit while preserving the rest of the added group, and the group remains part of the resume.

---

### Edge Cases

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right edge cases.
-->

- What happens when the target job posting provides no usable signal (e.g., extremely short or non-text content)? The system should fall back to light-touch edits (summary polish only) and inform the user that limited tailoring was possible.
- What happens when the master resume's summary is empty? The AI may draft a new summary grounded in the user's profile data and the job posting, and the user review gate still applies.
- What happens when a single-page PDF cannot be produced even at minimum density because the user has too many roles? The system blocks export and shows actionable feedback listing which sections/excess content must be shortened (see US2 scenario 3).
- What happens when the AI proposes changes that would alter the meaning of a work-experience bullet (e.g., claiming a scope or metric not present in the master resume)? Such edits must be suppressed by the grounding guardrail, since only rewording of existing user-authored descriptions is permitted, not augmentation with new facts.
- What happens when the user edits the tailored resume manually and then re-triggers AI tailoring? The current state of the resume (master + accepted AI edits + user manual edits) becomes the new baseline for the next AI pass; fields outside the allow-list remain locked.
- What happens when two consecutive AI tailoring runs disagree on a skills-group add/remove? The user retains final accept/reject control on each run; the system never auto-applies group removals without user confirmation.
- What happens when the local AI model is unreachable, the tailoring run times out, or the model returns malformed output? The system surfaces a clear error, preserves the baseline resume unchanged (no partial edits applied), and offers a single user-initiated retry; no silent auto-retry is performed.

## Requirements *(mandatory)*

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right functional requirements.
-->

### Functional Requirements

- **FR-001**: The system MUST restrict AI resume edits to an allow-list of fields: (a) the professional summary, (b) the description bullets under each work-experience/company entry, and (c) the skills section, where skill groups may be added or removed entirely and individual skills may be added or removed.
- **FR-002**: The system MUST NOT permit the AI to alter any field outside the allow-list, including but not limited to: contact information, name, job titles, employer names, employment dates, location, education entries, certifications, project URLs, and any other resume field not enumerated in FR-001.
- **FR-003**: AI-generated text for the summary and work-experience descriptions MUST be derived from and traceable to the user's existing master resume content and the target job posting — no fabricated experience, metrics, credentials, or skills may be introduced.
- **FR-004**: The system MUST present every proposed AI change to the user for explicit accept/reject before it is committed to the tailored resume; no AI edit is applied silently.
- **FR-005**: The user MUST be able to accept or reject changes at field-level granularity: the summary as a whole, each individual work-experience description bullet, each proposed skill addition, each proposed skill removal, each proposed skill-group addition, and each proposed skill-group removal. Skill-group additions and removals are presented to the user as atomic accept/reject units; once a proposed skill-group **addition** is accepted, the new group is added to the resume and the user may then edit individual skills within the newly added group (add/remove specific skills) using the same skill-edit interactions available for any existing group. Skill-group **removals**, when accepted, remove the entire group and all its skills.
- **FR-006**: Rejecting an AI edit MUST restore the corresponding field to its value in the user's current baseline resume (master + previously accepted edits). For a rejected skill-group addition, the proposed group is not added; for a rejected skill-group removal, the existing group and all its skills remain untouched.
- **FR-007**: The system MUST produce resume PDF exports that are exactly one page, rendering all content as searchable/selectable text (not rasterized), within a bounded, professional density range (margins, font size, line spacing) that the system may adjust automatically to fit content.
- **FR-008**: When content cannot fit on a single page even at the minimum-density bound, the system MUST block the export and present the user an actionable message identifying which content blocks (e.g., excessive roles, overly long descriptions) must be shortened to achieve a single page.
- **FR-009**: A user-initiated AI-tailoring run MUST NOT submit any application, contact any employer, or take any action beyond updating the local resume draft. (Per the No Auto-Apply constitution principle.)
- **FR-010**: The system MUST allow the user to re-run AI tailoring on an already-tailored resume, treating the current resume state (master + accepted AI edits + user manual edits within the allow-list) as the new baseline while continuing to enforce the field allow-list and grounding guardrails. On re-run, the diff/proposal view MUST compare new AI proposals against the current baseline (not the original master resume), so previously accepted edits are not re-surfaced as new proposals.
- **FR-011**: The system MUST surface a diff view of AI-proposed changes against the baseline resume so the user can review exactly what changed before accepting.
- **FR-012**: When the target job posting provides insufficient signal for meaningful tailoring, the system MUST degrade gracefully (e.g., summary-only polish) and inform the user that tailoring was limited.
- **FR-013**: A single AI tailoring run MUST complete in under 60 seconds on average for a typical one-page resume against the local/self-hosted model; the system MUST surface an indeterminate progress indicator if the run exceeds 30 seconds, and MUST allow the user to remain on the same view without blocking other dashboard interactions while tailoring runs in the background.
- **FR-014**: When the local AI model is unreachable, the tailoring run times out, or the model returns malformed/unparseable output, the system MUST surface a clear, non-blocking error to the user, MUST preserve the baseline resume in its pre-run state (no partial or mutated edits applied), and MUST offer a single, explicit user-initiated retry action. The system MUST NOT auto-retry silently.

### Key Entities *(include if feature involves data)*

- **Master Resume**: The user's verified, hand-authored baseline resume containing all sections (contact, summary, work experience with per-company descriptions, education, skills organized as skill groups). The single source of truth that the AI is permitted to edit only within the allow-list.
- **Target Job Posting**: A job listing the user is considering applying to, used as the grounding signal for tailoring the resume's allowed fields.
- **Tailored Resume**: A derived version of the master resume consisting of the master's untouched fields plus AI-proposed and user-accepted edits to the allow-listed fields. Exists as a draft until the user finalizes it.
- **AI Edit Proposal**: A single, atomic, reviewable change proposed by the AI, classified by field type (summary / work-experience-description / skill / skill-group), with a before-value, an after-value, and a traceability pointer to the master resume content and/or job-posting signal it was derived from.
- **Single-Page PDF Export**: A one-page, text-based PDF rendering of a finalized tailored resume, produced with automatic density controls when needed and rejected with actionable feedback when impossibility is detected.

## Success Criteria *(mandatory)*

<!--
  ACTION REQUIRED: Define measurable success criteria.
  These must be technology-agnostic and measurable.
-->

### Measurable Outcomes

- **SC-001**: 100% of AI-tailored resumes contain changes only within the allow-list (summary, work-experience descriptions, skills/skill-groups) when diffed against the master resume — i.e., zero changes to disallowed fields across all tailoring runs.
- **SC-002**: 100% of successfully exported tailored resumes are exactly one page in length when rendered to PDF.
- **SC-003**: At least 95% of resumes that fit within single-page content bounds are exported as a single-page PDF on the first attempt without the user needing to manually shorten content.
- **SC-004**: Users are able to review and accept/reject individual AI proposals in under 90 seconds on average for a typical one-page resume.
- **SC-005**: Zero accepted AI edits contain claims (employers, dates, titles, credentials, metrics) not present in the user's master resume — auditable via the traceability pointer attached to each proposal.
- **SC-006**: When content exceeds single-page capacity, 100% of export attempts result in a clear, actionable user-facing message rather than a multi-page or truncated PDF.
- **SC-007**: At least 90% of single AI tailoring runs complete in under 60 seconds on the local/self-hosted model with a typical one-page resume; an indeterminate progress indicator appears within 30 seconds.

## Clarifications

### Session 2026-07-28

- Q: What is the target end-to-end latency for a single AI tailoring run? → A: < 60 seconds typical
- Q: For proposed skill-group additions/removals, is the proposal atomic or per-skill? → A: Atomic at accept-time; after accepting an added group, the user may edit individual skills within it
- Q: How should the system behave when the local AI model is unavailable, times out, or returns malformed output? → A: Surface a clear error, preserve the baseline resume unchanged, and offer a single user-initiated retry
- Q: On re-run, should the diff view compare new AI proposals against the current baseline or the original master? → A: Against the current baseline (master + previously accepted edits), so already-accepted edits are not re-surfaced as new proposals

## Assumptions

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right assumptions based on reasonable defaults
  chosen when the feature description did not specify certain details.
-->

- The user already has a master resume stored in the system (work covered by the prior 009-editable-resume-profile feature), and that master resume contains a structured skills section organized into named skill groups.
- The user provides, or has already saved, a target job posting to tailor against; tailoring without a target job is out of scope for this feature.
- The existing resume profile data model already supports the field structure (summary, per-company description bullets, skill groups) needed to express the allow-list of editable fields. If not, a small schema extension is in scope as part of implementation.
- "Single page" refers to a standard page size (e.g., A4 or US Letter), with the system using one consistent default; configurable page size is out of scope for v1.
- The minimum-density bound (smallest acceptable font size, tightest acceptable margins) is defined at implementation time and stays within professional, ATS-readable norms; making these bounds user-configurable is out of scope for v1.
- Tailoring is initiated explicitly by the user for a chosen job posting; background/bulk tailoring of multiple resumes at once is out of scope for v1.
- The user reviews proposals in the existing dashboard UI; building a new dedicated review surface is out of scope for v1 unless implementation reveals it is required.
- The AI model used is the local/self-hosted model per the Local-First constitution principle; falling back to paid external APIs for this flow is not permitted.