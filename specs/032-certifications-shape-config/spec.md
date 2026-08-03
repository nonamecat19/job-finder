# Feature Specification: Certifications as a Configurable Resume Category

**Feature Branch**: `032-certifications-shape-config`

**Created**: 2026-08-03

**Status**: Draft

**Input**: User description: "add support of certifications, similar to other categories"

## Context

The resume generation shape settings let a user control how much of each resume
category survives tailoring: how long the summary is, whether skills render and how
many groups are kept, how many bullets each experience entry keeps, and whether
projects render plus how many projects and project bullets survive.

Certifications are already a recognised resume section — they hold a fixed position in
the enforced section order (between education and publications) — but they are the only
such section with no controls at all. A certifications section passes through generation
verbatim: it cannot be turned off, cannot be capped, and never participates in
vacancy-relevance selection. This feature closes that gap by giving certifications the
same class of controls the other configurable categories already have.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Turn the certifications section off (Priority: P1)

A user whose master profile lists certifications that are irrelevant to the roles they
are currently targeting wants generated resumes to omit the certifications section
entirely, without deleting the certifications from their master profile.

**Why this priority**: This is the smallest complete slice and mirrors the existing
skills/projects toggle exactly. On its own it delivers real value — reclaiming page
space on a length-constrained resume — and it is a precondition for every other
certifications control.

**Independent Test**: Set the certifications toggle off, generate a resume from a
profile that has a certifications section, and confirm the rendered document contains
no certifications section while the master profile still lists them. Toggle back on and
confirm the section returns.

**Acceptance Scenarios**:

1. **Given** a profile with a certifications section and the certifications toggle
   enabled, **When** a resume is generated, **Then** the rendered resume contains the
   certifications section in its established position in the section order.
2. **Given** the same profile with the certifications toggle disabled, **When** a
   resume is generated, **Then** the rendered resume contains no certifications section
   and the remaining sections keep their relative order.
3. **Given** the certifications toggle is disabled, **When** the user views their master
   profile, **Then** the certifications entries are still present and unmodified.
4. **Given** a profile with no certifications section at all, **When** a resume is
   generated with the toggle in either state, **Then** generation succeeds and no empty
   certifications heading is rendered.

---

### User Story 2 - Cap how many certifications appear (Priority: P2)

A user with a long list of certifications wants only a limited number to appear on the
generated resume, so the resume stays within its page target.

**Why this priority**: Delivers the length control that motivates the feature for users
who want certifications kept but shortened. Depends on Story 1 only for the settings
surface, and is independently demonstrable.

**Independent Test**: Configure a maximum of 3 certifications on a profile that has 8,
generate, and confirm exactly 3 appear.

**Acceptance Scenarios**:

1. **Given** a maximum of 3 certifications and a profile with 8, **When** a resume is
   generated, **Then** exactly 3 certifications appear in the rendered resume.
2. **Given** a maximum of 3 certifications and a profile with 2, **When** a resume is
   generated, **Then** both certifications appear and none are invented to reach the cap.
3. **Given** the maximum is set to the "no limit" value, **When** a resume is generated,
   **Then** every certification in the master profile appears.
4. **Given** a minimum of 4 certifications and a profile with only 2, **When** a resume
   is generated, **Then** generation succeeds, both certifications appear, and the run
   records a shortfall reporting 4 requested against 2 available.

---

### User Story 3 - Manage certifications settings alongside the other categories (Priority: P2)

A user adjusting resume shape settings sees and edits the certifications controls in the
same place, with the same behaviour, as the summary, skills, experience and projects
controls.

**Why this priority**: The user's request is explicitly about parity with other
categories. Without this, the capability exists but is not reachable or discoverable.

**Independent Test**: Open the resume shape settings, confirm certifications controls
appear grouped with the others, change them, save, reload, and confirm the values
persisted.

**Acceptance Scenarios**:

1. **Given** the resume shape settings, **When** the user views them, **Then**
   certifications controls are shown alongside the existing category controls.
2. **Given** edited certifications settings, **When** the user saves and reloads,
   **Then** the saved values are shown.
3. **Given** a certifications setting outside its permitted range, **When** the user
   saves, **Then** the update is rejected with a message naming the offending setting
   and its valid range, and no previously stored setting is changed.
4. **Given** any certifications settings, **When** the user resets resume shape settings
   to defaults, **Then** the certifications settings return to their documented defaults
   along with the rest.
5. **Given** a system upgraded from before this feature, **When** settings are read
   without ever being edited, **Then** certifications settings hold defaults that
   reproduce the previous behaviour: section shown, no cap applied.

---

### Deferred - Vacancy-relevance selection of certifications

Selecting *which* certifications survive a cap by relevance to the target vacancy is
explicitly **out of scope** (see FR-015). A cap truncates in the master profile's
authored order.

**Why deferred**: Relevance selection would pull certifications into the tailoring
prompt and therefore into grounding verification, the way projects are. That is the
largest cost and risk in the feature, and it buys little: certifications are short,
atomic credentials that a user can order by hand once, whereas projects are long,
numerous and genuinely vacancy-dependent. Truncation is deterministic, adds zero tokens,
and introduces no fabrication surface. If users later report that hand-ordering is
insufficient, this becomes its own feature.

---

### Edge Cases

- A certifications section exists but is empty: no empty heading is rendered, and no
  shortfall is reported for a configured minimum of zero.
- The certifications section is named unconventionally in the master profile (e.g.
  "Licenses & Certifications"): behaviour must be predictable and documented — see
  Assumptions.
- Minimum is set above maximum: rejected at save time as a cross-field validation error.
- Minimum above zero while the section toggle is off: rejected, mirroring the existing
  projects rule.
