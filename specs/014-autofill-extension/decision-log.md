# Decision Log: No Auto-Submit Constraint

**Decision**: The autofill extension MUST NEVER call `form.submit()`, click a submit
control, or otherwise programmatically submit an application form. The extension fills
fields; the user clicks submit.

**Category**: Product line (hard constraint)

**Date**: 2026-07-20

**Task/s**: el-5yt8 (014-1), el-5pue (014-5)

---

## Context

The extension was designed to fill fields from the user's stored profile. In early
discussion, auto-submit — filling AND submitting in one click — was considered as a
convenience feature. It would have saved the user one click per application.

## Decision

Auto-submit is rejected. The extension never:

- Calls `form.submit()` in JavaScript
- Programmatically clicks any button or input of type `submit`
- Dispatches a `submit` event on any form
- Sets `window.location` or triggers a navigation that would cause form transmission

## Rationale

1. **Wrong-field liability**: If the extension fills an incorrect field or value
   (ambiguous mapping, misidentified field), auto-submit means the user cannot correct it.
   The application is sent wrong without review.

2. **Trust erosion**: A single bad auto-submit erodes user trust in the entire
   extension. Manual review before submit preserves trust even after a mapping error.

3. **No post-submit observability**: The extension cannot reliably observe what happens
   after submit (redirects, multi-page flows, CAPTCHAs, errors). This makes auto-submit
   fragile across ATS implementations.

4. **Product differentiation**: The extension is a productivity tool, not a bot. The
   user stays in control of the application action. This aligns with the product's ethos
   (see also: draft-only outreach in 012, no LinkedIn scraping in 011).

## Enforcement

- Automated test in 014-5 asserts no `form.submit()` or submit-click code path exists in
  the extension bundle
- Code review gate: any PR adding a form-submission code path is rejected
- This decision-log entry survives future tasks so the constraint is documented even if
  the implementation team changes
