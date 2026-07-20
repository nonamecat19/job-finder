# Feature Specification: Post-Apply Outreach Draft Generator

**Feature Branch**: `012-outreach-generator`

**Created**: 2026-07-20

**Status**: Draft

**Input**: User description: "After a user applies, generate a short outreach draft to the hiring manager or recruiter. Ground every claim about the team in the company-intel signals we already store (plan 004). Address it to the contact we resolved (plan 007). Draft only — the system never sends. The user copies and sends it themselves."

## Clarifications

### Draft-Only Is a Hard Product Line

This feature generates text and nothing else. It has **no send path** — no SMTP, no `mailto:` auto-fire, no queued or scheduled delivery, no integration with any mail transport. The workspace records this in its Hard Product Lines: *"Draft-only outreach: 012 generates, the user sends."* The line exists so a later task cannot quietly add a "send" button; every requirement below is written to make sending impossible by construction, not merely disabled by default. The only egress of a generated draft is the user copying it out of the interface by hand.

### Grounding Is a Hard Constraint, Not a Quality Goal

Every specific claim the draft makes about the team, its stack, or the company must trace to a stored company-intel signal (plan 004). The model may not assert anything about the target's technology, funding, size, or situation that is not backed by a signal already in the store. A draft that would need an ungrounded claim to be specific must instead be less specific — vagueness is always preferred to invention.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Generate a grounded outreach draft after applying (Priority: P1)

A user has just applied to a role. On the job's detail page they open outreach and get a short, ready-to-copy message addressed to the resolved contact — the recruiter or hiring manager from plan 007 — that references the team's actual stack and situation using only what the company-intel card already knows. The user reads it, edits if they wish, copies it, and sends it themselves from their own email or LinkedIn.

**Why this priority**: This is the entire feature in one increment — a grounded draft addressed to a real contact. It ships alone and is immediately useful: the user goes from a blank message box to a specific, factual opener without leaving the job page.

**Independent Test**: Open a job that has at least one resolved contact and at least one company-intel signal, request an outreach draft, and confirm a message is produced that names the contact, stays within the length limit, and references only stored signals. No sending occurs and none is offered.

**Acceptance Scenarios**:

1. **Given** a job with a resolved contact and one or more company-intel signals, **When** the user requests an outreach draft, **Then** a single message is produced that addresses the contact by name, references at least one stored signal, and fits within the configured length limit.
2. **Given** the produced draft, **When** it is presented, **Then** the interface offers only copy and regenerate actions — there is no send, schedule, or queue action anywhere in the flow.
3. **Given** a draft that references the team's stack, **When** each specific claim is checked, **Then** every one traces to a stored company-intel signal and none is invented.
4. **Given** the user edits the draft text, **When** they copy it, **Then** the edited text is what leaves the interface, and the system takes no further action with it.
5. **Given** the same job, **When** the user regenerates, **Then** a new draft is produced under the same length and grounding rules, and the previous draft is not sent or retained as an action.

---

### User Story 2 - Choose a tone for the draft (Priority: P2)

Before or after generating, the user picks a tone — warm, direct, or formal — and the draft is written in that register while keeping the same grounded facts and the same length limit. The user who is cold-emailing a startup founder and the user contacting a corporate recruiter get messages that read appropriately different without either one inventing anything.

**Why this priority**: Depends on Story 1 producing a draft. Tone makes the draft usable across very different recipients, but the feature is already valuable with a single default tone. Independently shippable once drafts exist.

**Independent Test**: Generate the same job's draft under each available tone and confirm the register changes while the grounded claims and the length ceiling do not.

**Acceptance Scenarios**:

1. **Given** a set of available tones, **When** the user selects one and generates, **Then** the draft is written in that tone.
2. **Given** two drafts of the same job under two different tones, **When** their specific claims are compared, **Then** both draw only from the same stored signals — tone changes wording, never facts.
3. **Given** any tone, **When** the draft is produced, **Then** it still respects the length limit and the draft-only constraint.
4. **Given** no tone is chosen, **When** the user generates, **Then** a defined default tone is used rather than an error.

---

### User Story 3 - See what each claim is grounded in (Priority: P3)

The user expands the draft and sees, for each specific claim it makes about the team or company, which stored signal backs it. This lets the user verify the message is factual before sending it under their own name — and spot immediately if a claim they thought was supported actually is not.

**Why this priority**: Trust and verification, not core function. The draft is usable without it, but a user putting their own name on a cold message wants to confirm its claims before a hiring manager reads them.

