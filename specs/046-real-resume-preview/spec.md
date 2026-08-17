# Feature Specification: Real Resume Preview on the Generate Page

**Feature Branch**: `046-real-resume-preview`

**Created**: 2026-08-14

**Status**: Draft

**Input**: User description: "the preview resume on generate page should be real. you can use wasm and rendercv-go to build pdf in browser"

## Overview

Today the Generate page's preview pane is a hand-built HTML approximation of the selected resume sections — it deliberately does not reflect the actual rendered document (no true pagination, fonts, spacing, or theme layout). Users only see what their resume truly looks like after they export it and download the PDF, which is a separate, slower, round-trip step. This feature replaces the approximate preview with a real, accurate rendering of the resume document, generated directly in the browser as the user works, so what they see while editing is what they get when they download.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Preview matches the final PDF (Priority: P1)

A user is on the Generate page reviewing a generated resume. The preview pane shows the resume exactly as it will appear in the downloaded PDF: same fonts, spacing, section layout, and page breaks — not an approximated re-styling of the section text.

**Why this priority**: This is the entire point of the feature. An inaccurate preview misleads users about length, layout, and formatting until they've already spent an export cycle finding out the truth.

**Independent Test**: Open a generation run with a known resume document, compare the preview pane's rendered output against the PDF produced by the existing export/download flow for the same document, and confirm they match in content, section order, pagination, and visual layout.

**Acceptance Scenarios**:

1. **Given** a generation run with a rendered resume document, **When** the user opens the Generate page, **Then** the preview pane displays a rendering of that document that matches the exported PDF's layout and pagination.
2. **Given** a resume that spans multiple pages, **When** the preview renders, **Then** the preview shows the same page count and page breaks as the downloaded PDF.
3. **Given** a resume using a specific theme or locale, **When** the preview renders, **Then** the preview reflects that theme's and locale's formatting, matching the exported PDF.

---

### User Story 2 - Preview updates as the user edits (Priority: P2)

A user changes which sections or entries are included in the resume (e.g., toggling a project, editing summary text). The preview pane updates to reflect the change without the user needing to export first.

**Why this priority**: Immediate, accurate feedback on edits is what makes the preview useful during the editing workflow rather than just at the end.

**Independent Test**: Toggle a section or edit content in the generation workspace and confirm the preview re-renders to reflect the change without requiring an export action.

**Acceptance Scenarios**:

1. **Given** the preview pane is showing a rendered resume, **When** the user changes the selected sections or content, **Then** the preview updates to reflect the new content within a few seconds.
2. **Given** an edit that changes the document's length, **When** the preview re-renders, **Then** the displayed page count updates accordingly.

---

### User Story 3 - Preview stays usable when rendering fails or is unsupported (Priority: P3)

A user's browser cannot render the preview (rendering error, missing capability, or slow/failed load of the rendering engine). The page remains usable and the user is told the preview isn't available rather than seeing a blank or broken pane.

**Why this priority**: The rest of the Generate page (editing, export, download) must keep working even if in-browser rendering fails for a given user or document.

**Independent Test**: Force a rendering failure (e.g., malformed document data) and confirm the page shows a clear fallback state instead of crashing or silently showing stale/blank content.

**Acceptance Scenarios**:

1. **Given** the resume document cannot be rendered, **When** the preview attempts to render, **Then** the pane shows a clear error/fallback message instead of a blank or broken preview.
2. **Given** the browser does not support running the in-browser renderer, **When** the user opens the Generate page, **Then** the page still loads and remains functional, with the preview pane indicating that live preview isn't available.
3. **Given** a rendering failure in the preview pane, **When** the user still wants a PDF, **Then** the existing export/download flow remains available and unaffected.

---

### Edge Cases

- What happens the first time the in-browser renderer loads (before it's ready) — does the pane show a loading state rather than nothing?
- How does the system handle a resume document that is empty or has no sections selected?
- How does the system handle rapid, successive edits (e.g., fast typing) without spamming re-renders or showing flickering intermediate states?
- What happens if the document data sent to the preview is invalid or incomplete (e.g., mid-generation)?
- How does the preview behave for resumes long enough to produce many pages — does the user get a way to scroll/page through them?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The Generate page MUST display a preview of the resume document that visually matches the document produced by the existing export/download flow (same content, section order, pagination, theme, and locale formatting).
- **FR-002**: The preview MUST be generated without requiring the user to complete a full export/download action first.
- **FR-003**: The preview MUST automatically refresh when the user changes the selected sections or content of the generation run, without a manual "refresh preview" step.
- **FR-004**: The preview MUST accurately reflect the document's true page count and page breaks.
- **FR-005**: The preview pane MUST show a loading indicator while the renderer is initializing or a render is in progress.
- **FR-006**: The preview pane MUST show a clear error state, distinct from the loading state, when a render fails, and MUST NOT silently display stale or blank content as if it were current.
- **FR-007**: The rest of the Generate page (editing controls, existing export/download flow) MUST remain fully usable if the preview fails to render or is unavailable.
- **FR-008**: The preview MUST support all resume themes and locales the export/download flow supports.
- **FR-009**: For multi-page resumes, the preview MUST let the user view all pages (e.g., scroll or page through them), not just the first page.
- **FR-010**: Repeated, rapid edits MUST NOT cause the preview to flicker or fall behind — the system MUST coalesce updates so the preview settles on the latest edit within a short, bounded delay.

### Key Entities

- **Generation Run**: The in-progress resume generation session on the Generate page, including the currently selected sections/content that the preview reflects.
- **Resume Document**: The structured resume content (sections, entries, theme, locale) that both the preview and the final export render from — the source of truth for what the preview must match.
- **Preview Render**: The up-to-date visual rendering of the Resume Document shown in the preview pane, tied to the current state of the Generation Run.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can see an accurate preview of their resume's real layout and page count without ever needing to export/download just to check formatting.
- **SC-002**: The preview's page count and layout match the downloaded PDF for 100% of supported themes and locales.
- **SC-003**: After an edit, the preview reflects the change within 3 seconds for at least 95% of edits.
- **SC-004**: Preview rendering failures never prevent the user from completing an export/download.
- **SC-005**: In usability review, users report the preview as trustworthy — i.e., what they see in the preview is what they get in the downloaded PDF — for at least 95% of sampled resumes.

## Assumptions

- The existing export/download flow (which produces the authoritative, downloadable PDF) is unchanged by this feature; this feature replaces only the live preview shown before export.
- The preview renders from the same document data and rendering logic already used to produce the final PDF, so visual parity is achievable without maintaining two separate rendering implementations.
- A small minority of users may be on browsers that cannot run the in-browser renderer; for them, a fallback message is acceptable and the export/download flow remains their path to a PDF.
- "A few seconds" / "3 seconds" for preview refresh is an acceptable default responsiveness target for this editing workflow, consistent with typical in-app preview experiences.
- This feature covers the resume preview pane on the Generate page specifically; other document types are out of scope unless they already share this same preview component.
