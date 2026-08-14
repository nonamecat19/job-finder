# Feature Specification: Replace the Python RenderCV dependency with rendercv-go

**Feature Branch**: `045-rendercv-go-migration`

**Created**: 2026-08-14

**Status**: Draft

**Input**: User description: "lets change python implementation of rendercv library to ../rendercv-go/"

## Overview

Resume PDFs are produced today by handing a RenderCV YAML document to the upstream Python `rendercv` tool, which the API invokes as an external command. That tool is the only reason the deployed API image carries a Python runtime, a virtual environment, and the Typst toolchain that ships with it. `rendercv-go` is a complete, parity-tested reimplementation of the same tool that runs natively inside the API process with no external runtime of any kind.

This feature swaps the rendering engine underneath every path that produces a resume document. Nothing about what a user asks for, uploads, or downloads changes: the same YAML documents in, the same PDFs out.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Generating a tailored resume still produces a correct PDF (Priority: P1)

A user generates a tailored resume for a vacancy. The system builds the RenderCV document as it always has, renders it, and the user downloads a PDF that looks and reads the same as one produced before the change — same sections in the same order, same content, same theme, same locale.

**Why this priority**: This is the primary product output. If it regresses, the feature has no value and the whole application's core promise breaks.

**Independent Test**: Take a set of stored RenderCV documents covering the themes, locales, and entry types the product actually uses, render each with the old engine and the new one, and compare the resulting documents for content and layout equivalence.

**Acceptance Scenarios**:

1. **Given** a valid RenderCV document produced by the generation pipeline, **When** the user requests a resume, **Then** a PDF is produced and made available for download, and the accompanying YAML is stored alongside it as before.
2. **Given** a document that rendered successfully before the change, **When** it is rendered after the change, **Then** it still renders successfully and its page count, section order, and text content match the previous output.
3. **Given** a rendered resume, **When** the system reports its page count for length enforcement, **Then** the count reflects the newly rendered document and length rules continue to apply.

---

### User Story 2 - Invalid documents fail with a usable explanation (Priority: P1)

A malformed or schema-invalid RenderCV document (from an edited profile, an unusual generation result, or an uploaded config) is submitted for rendering. The system rejects it and surfaces an error that identifies what is wrong, rather than an opaque failure.

**Why this priority**: Rendering failures are the main way this subsystem is observed in operation. A migration that degrades error reporting turns every content bug into an unexplained 500.

**Independent Test**: Submit documents with known defects — unknown field, wrong value type, missing required section, invalid date — and confirm each surfaces a message naming the offending field.

**Acceptance Scenarios**:

1. **Given** a document that fails validation, **When** rendering is attempted, **Then** the operation fails, the failure is logged with the validation detail, and no partial PDF is presented as a finished document.
2. **Given** a document that fails validation, **When** the failure reaches the caller, **Then** the error distinguishes "this document is invalid" from "the renderer broke".
3. **Given** a rendering attempt that exceeds its time budget, **When** the budget expires, **Then** the operation is abandoned and reported as a failure rather than hanging.

---

### User Story 3 - The profile smoke render keeps guarding bad profiles (Priority: P2)

When a user's profile changes in ways that feed the resume document, the system performs a throwaway render to confirm the profile can still produce a document. That guard continues to work, using the new engine, without leaving temporary files behind.

**Why this priority**: It protects users from discovering a broken profile only at generation time, but it is a secondary safeguard rather than the primary output path.

**Independent Test**: Save a profile that yields an unrenderable document and confirm the save path reports the problem; save a valid one and confirm it succeeds and leaves no stray artifacts.

**Acceptance Scenarios**:

1. **Given** a profile that produces a valid document, **When** the smoke render runs, **Then** it succeeds and its temporary output is cleaned up.
2. **Given** a profile that produces an invalid document, **When** the smoke render runs, **Then** it fails with the validation detail attached.

---

### User Story 4 - Deployments no longer carry a Python runtime (Priority: P2)

An operator builds and deploys the API. The resulting image contains no Python interpreter, no virtual environment, and no separately installed Typst toolchain for resume rendering, and rendering works identically in that image.

**Why this priority**: This is the operational payoff — a smaller image, one fewer language runtime to patch, and one fewer external process to fail. It is only realizable after Stories 1–3 land.

**Independent Test**: Build the image, confirm the Python-based renderer install is absent, run a render inside the container, and confirm a valid PDF comes out.

**Acceptance Scenarios**:

1. **Given** the deployed image, **When** a resume render runs, **Then** it completes without invoking any external rendering process.
2. **Given** the deployed image, **When** its contents are inspected, **Then** the Python-based resume renderer and its virtual environment are absent.
3. **Given** existing deployment configuration that still sets the old renderer-binary setting, **When** the service starts, **Then** it starts normally and the obsolete setting is ignored rather than causing a startup failure.

---

### Edge Cases