**Independent Test**: Generate a draft with two or more grounded claims and confirm each claim can be traced in the interface to the specific company-intel signal that supports it.

**Acceptance Scenarios**:

1. **Given** a draft that makes a specific claim about the team's stack, **When** the user inspects that claim, **Then** the company-intel signal backing it is identifiable.
2. **Given** a draft that makes no specific team claim because no signal supported one, **When** the user inspects it, **Then** the draft reads as a generic-but-honest opener rather than a fabricated-specific one, and nothing is presented as grounded that is not.

---

### Edge Cases

- **No resolved contact for the job** → no addressable recipient exists; the draft is either not offered or is generated with a neutral salutation clearly marked as having no named recipient. The system never guesses a contact name.
- **A contact with a name but no email or channel** → a draft may still be generated (the user has their own way to reach the person); the absence of an email is never a reason to fabricate one, and the draft-only constraint means the missing email has no functional effect anyway.
- **No company-intel signals for the company** → no specific team claim can be grounded; the draft falls back to an honest generic opener with no invented specifics rather than refusing outright.
- **Only low-confidence or stale signals available** → the draft either omits the weakly-grounded claim or is not offered a specific claim for it; a shaky signal is never laundered into a confident-sounding sentence.
- **The model tries to add an ungrounded specific** (e.g. inventing a funding round or a framework the company never mentioned) → that claim must be rejected or stripped; a draft is never shipped containing a specific claim with no backing signal.
- **The generated text exceeds the length limit** → it is regenerated or truncated to fit; an over-length draft is never presented.
- **The resolved contact came from an opt-in LinkedIn source that is disabled** → outreach uses only the contacts actually available under current settings; it never re-derives a contact from a disabled source.
- **The user has not applied to the job yet** → outreach is a post-apply action; whether it is offered before applying is a product choice, but the draft-only and grounding constraints hold regardless of when it is generated.
- **Two contacts resolved for one job** (e.g. a recruiter and a hiring manager) → the user chooses which contact the draft addresses; the draft addresses exactly one recipient and does not silently merge them.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST generate, on request for a job the user has applied to, a single short outreach message addressed to a contact resolved for that job (plan 007).
- **FR-002**: System MUST NOT provide any means of sending, scheduling, queuing, or transmitting a generated draft — no SMTP path, no `mailto:` auto-fire, no delivery integration of any kind. The only way a draft leaves the system is the user copying it manually.
- **FR-003**: System MUST offer, in the outreach flow, only actions that keep the draft inside the system — at minimum copy and regenerate — and MUST NOT present a send, schedule, or deliver action.
- **FR-004**: System MUST ground every specific claim the draft makes about the target team, its technology stack, its funding, its size, or its situation in a stored company-intel signal (plan 004), and MUST NOT assert any such specific claim that is not backed by a stored signal.
- **FR-005**: System MUST prefer a less specific, honest message over inventing a specific claim; when no signal supports a specific, the draft MUST fall back to a generic opener rather than fabricate one.
- **FR-006**: System MUST reject or strip any model-produced specific claim about the team or company that cannot be traced to a stored signal, before the draft is presented.
- **FR-007**: System MUST address the draft to exactly one resolved contact and MUST use only the contact's actually-resolved identity; it MUST NOT invent a contact name, email, or channel.
- **FR-008**: System MUST let the user choose which resolved contact a draft addresses when more than one contact exists for the job.
- **FR-009**: System MUST enforce a maximum draft length and MUST NOT present a draft that exceeds it, regenerating or truncating to fit.
- **FR-010**: System MUST offer a defined set of tone options and MUST apply the selected tone to the draft's wording without changing which facts it asserts.
- **FR-011**: System MUST use a defined default tone when the user selects none, rather than failing.
- **FR-012**: System MUST produce a coherent, honest draft when no company-intel signal is available for the company, containing no specific team claim.
- **FR-013**: System MUST NOT treat the absence of a contact email or channel as license to fabricate one; missing contact details have no functional effect because the system never sends.
- **FR-014**: System MUST make each specific grounded claim in the draft traceable to the company-intel signal that supports it, so the user can verify the message before sending it themselves.
- **FR-015**: System MUST use only contacts available under current settings, and MUST NOT re-derive a contact from a data source that is currently disabled.
- **FR-016**: System MUST run generation using only the self-hosted model runtime the rest of the system uses, with no dependency on a third-party paid inference API.
- **FR-017**: System MUST NOT change the job's status or the contact's record as a result of generating, regenerating, or copying a draft — outreach generation is a read-and-produce action, never a state change.

### Key Entities

