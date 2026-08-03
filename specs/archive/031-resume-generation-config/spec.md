> **ARCHIVED — SHIPPED.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/resume-generation.md`](../../domains/resume-generation.md) — read that first.

---
# Feature Specification: Configurable Resume Generation Shape

**Feature Branch**: `031-resume-generation-config`

**Created**: 2026-08-02

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "add support of dynamic config of resume generation. for summary should be configured amount of lines in summary (close, not exact if its simplify the task), the amount in skills (also should be ability to disable section). each project achievements (for example set 5-8 or other options). set preferable amount of pages in resume. add support of projects in resume (example you can see in @resume/resume, should be also configurable number for example 3-4)"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Control the length of generated resume content (Priority: P1)

A job seeker generating a tailored resume wants the generated document to match the shape they know works for their target market: a summary of roughly N lines, roughly N bullets under each work-experience entry, and a document that lands on their preferred page count. Today those numbers are fixed inside the generation pipeline, so the only way to change them is to modify the system itself. The user opens resume generation settings, sets the desired ranges, and every subsequently generated resume follows them.

**Why this priority**: This is the core of the request and delivers value on its own — without any other story, users gain control over the three highest-impact dimensions of resume shape (summary length, bullet density, page count). Everything else refines that control.

**Independent Test**: Set summary length to a short range, bullets-per-role to a low range, and target pages to 1; generate a resume for a known vacancy; confirm the produced document's summary, bullet counts and page count fall within the configured ranges. Repeat with a long/2-page configuration and confirm the output changes accordingly.

**Acceptance Scenarios**:

1. **Given** default generation settings, **When** the user generates a tailored resume, **Then** the resulting document matches the current default shape (summary ~3-4 sentences, 8-10 bullets per role, 2 pages), i.e. behaviour is unchanged for users who never touch settings.
2. **Given** the user sets summary length to "short" (~2 lines) and target pages to 1, **When** a resume is generated, **Then** the summary is approximately 2 lines and the rendered document is 1 page.
3. **Given** the user sets bullets-per-experience to 5-8, **When** a resume is generated, **Then** each work-experience entry carries between 5 and 8 bullets, subject to how many the master profile actually contains for that role.
4. **Given** the user sets target pages to 2 and the first render comes out at 1 page, **When** generation completes, **Then** the system has attempted to lengthen content toward 2 pages rather than accepting the 1-page result.
5. **Given** a configured value that the master profile cannot satisfy (e.g. 8 bullets requested but the role has only 4 in the master), **When** a resume is generated, **Then** the system uses what exists without fabricating content, and the shortfall is visible in the generation record.

---

### User Story 2 - Include a configurable projects section (Priority: P2)

The user's master profile contains a projects section (personal/open-source work with name, link, dates and achievement bullets). They want that section to appear in generated resumes, with a configurable number of projects included (e.g. 3-4) so the section can be tuned per target role and page budget, and a configurable number of achievements per project.

**Why this priority**: Adds a new content section to the output rather than tuning existing sections. Valuable, but the resume is already usable without it, and it depends on the same settings surface built in Story 1.

**Independent Test**: With a master profile containing more projects than the configured limit, generate a resume and confirm the projects section renders with exactly the configured number of entries, each showing at most the configured number of achievement bullets, and that projects appear in their pinned position in the section order.

**Acceptance Scenarios**:

1. **Given** a master profile with 6 projects and a configured limit of 3-4, **When** a resume is generated, **Then** the projects section appears with 3 or 4 project entries.
2. **Given** projects are included, **When** a resume is generated, **Then** each project's name, link and dates are reproduced from the master profile without alteration.
3. **Given** the configured achievements-per-project is 2, **When** a resume is generated, **Then** each project shows at most 2 achievement bullets, drawn only from that project's master content.
4. **Given** the master profile contains no projects section, **When** a resume is generated with projects enabled, **Then** no empty projects heading appears in the output and generation succeeds.

---

### User Story 3 - Enable or disable optional sections (Priority: P2)

The user wants to turn whole sections on or off — most importantly skills, and equally the projects section — so a single master profile can produce, for example, a skills-heavy engineering resume and a compact skills-free one, without editing the master profile.

**Why this priority**: Same settings surface as Story 1, small increment, but it changes the document's section set rather than just its lengths, so it is separable and testable on its own.

**Independent Test**: Disable the skills section, generate a resume, confirm no skills section is present anywhere in the output and that no other section's content was lost. Re-enable, regenerate, confirm skills return.

**Acceptance Scenarios**:

1. **Given** the skills section is disabled, **When** a resume is generated, **Then** the output contains no skills section and no orphan heading.
2. **Given** the skills section is disabled, **When** a resume is generated, **Then** all other sections (summary, experience, education, and any others in the master) are still present in the master's order.
3. **Given** a section is disabled, **When** the resume is checked against the master profile for grounding, **Then** the omission is not reported as a structural violation.
4. **Given** the user re-enables a previously disabled section, **When** the next resume is generated, **Then** the section reappears with its content tailored as normal.

---

### User Story 4 - Discover and reset the configuration (Priority: P3)

The user wants to see the current generation settings, understand what each one does and what its allowed range is, and reset back to defaults after experimenting.

**Why this priority**: Usability polish on top of a working configuration system; the feature is functional without it.

**Independent Test**: Open the settings surface, confirm every configurable value is shown with its current value and allowed range, change several, then reset and confirm all values return to documented defaults.

**Acceptance Scenarios**:

1. **Given** settings have never been changed, **When** the user views resume generation settings, **Then** every configurable value is shown with its default and its allowed range.
2. **Given** the user has changed several settings, **When** they reset to defaults, **Then** all values return to the documented defaults and the next generated resume matches default behaviour.
3. **Given** the user enters a value outside the allowed range, **When** they save, **Then** the save is rejected with a message naming the offending setting and its valid range, and no partial change is stored.

---

### Edge Cases

- **Requested content exceeds what the master profile holds**: the user asks for 10 bullets per role but a role has 4. The system must not fabricate; it uses the available content and records that the target was not reachable.
- **Conflicting settings**: a 1-page target combined with maximum-length settings for every section. The page target is the tie-breaker — content is condensed toward the page goal even where that means falling below a configured length range — and the conflict is recorded in the generation record.
- **Page target unreachable**: after the allowed number of adjustment attempts the document still misses the page target. Generation completes with the best result achieved rather than failing, and the final page count is reported.
- **All optional sections disabled**: the resume still renders with the mandatory identity/contact block and whatever sections remain enabled; generation does not fail.
- **Projects requested but master projects lack achievement bullets**: entries render with name/link/dates only.
- **Configured project count exceeds available projects**: all available projects are included; no placeholders are invented.
- **Settings changed while a generation job is in flight**: the in-flight job completes with the settings that were in effect when it started; new settings apply to the next job.
- **Range collapses to a single value** (min = max): treated as an exact target, not an error.

## Requirements *(mandatory)*

### Functional Requirements

#### Configuration surface

- **FR-001**: The system MUST expose a resume generation configuration that a user can read and update without changing code or restarting the system.
- **FR-002**: Configuration changes MUST persist across restarts and apply to all resume generations started after the change.
- **FR-003**: The system MUST supply documented defaults for every configurable value, and those defaults MUST reproduce today's generation behaviour so existing users see no change until they opt in.
- **FR-004**: The system MUST validate every submitted value against a published allowed range and reject the whole update — storing nothing — when any value is invalid, naming the offending setting and its valid range.
- **FR-005**: Users MUST be able to reset the configuration to its documented defaults in one action.
- **FR-006**: Each generated resume MUST record which configuration values it was produced with, so a past result can be explained after settings change.

#### Configurable dimensions

- **FR-007**: Users MUST be able to configure the target length of the summary section, expressed as an approximate line count or a min–max range. The generated summary MUST land close to the configured target; exact matching is not required.
- **FR-008**: Users MUST be able to configure the amount of content in the skills section (how many skill groups and how much detail per group is retained).
- **FR-009**: Users MUST be able to disable the skills section entirely, so it is absent from the generated document.
- **FR-010**: Users MUST be able to configure the number of achievement bullets per work-experience entry as a min–max range (e.g. 5-8), and the generated resume MUST respect that range wherever the master profile supplies enough content.
- **FR-011**: Users MUST be able to configure a preferred page count for the generated resume.
- **FR-012**: Users MUST be able to enable or disable the projects section and configure how many projects are included, as a min–max range (e.g. 3-4).
- **FR-013**: Users MUST be able to configure the number of achievement bullets shown per project.

#### Generation behaviour

- **FR-014**: The system MUST steer generated content toward the configured lengths, treating them as approximate targets where an approximate match preserves readability (summary length, bullets per entry), and enforcing them as hard limits where the value is a count or a switch (section enabled/disabled, project count, bullets per project).
- **FR-015**: The system MUST drive its length-adjustment loop (lengthen when short, condense when long) toward the configured page count rather than a fixed page count.
- **FR-016**: When the page target conflicts with configured section lengths, the system MUST prioritise the page target and record the conflict on the generation.
- **FR-017**: The system MUST NOT fabricate content to satisfy any configured minimum; when the master profile has less content than requested, it MUST use what exists and record the shortfall.
- **FR-018**: The system MUST include the projects section in generated resumes when enabled, reproducing each project's name, link and dates from the master profile unchanged, and selecting and phrasing achievement bullets only from that project's own master content.
- **FR-019**: The system MUST place the projects section in the position the master profile defines, consistent with existing section-order preservation.
- **FR-020**: Disabling a section MUST NOT be reported as a structural or grounding violation, while all other structure and grounding checks continue to apply unchanged.
- **FR-021**: When the allowed adjustment attempts for hitting the page target are exhausted, the system MUST return the best result achieved rather than failing, and MUST report the final page count.

### Key Entities

- **Resume Generation Configuration**: the user-level set of preferences governing generated resume shape — summary length target, skills section enablement and volume, bullets-per-experience range, preferred page count, projects section enablement, project count range, and bullets-per-project. Has documented defaults and per-value allowed ranges.
- **Section Setting**: the per-section slice of the configuration — whether the section is enabled, and how much content it should carry (a count, a range, or an approximate target).
- **Generation Record**: the existing per-generation trace, extended to capture the configuration values used, any target that could not be met (content shortfall, page-count miss), and any resolved conflict between page target and section lengths.
- **Project Entry**: an item in the master profile's projects section — name, link, start/end dates, and achievement bullets — reproduced into generated resumes subject to the configured project and bullet counts.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can change any configurable value and see it reflected in the next generated resume without any code change, config-file edit, or restart.
- **SC-002**: With defaults unchanged, resumes generated after this feature ships match the shape of resumes generated before it (same summary length band, same bullet density, same page count) in at least 9 of 10 sampled generations.
- **SC-003**: For a configured bullets-per-entry range, at least 90% of generated experience entries fall inside the range whenever the master profile holds enough content for that entry; entries that fall short are explained by a recorded shortfall.
- **SC-004**: Generated summaries land within ±1 line of the configured target in at least 80% of generations.
- **SC-005**: The preferred page count is achieved in at least 80% of generations; every miss reports the final page count and the reason.
- **SC-006**: Disabling a section removes it from 100% of subsequently generated resumes, with no other section's content lost.
- **SC-007**: With projects enabled, 100% of generated resumes contain a number of projects inside the configured range (or all available projects when fewer exist), with names, links and dates identical to the master profile.
- **SC-008**: Zero fabricated content is introduced by any configuration value — grounding checks pass at the same rate as before this feature.
- **SC-009**: A user can discover every configurable value, its current setting and its allowed range from a single place, and restore defaults in one action.

## Assumptions

- Configuration is global to the single-user self-hosted deployment; per-vacancy or per-profile overrides are out of scope for this feature.
- The master profile remains the sole source of resume content; this feature changes only how much of it is selected and how it is shaped, never what it contains.
- "Lines" in the summary setting is an approximate proxy for length — the system may internally target sentences or characters — because the user explicitly accepted a close rather than exact match.
- Projects already exist in the master profile format and pass through the current pipeline verbatim; this feature adds selection/limiting on top rather than introducing projects to the profile schema.
- Preferred page count is bounded to a small sensible range (1-3 pages); values outside that are rejected by validation.
- Section content targets are enforced through a combination of instruction to the generation step and post-generation checking; a small residual variance is acceptable, which is why success criteria are stated as percentages rather than absolutes.
- The existing page-count adjustment loop (expand when short, condense when long) is reused and parameterised; no new rendering strategy is introduced.
- Configuration is exposed through the existing settings surface used by other user-tunable behaviour, following the same read/update patterns.
- Existing structure-preservation and grounding guarantees remain in force; deliberate omissions from disabled sections are the only new exemption.