- **Typography drift**: the new engine resolves fonts from a fixed, embedded set rather than from whatever fonts the host image happens to have. A document sitting exactly on a page boundary may paginate differently than before. Documents near a page limit must be checked as part of the comparison in Story 1, and any drift recorded.
- **Documents referencing a photo or other relative path**: paths inside a document resolve relative to the document's own location, not the process working directory. Rendering must keep resolving them the way it does today.
- **Concurrent renders**: several resumes rendering at once must not collide over output paths or shared temporary state.
- **Very large documents**: an oversized profile that previously rendered (slowly) must still render within the operation's time budget, or fail cleanly when it does not.
- **Stored documents from earlier versions**: YAML already stored in the system, including uploaded custom configs, must remain renderable without user edits.
- **Cancellation**: if the caller's request is cancelled mid-render, the render stops and leaves no half-written output presented as complete.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST render resume documents using the Go implementation of RenderCV, in-process, for every path that currently produces a rendered resume.
- **FR-002**: The system MUST accept the same RenderCV document format it accepts today, including documents already stored in the system and configs uploaded by users, with no migration of stored content.
- **FR-003**: The system MUST continue to write both the YAML document and the rendered PDF to the configured document location and upload both to blob storage where it does that today.
- **FR-004**: The system MUST continue to suppress the output formats it does not use, producing the PDF only.
- **FR-005**: The system MUST report a rendering failure caused by an invalid document distinguishably from an internal renderer failure, and MUST include the validation detail (offending field and reason) in the reported error.
- **FR-006**: The system MUST enforce a bounded time budget on a render and MUST abort a render when the initiating request is cancelled.
- **FR-007**: The system MUST continue to determine and report the page count of a rendered resume so existing length rules keep applying.
- **FR-008**: The system MUST perform the profile smoke render with the new engine and MUST remove its temporary output afterwards.
- **FR-009**: The deployed application image MUST NOT install a Python runtime or virtual environment for resume rendering, and MUST NOT require any externally installed rendering or typesetting tool.
- **FR-010**: The system MUST start successfully when deployment configuration still supplies the now-obsolete external-renderer setting, ignoring it rather than failing.
- **FR-011**: Automated tests MUST cover a successful render, a validation failure, and the page-count path, and MUST run without requiring an external renderer to be installed on the machine running them.
- **FR-012**: Documentation describing how resumes are rendered, what the runtime requires, and what configuration exists MUST be updated to match the new arrangement.
- **FR-013**: Any behavioral difference observed between the old and new engine during the comparison in Story 1 MUST be recorded, with a decision to accept or fix it, before the change is considered complete.

### Key Entities

- **RenderCV document**: the YAML description of a resume — personal details, sections, entries, plus design, locale, and render settings. Unchanged by this feature; it is the input contract that must be preserved exactly.
- **Rendered resume**: the PDF produced from a document, along with the stored YAML beside it, addressed by a base name and a page count.
- **Rendering engine**: the component that turns a document into a rendered resume. This feature replaces its implementation while holding its inputs and outputs fixed.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of resume documents in the existing test corpus that rendered successfully before the change still render successfully after it.
- **SC-002**: For every document in that corpus, the extracted text of the new PDF matches the old one, and the page count matches; any exception is individually recorded and accepted under FR-013.
- **SC-003**: Resume rendering completes within the same time budget as before, with no observed increase in rendering failures or timeouts in the week following rollout.
- **SC-004**: The deployed application image no longer contains a Python runtime, and its size is measurably smaller than before the change.
- **SC-005**: A developer can run the full rendering test suite on a clean checkout with no renderer installed beyond the project's own dependencies.
- **SC-006**: Every rendering failure surfaced to an operator during acceptance testing names either the offending document field or an explicit internal-error condition — no unexplained failures.

## Assumptions

- The replacement is consumed as a library used directly by the application rather than as a separate command-line binary invoked as a subprocess. Direct use is what removes the external-process and external-runtime cost that motivates the change; the command-line binary remains available but is not what the application uses.
- The Python renderer is removed outright rather than retained behind a fallback switch. Keeping both would preserve the Python runtime in the image and forfeit the main benefit; the comparison in Story 1 is the safety net instead.
- The replacement is at parity with the version of upstream RenderCV currently in use, so documents targeting today's schema — including the settings block and locale form used by uploaded configs — are accepted unchanged.
- Minor typographic differences arising from the replacement's fixed font set are acceptable provided content, section order, and page counts hold; a page-count change on any corpus document is treated as a finding under FR-013, not a silent acceptance.
- The obsolete external-renderer configuration setting is retired from documentation and defaults, but its continued presence in an operator's environment is tolerated for at least one release rather than treated as an error.
- Chromium remains in the image for the unrelated HTML-to-PDF path, and the PDF text-extraction tooling remains for resume import; neither is affected by this change.
- The replacement module is reachable as a normal dependency from the application's build, at its frozen released version.
