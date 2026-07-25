# Feature Specification: Fully Editable Resume Profile Tab

**Feature Branch**: `009-editable-resume-profile`

**Created**: 2026-07-25

**Status**: Draft

**Input**: User description: "rewrite profile tab, resume should be fully editable. the config file can be optionally used to set all required fields with data but its not mandatory. all UI should be elegand and have awesome UX"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Build a resume from scratch with no config file (Priority: P1)

A user with no existing RenderCV config opens the Profile tab and builds a complete resume by hand: name, contact details, and every section (experience, education, skills, projects, publications, patents, talks, custom sections) using guided forms.

**Why this priority**: This is the core promise of the feature — the config file becomes optional, not a prerequisite. Without this, the tab is still upload-only and the feature has no value.

**Independent Test**: Starting from a profile with no config, a user can add at least one entry to every supported section type, save, and see the data persisted and correctly summarized — without ever touching a YAML file.

**Acceptance Scenarios**:

1. **Given** a profile with no config data, **When** the user opens the Profile tab, **Then** they see an empty, clearly-labeled editable resume (not an error or blank/broken state) with an obvious way to add content to each section.
2. **Given** an empty resume, **When** the user adds a new experience entry with company, position, dates, and highlights, **Then** the entry is saved and appears in the experience list immediately.
3. **Given** an empty resume, **When** the user adds entries to a section type such as publications, patents, or invited talks, **Then** each uses a form suited to that entry type's fields (not a generic free-text blob).
4. **Given** unsaved edits in progress, **When** a save fails (e.g., network error), **Then** the user sees a clear error and their in-progress edits are not lost.

---

### User Story 2 - Import a config file to pre-fill, then keep editing (Priority: P2)

A user uploads an existing RenderCV YAML config to pre-fill their resume, then continues editing individual fields and entries directly in the UI rather than re-uploading a file for every change.

**Why this priority**: Preserves the existing import path as a convenience/shortcut, but establishes that the file is a starting point, not the system of record — editing must not require re-uploading.

**Independent Test**: Upload a sample config, verify all its fields populate the editable forms correctly, then edit one field directly in the UI and confirm the file is not needed again to persist that change.

**Acceptance Scenarios**:

1. **Given** a profile with no data, **When** the user uploads a valid RenderCV YAML file, **Then** every field and section entry from the file appears pre-filled in the corresponding editable form.
2. **Given** a profile pre-filled from an uploaded config, **When** the user edits a field (e.g., changes a job title) and saves, **Then** the change persists without requiring another file upload.
3. **Given** an existing profile with prior manual edits, **When** the user uploads a new config file, **Then** the user is warned that this will replace current resume content before the import proceeds.

---

### User Story 3 - Manage resume structure (add, edit, delete, reorder) (Priority: P2)

A user reorganizes their resume: reordering entries within a section, reordering sections themselves, editing any field inline, and deleting entries or entire sections they no longer want.

**Why this priority**: "Fully editable" requires structural control, not just field edits — this is what makes the resume actually maintainable over time as a living document.

**Independent Test**: Starting from a resume with multiple sections and multiple entries per section, a user can reorder entries within a section, reorder sections, delete an entry, and delete a whole section, with each change reflected and persisted.

**Acceptance Scenarios**:

1. **Given** a section with multiple entries, **When** the user reorders them (e.g., drag or move up/down), **Then** the new order is saved and reflected on reload.
2. **Given** a resume with multiple sections, **When** the user reorders the sections, **Then** the new section order is saved and reflected on reload.
3. **Given** an entry the user no longer wants, **When** they delete it, **Then** they are asked to confirm, and upon confirmation the entry is removed and the deletion persists.
4. **Given** a section the user no longer wants, **When** they delete the whole section, **Then** they are asked to confirm, and upon confirmation the section and its entries are removed.
5. **Given** a resume with no sections, **When** the user adds a brand-new section (including a custom/non-standard section type), **Then** they can name it and begin adding entries to it.

---

### Edge Cases

