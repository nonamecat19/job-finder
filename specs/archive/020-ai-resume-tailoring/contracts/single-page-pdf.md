# Contract: Single-Page PDF Renderer

The single-page PDF fitter (`internal/generation/singlepage`) is the only new server-side rendering surface introduced by feature 020. Strict contract: given a finalized `dto.Resume` (the 009 structured shape), produce exactly one page of selectable text PDF, or report a structured "blocked" reason.

## Inputs

| Field | Type | Notes |
|-------|------|-------|
| `resume` | `dto.Resume` | The finalized resume (master + accepted proposals). Always non-nil. |
| `draftID` | `uuid.UUID` | For artifact naming and audit logging. |
| `outDir` | `string` | Where to write the PDF (matches existing `HtmlPdfRenderer.outDir` default `/data/documents`). |
| `store` | `storage.Store` (optional) | MinIO upload if configured (mirrors existing renderer behavior). |

## Outputs

```go
type FitResult struct {
    Status      FitStatus   // fit|blocked|error
    DocumentID  *uuid.UUID  // generated_documents row id on fit
    FilePath    string      // local pdf path on fit
    Feedback    []ExportBlockDto  // populated on blocked
    PageCount   int         // always 1 on fit
    MeasuredMM  int         // content height in mm at the chosen density; for telemetry
    DensityUsed DensityCfg   // the density ladder step that fit (see below)
}
type FitStatus string
const (
    FitStatusFit     FitStatus = "fit"
    FitStatusBlocked FitStatus = "blocked"
    FitStatusError   FitStatus = "error"
)
type DensityCfg struct {
    BodyFontPt   float64  // 11.0 → 9.0 step 0.5
    MarginMM     float64  // 14 → 8  step 2
    LineHeight   float64  // 1.4 → 1.1 step 0.1
    BulletGapPx  int      // 4 → 0  step 1
}
```

## Density ladder (deterministic)

Ordered largest → smallest (cheapest-first search). The fitter tries each in order; first one that fits wins.

```
[
  {bodyFontPt: 11.0, marginMM: 14, lineHeight: 1.4, bulletGapPx: 4},
  {bodyFontPt: 11.0, marginMM: 14, lineHeight: 1.3, bulletGapPx: 4},
  {bodyFontPt: 11.0, marginMM: 14, lineHeight: 1.3, bulletGapPx: 2},
  {bodyFontPt: 10.5, marginMM: 12, lineHeight: 1.3, bulletGapPx: 2},
  {bodyFontPt: 10.5, marginMM: 10, lineHeight: 1.2, bulletGapPx: 2},
  {bodyFontPt: 10.0, marginMM: 10, lineHeight: 1.2, bulletGapPx: 1},
  {bodyFontPt: 10.0, marginMM: 8,  lineHeight: 1.2, bulletGapPx: 1},
  {bodyFontPt: 9.5,  marginMM: 8,  lineHeight: 1.1, bulletGapPx: 0},
  {bodyFontPt: 9.0,  marginMM: 8,  lineHeight: 1.1, bulletGapPx: 0},  // minimum bound
]
```

Bounds (Assumptions:148):
- **Page**: A4 (210 × 297 mm). Fixed for v1; configurable page size is out of scope.
- **Body**: sans-serif Tailwind/system stack; ATS-clean (no fonts that render as images).
- **Density bounds**: minimum is the last row above; anything below is rejected as `blocked` to stay ATS-readable.

## Measurement protocol (chromedp)

For each `DensityCfg` step:
1. Render `resume.html` template with the density knobs exposed as CSS custom properties on `:root` (`--fs`, `--m`, `--lh`, `--bg`).
2. `chromedp.Navigate("about:blank")`; `page.SetDocumentContent(html)` (same pattern as `pdf_renderer.go:185-190`).
3. `chromedp.Runtime.evaluate` `({heightPx: document.documentElement.scrollHeight, widthPx: document.documentElement.scrollWidth})`.
4. Compute `printable_height_mm = 297 − 2×DensityCfg.marginMM`. Convert measured `heightPx` to mm using the page's CSS px → mm ratio (96dpi → 1px = 0.2645mm; verified by the test suite).
5. **Fit** if `measuredMM ≤ printable_height_mm` AND `widthPx ≤ printableWidthPx` (no horizontal overflow).
6. Otherwise advance to the next density step.
7. On the minimum-bound step with no fit, **return `blocked`** with computed feedback.