- **Outreach Draft**: A short generated message addressed to one resolved contact for one job, written in a chosen tone, whose every specific claim is backed by a company-intel signal. Produced on demand; whether it is persisted at all is a product choice, but it is never an object that can be *sent* by the system.
- **Grounding Trace**: The mapping from each specific claim in a draft to the company-intel signal that supports it. The content of the Story 3 verification view and the mechanism by which FR-006 rejects ungrounded specifics.
- **Tone**: One of a defined set of registers (e.g. warm, direct, formal) that changes a draft's wording without changing its facts. Has a default.
- **Company-Intel Signal** *(existing, plan 004)*: A stored fact about a company — funding, layoffs, Glassdoor rating, headcount trend, tech stack. The sole permitted source of any specific claim the draft makes about the team or company.
- **Resolved Contact** *(existing, plan 007 `JobContact`)*: A recruiter or hiring manager resolved for a job, carrying name, title, and possibly email/LinkedIn/phone with a source and confidence. The sole permitted addressee of a draft.
- **Job** *(existing)*: The applied-to posting the outreach is about. Unchanged by this feature — no status transition results from generating a draft.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero code paths exist that transmit a draft — no SMTP client, no `mailto:` auto-navigation, no queue or scheduler — verifiable by inspection of the feature's surface and confirmed by the absence of any send action in the interface.
- **SC-002**: 100% of specific claims about the team or company across a hand-checked sample of generated drafts trace to a stored company-intel signal; zero fabricated specifics.
- **SC-003**: When a company has no intel signals, 100% of generated drafts are honest generic openers containing no specific team claim — none invents a stack, a funding round, or a headcount to fill the gap.
- **SC-004**: Every generated draft addresses a contact that actually exists in the job's resolved contacts, or a clearly-marked neutral salutation when none exists — zero drafts address an invented person.
- **SC-005**: 100% of presented drafts fall within the length limit; no over-length draft reaches the user.
- **SC-006**: Every offered tone produces a draft in that register while the set of asserted facts stays identical across tones for the same job — tone never adds or removes a claim.
- **SC-007**: A user can trace every specific claim in a draft to its backing signal from the interface without reading logs or querying the database.
- **SC-008**: Generating, regenerating, or copying a draft produces zero job-status transitions and zero contact-record mutations attributable to this feature.

## Assumptions

- **Draft-only is inviolable**: The user's explicit product line is that the system generates and the user sends. This spec treats a send path as out of scope permanently, not deferred — a future "add sending" request is a new product decision, not a task under this spec.
- **Grounding source is plan 004 only**: Specific team/company claims may draw only from stored company-intel signals. The model's own training knowledge about the company is not a permitted source — if it is not a stored signal, it cannot be asserted as a specific.
- **Addressee source is plan 007 only**: Recipients come from resolved `JobContact` rows. Outreach was explicitly out of scope for plan 007 itself; this feature is where it lands, and it consumes 007's output rather than re-resolving contacts.
- **Vagueness beats invention**: When grounding is thin, the correct output is a less specific message, never a fabricated specific one. This is a hard rule, not a tunable quality trade-off.
- **Post-apply trigger**: Outreach is framed as a post-apply action. The exact placement (only after apply, or also before) is a plan/UI decision; the constraints in this spec hold whenever a draft is generated.
- **Tone changes wording only**: The tone options restyle the same grounded facts. No tone is permitted to introduce a claim another tone would omit.
- **Self-hosted inference**: Generation uses the existing self-hosted model runtime, consistent with the rest of the system's inference flows and with the no-paid-API stance elsewhere in the project.
- **Length limit is short by design**: Outreach is a brief opener, not a cover letter. The specific ceiling is set in plan.md; the intent is a message a busy recruiter reads in one glance.
- **Persistence of drafts is undecided here**: Whether a generated draft is stored, and for how long, is a plan.md concern. Whatever the answer, a stored draft is never a sendable object.

## Dependencies

- **Plan 004 (Company Intel Card)** — the stored company-intel signals that are the sole grounding source. Outreach cannot make a specific claim before 004's signals exist for a company. (Blocking, per Documentation Directory: 012 blocked on 004-4.)
- **Plan 007 (Recruiter Resolution)** — the resolved `JobContact` rows that are the sole addressee source. Outreach cannot address a real contact before 007 resolves one. (Blocking, per Documentation Directory: 012 blocked on 007-3.)
- The existing self-hosted model runtime and its structured-completion path, used for generation.
- The existing job detail page, which the outreach action extends.
- The existing configuration surface, for the length limit and available tones.