- Cap exceeds the number of certifications available: all are kept, none invented.
- Disabling certifications removes the section from both the rendered document and the
  enforced section order, leaving no gap or placeholder.
- Certifications are disabled on a profile that has no certifications: no error.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The resume shape settings MUST include a certifications enable/disable
  control that determines whether the certifications section renders in generated
  resumes.
- **FR-002**: The resume shape settings MUST include a maximum number of certifications
  retained in a generated resume, with a designated "no limit" value.
- **FR-003**: The resume shape settings MUST include a minimum target number of
  certifications, with a designated "no minimum" value.
- **FR-004**: When certifications are disabled, the system MUST remove the certifications
  section from the generated resume and from the enforced section order, without
  modifying the user's master profile.
- **FR-005**: When a certifications maximum is configured and the master profile holds
  more than that number, the system MUST retain exactly that number and discard the rest.
- **FR-006**: The system MUST NOT invent, duplicate or pad certifications to satisfy a
  configured minimum; a shortfall MUST be recorded instead, naming the certifications
  section, the requested count and the available count.
- **FR-007**: Every certification appearing in a generated resume MUST be traceable to a
  certification in the user's master profile; a certification with no such source MUST be
  rejected as a grounding violation before the resume is delivered.
- **FR-008**: Certifications settings MUST be validated before any of them are persisted,
  rejecting the whole update with a message naming the offending setting and its valid
  range, leaving all previously stored settings unchanged.
- **FR-009**: Validation MUST reject a certifications minimum greater than a configured
  certifications maximum, and MUST reject a certifications minimum above zero while
  certifications are disabled.
- **FR-010**: Certifications settings MUST have defaults that reproduce current
  behaviour — section rendered, no cap, no minimum — so an existing user's generated
  resumes are unchanged until they opt in.
- **FR-011**: Certifications settings MUST persist across restarts and MUST be included
  in the reset-to-defaults action alongside the existing settings.
- **FR-012**: Certifications settings MUST be readable and editable through the same
  settings surface as the existing resume shape categories, and MUST be carried across
  the system boundary using the project's shared, generated type contract rather than a
  hand-duplicated definition.
- **FR-013**: Certifications MUST keep their established position in the enforced resume
  section order whenever they render, regardless of how the master profile orders its
  own sections.
- **FR-014**: When no certifications limit is configured, generation MUST behave exactly
  as it does today — certifications pass through verbatim, with no added generation cost.
- **FR-015**: When a certifications maximum is configured, the system MUST retain the
  first N certifications in the master profile's authored order and MUST NOT reorder,
  rewrite or relevance-rank them. Certifications MUST NOT be sent to the tailoring
  model, so a certifications limit adds no generation cost. The user controls which
  certifications survive by ordering them in their master profile.
- **FR-016**: The system MUST NOT introduce a per-certification detail-line cap in this
  feature. Certification entry bodies are passed through unchanged, whatever entry shape
  the master profile stores them as.

### Key Entities *(include if data involved)*

- **Resume shape configuration**: The single persisted set of user-controlled resume
  shape settings. Gains certifications attributes — enabled, minimum, maximum — matching
  the naming and "zero means unlimited" convention of the existing project attributes.
- **Certifications section**: A named, ordered group of certification entries within a
  user's master resume profile, occupying a fixed position in the enforced section order.
- **Certification entry**: A single credential in that section, identified by a name or
  label that must be reproduced exactly when it appears in a generated resume.
- **Shape report**: The per-generation record of shape outcomes. Gains the ability to
  report a certifications shortfall, in the same form as existing shortfalls.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can disable the certifications section and see it absent from the
  next generated resume, with no other section's content or order changed.
- **SC-002**: With a certifications maximum of N configured and more than N
  certifications in the master profile, 100% of generated resumes contain exactly N
  certifications.
- **SC-003**: 100% of certifications appearing in generated resumes are traceable to a
  certification in the user's master profile, across the full generation test suite.
- **SC-004**: With default certifications settings, generated resumes are identical to
  those produced before this feature for the same profile, vacancy and model output.
- **SC-005**: Every certifications setting can be changed, saved and confirmed persisted
  in under 1 minute from opening the settings surface.
- **SC-006**: 100% of out-of-range or contradictory certifications settings updates are
  rejected without altering any stored setting.
- **SC-007**: A configured minimum the profile cannot meet produces a resume plus a
  recorded shortfall in 100% of such runs, and never a failed generation or an invented
  certification.

## Assumptions

- "Similar to other categories" means parity with the projects category — the most
  recently and most fully configured one — namely an enable toggle, a minimum, and a
  maximum, following the same "zero means unlimited / no minimum" convention and the
  same validation shape.
- Certifications settings live in the existing single resume shape configuration record
  rather than in a new settings area; no per-profile or per-application override is in
  scope.
- The certifications section is identified by the same section key already used by the
  enforced section order. Differently named sections holding credentials are treated as
  ordinary custom sections and are out of scope for these controls.
- Permitted ranges for the certifications minimum and maximum follow the existing project
  count ranges rather than introducing new limits.
- Existing installations are migrated with defaults that preserve current behaviour; no
  user action is required after upgrade and no existing generated resume changes.
- Reordering certifications relative to one another, editing certification text, and
  adding new certification data-entry fields to the profile editor are out of scope.
- No new resume entry shape is introduced; certifications continue to use whichever
  existing entry shape the master profile already stores them as.

## Dependencies

- Builds directly on the existing resume generation shape configuration, its settings
  surface, its persistence, and its reset behaviour.
- Relies on the existing enforced resume section order, which already reserves a position
  for certifications.
- Relies on the existing grounding verification and shortfall reporting used for
  experience and projects.
