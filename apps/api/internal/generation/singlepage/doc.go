// Package singlepage implements the ATS-clean single-page PDF fitter for
// tailored resumes (specs/020-ai-resume-tailoring,
// contracts/single-page-pdf.md): it renders a dto.Resume to HTML and uses
// the existing chromedp pipeline to measure the rendered output and
// iteratively adjust density (font size, margins, line height) until the
// content fits exactly one page, or reports a blocked export with
// actionable per-field feedback when it cannot. This is a pure post-accept
// concern, separate from internal/generation's LLM prompt/merge logic and
// from the existing RenderCV-based multi-page export path.
package singlepage
