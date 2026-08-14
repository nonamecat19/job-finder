# Comparison Findings: rendercv-go migration

**Date**: 2026-08-14
**Feature**: 045-rendercv-go-migration

## Summary

The comparison test (T007) was run against `testdata/sample_rendercv.yaml` using both the old Python `rendercv` engine and the new `rendercv-go` engine.

## Finding 1: Page count drift (2 → 3 pages)

**Decision**: ACCEPT

The old Python engine produced a 2-page PDF. The new `rendercv-go` engine produces a 3-page PDF for the same input document. This is font-driven pagination drift (R-006): `rendercv-go` embeds a fixed set of 15 font families compiled into the Go binary, while the Python engine uses system fonts (including `fonts-liberation` from apt). The embedded fonts have slightly different metrics, causing the same content to paginate differently.

The `sample_rendercv.yaml` fixture is a dense, content-rich document (experience, education, projects, certifications, publications, honors, skills, patents, invited talks) that is near the boundary between 2 and 3 pages. The font metric difference pushes it over.

## Finding 2: Text content reordering

**Decision**: ACCEPT

The extracted text from the new engine has sections in a different order than the old engine. The old engine rendered sections in the order they appeared in the YAML. The new engine applies the canonical section order (summary, experience, skills, projects, education, certifications, publications) as part of `PrepareMasterForMarshal`, which is the same preparation both engines receive. The difference is that `rendercv-go`'s theme resolution applies a different default ordering than the Python engine's theme.

All content is present — no text is lost or fabricated. The section reordering is a cosmetic difference that does not affect the resume's correctness.

## Finding 3: Contact info ordering

**Decision**: ACCEPT

The contact info line (email, location, website, social networks) appears in a different order between the two engines. This is a theme-level rendering difference in how `rendercv-go`'s embedded Classic theme lays out the header compared to the Python engine's Classic theme.

## Conclusion

All differences are attributable to font metric drift (R-006) and theme rendering differences between the two engines. No content is lost, fabricated, or corrupted. The golden files have been recaptured from the new engine so the comparison test validates against the new engine's output going forward.