- What happens when a user uploads a config file that is malformed YAML or missing the required `cv.name` field? System must reject the import with a clear, specific error and leave existing resume data untouched.
- What happens when a user uploads a config file containing section entry types or fields the UI doesn't recognize? Unrecognized data must be preserved (not silently dropped) and surfaced in a fallback editor so it isn't lost, even if not fully structured.
- How does the system handle a save attempt with invalid data (e.g., an end date before a start date, an empty required field like a section's entries missing a name)? Inline validation must block the save and point to the specific offending field.
- What happens if two browser tabs/sessions edit the same profile concurrently? Last-write-wins is acceptable for v1; no requirement for real-time conflict resolution.
- What happens when a user deletes every section and every field, leaving a fully empty resume? This must be an allowed, non-error state (not a broken/blank screen).
- What happens when the resume has a very large number of entries (e.g., 50+ experience entries)? The editing UI must remain responsive and usable (no pagination requirement, but no freezing).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Users MUST be able to create and fully populate a resume (name, headline, contact/location/social links, and all resume sections) directly in the Profile tab without ever uploading a config file.
- **FR-002**: Uploading a config file MUST remain optional and MUST act only as a pre-fill/import convenience — no functionality may require a config file to be present.
- **FR-003**: System MUST provide a dedicated, structured editing form for every RenderCV entry type, including at minimum: experience, education, skills, projects, publications, patents, and invited talks.
- **FR-004**: System MUST allow users to add, edit, and delete individual entries within any section.
- **FR-005**: System MUST allow users to add, rename, and delete whole sections, including sections with custom/non-standard names not part of the predefined type list.
- **FR-006**: System MUST allow users to reorder entries within a section and reorder sections themselves, and persist the resulting order.
- **FR-007**: System MUST validate resume data inline (e.g., required fields, date ordering) before saving, and surface field-specific error messages without discarding the user's other in-progress edits.
- **FR-008**: System MUST auto-save or explicitly save user edits reliably, with clear visual confirmation of save success or failure.
- **FR-009**: System MUST preserve and surface, via a fallback editor, any resume data present in an uploaded config that doesn't match a recognized structured entry type, so no imported data is silently lost.
- **FR-010**: When a user uploads a new config file to a profile that already has resume content, system MUST warn that existing content will be replaced and require confirmation before overwriting.
- **FR-011**: System MUST require confirmation before destructive actions (deleting an entry or a whole section).
- **FR-012**: System MUST support a profile with zero sections and zero fields as a valid, non-error state.
- **FR-013**: Editing UI MUST present a clean, uncluttered layout with clear visual hierarchy (distinguishing section-level actions from entry-level actions), consistent with modern resume-builder UX conventions (e.g., add/edit/delete affordances that are discoverable without instruction).

### Key Entities

- **Resume Profile**: A user's editable resume — top-level identity/contact fields (name, headline, location, email, phone, website, social links) plus an ordered list of Sections. Maps to the existing profile record; today only name/extraNotes are structured, this feature makes the full resume structured and editable.
- **Section**: A named, ordered group of Entries of a single type (e.g., "Experience", "Education", or a user-defined custom name). Has a display order relative to other sections.
- **Entry**: A single item within a Section, shape depends on the section's entry type (e.g., an Experience entry has company/position/dates/highlights; an Education entry has institution/degree/dates; a Skills entry has a skill group and items; a Publication entry has authors/title/venue/date, etc.). Has a display order relative to other entries in its section.
- **Imported Config**: The optional uploaded RenderCV YAML file, used once at import time to pre-fill a Resume Profile's fields, Sections, and Entries. Not authoritative after import — subsequent edits happen directly on the Resume Profile.
- **Unrecognized Data**: Any fields or entry data from an Imported Config that don't map to a known structured Entry shape; retained and shown via a fallback editor rather than discarded.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can build a complete, multi-section resume entirely by hand (no config upload) in under 15 minutes.
- **SC-002**: 100% of fields and section entries present in a valid uploaded config file are correctly pre-filled into editable forms, with zero silent data loss for unrecognized entries.
- **SC-003**: Users can locate and use add/edit/delete/reorder controls for any section or entry without external instructions, on first attempt, in usability observation.
- **SC-004**: Saved edits are reflected correctly on reload 100% of the time under normal network conditions.
- **SC-005**: Destructive actions (entry/section deletion) never occur without an explicit confirmation step, verified across all delete paths.

## Assumptions

- The existing multi-profile model (a user may have several named profiles) is retained; full editability applies per-profile, using the currently selected profile.
- Visual/theme/design settings (fonts, colors, layout template) from the RenderCV `design` block remain out of scope for this feature; this spec covers resume *content* editing only.
- No live rendered preview (PDF/visual layout) is required as part of this feature; users rely on the structured form view while editing.
- Export/download of the resume as YAML or PDF is handled by existing/adjacent generation functionality and is not being changed by this feature, beyond ensuring the underlying data it reads remains accurate.
- Auto-save vs. explicit save-button is an implementation detail; either satisfies FR-008 as long as save state is clearly communicated to the user.
- "Elegant UI and awesome UX" is interpreted per SC-003 and FR-013: clear hierarchy, discoverable controls, and low friction for common edits, evaluated via usability observation rather than a specific visual style mandate.