Synchronous budget: 5s per fit attempt; if exceeded (rare on a one-page resume), kick off the remaining steps as a background task and return `pending` to the caller (`POST /export-pdf` returns 200 with `status:"pending"`). The dashboard polls `/export-status`.

## Blocked feedback generation

When `blocked`, produce `[]ExportBlockDto` ranked by space saved:
1. **Longest bullets** — `experience:Acme:3` (bullet index), each ranked by length; suggestion `"shorten or drop this bullet to gain ~<mm>"`.
2. **Skill-group removals** — `skill_group:Cloud`; suggestion `"consider removing this skill group to gain ~<mm>"`.
3. **Summary** — `summary`; suggestion `"consider shortening the professional summary"`.

Gains are computed from each candidate's measured height at the minimum density step, removed one at a time, until the residual content fits; the `blocks` array lists the shortest set whose removal achieves fit.

## Text-PDF guarantee

- `page.PrintToPDF().WithPrintBackground(true)` (matches existing `pdf_renderer.go:196`) emits true text PDFs.
- No `<canvas>`, no `<img>` of text, no `user-select:none`. The template renders text in real `<p>`/`<ul>` elements; ATS parsers extract every word.
- Verified by an integration test that opens the produced PDF with `pdfcpu` (or equivalent) and asserts `pageCount == 1` AND text extraction returns the expected profile name + summary + first bullet.

## HTML template contract

`apps/api/internal/generation/singlepage/template.go` embeds `resume.html` (new). It renders only from `dto.Resume` — **does not import** the legacy `resume_view` (JSON-Resume) struct. Required sections (matching 009's `Resume.Sections` semantics):
- Header: `resume.name` + (optional) `resume.headline`, `resume.location`, `resume.email`, `resume.phone`, `resume.website`, `resume.socialNetworks`, `resume.customConnections`.
- For each `Section` (in declared order): section name heading, then entries by `entryType`:
  - `experience`/`education`/`publication` — institution/area/degree/company/position/dates/summary/highlights.
  - `normal`/`text`/`bullet`/`numbered`/`reversed_numbered`/`one_line` — text/bullet content.
  - `unrecognized` entries — render raw fields under `entry.unrecognized` (per 009 data-model).
- Skill groups: rendered as `cv.sections.skills` (label + comma-separated details). Each group gets one row; the template never strips tokens.

The template is ATS-clean: single document flow, no columns, no flexbox layouts that paper-print can't replicate. Margins/floating/skill-order are stable across the density ladder.

## What it does NOT do

- Does **not** render the user's RenderCV Typst theme (cookie-cutter, ATS-clean layout only). The user opts into a tailored resume specifically to deviate from the master theme.
- Does **not** fall back to `RenderCvRenderer`. Two distinct export paths exist in the product: the legacy `POST /documents/tailor` merge-and-render (unchanged) and the new review-gated `/export-pdf` (this renderer).
- Does **not** accept the master's `RendercvMaster` opaque map; only `dto.Resume`. The fitter is concerned with the structured shape, not the YAML round-trip.

## Test surfaces

- `fitter_test.go` — unit: density-ladder bounds, `DensityCfg` ordering, blocking-feedback ranking algorithm.
- `singlepage/integration_test.go` (`//go:build integration`) — real chromedp against a fixture `dto.Resume`:
  - content that fits at default density → `status=fit`, `pageCount=1`.
  - content that forces step 5 → `status=fit`, `densityUsed.BodyFontPt=10.5`.
  - content that cannot fit at minimum density → `status=blocked`, `feedback` non-empty and ranked by saved mm.
  - produced PDF: assert `pageCount==1`, content text exists, `widthPx ≤ printableWidthPx`.