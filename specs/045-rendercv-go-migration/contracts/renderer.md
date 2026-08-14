# Contract: `RenderCvRenderer` — the resume rendering interface

**Feature**: 045-rendercv-go-migration
**Location**: `apps/api/internal/generation/infrastructure/rendercv_renderer.go`
**Re-exported at**: `apps/api/internal/generation/generation.go:22,53`

This is the interface contract the migration holds fixed. Every caller depends on this signature; only the implementation behind it changes (Python subprocess → `rendercv-go` library). No caller outside `infrastructure` is modified.

## Constructor

```go
func NewRenderCvRenderer(outDir string) *RenderCvRenderer
```

- `outDir` — the directory where rendered PDFs and YAML documents are written. Defaults to `/data/documents` when empty. Falls back to `./data/documents` on a permission error (via `ensureOutDir`).
- **Change from before**: the `bin string` parameter is removed. The renderer no longer invokes an external binary; it calls `rendercv-go` in-process. (See FR-010 for tolerance of the obsolete `RENDERCV_BIN` setting.)

## Struct

```go
type RenderCvRenderer struct {
    outDir string
    Store  storage.Blobstore  // exported; set after construction. nil = no upload (smoke render, tests).
}
```

## Render method — the primary contract (UNCHANGED signature)

```go
func (r *RenderCvRenderer) Render(
    ctx context.Context,
    master domain.RendercvMaster,
    baseName string,
) (yamlPath, pdfPath string, err error)
```

### Inputs

| Parameter | Type | Contract |
|---|---|---|
| `ctx` | `context.Context` | The caller's context. The render is bounded by a 120-second timeout derived from it and is aborted when it is cancelled (FR-006). Cancellation does not produce a partial PDF (see Outputs). |
| `master` | `domain.RendercvMaster` (`map[string]any`) | The RenderCV document. Must be a valid, normalized master. `PrepareMasterForMarshal` is applied internally before marshalling to YAML. |
| `baseName` | `string` | The base filename (no extension). The YAML is written to `<outDir>/<baseName>.yaml` and the PDF to `<outDir>/<baseName>.pdf`. |

### Outputs

| Return | Contract |
|---|---|
| `yamlPath` | The absolute path of the written YAML document (`<outDir>/<baseName>.yaml`). Always written before the render begins. |
| `pdfPath` | The absolute path of the rendered PDF (`<outDir>/<baseName>.pdf`). Only returned on success. On cancellation or error, no file exists at this path (the render writes to a temp path and atomically renames on success — no half-written output is ever presented as complete). |
| `err` | See Error contract below. |

### Side effects

1. Ensures `outDir` exists (`ensureOutDir`, with permission fallback).
2. Writes the YAML document to `yamlPath` (permissions `0o644`).
3. Renders the PDF in-process via `rendercv-go` (Build → GenerateTypst → GeneratePDF), writing to a temp path then renaming to `pdfPath`.
4. If `r.Store != nil`, uploads both the YAML (`application/x-yaml`) and the PDF (`application/pdf`) to the blobstore, keyed by the base filename. A blob upload failure returns an error and is the caller's responsibility to handle (unchanged).

### Error contract (FR-005)

| Condition | Error prefix | Distinguishable as |
|---|---|---|
| Invalid document (`*rendercv.UserValidationError` / `*rendercv.UserError`) | `rendercv: invalid document:` | user-facing validation failure — the offending field(s) and reason(s) are in the message; the original typed error is preserved via `%w` |
| Renderer internal failure (`*rendercv.InternalError` or non-typed error from generators) | `rendercv: internal error:` | internal failure — not the user's document |
| Timeout or cancellation | `rendercv: render cancelled:` | cancelled/timed-out — `context.DeadlineExceeded` or `context.Canceled` is wrapped |
| Filesystem error | `rendercv: internal error:` or `rendercv: mkdir:` / `rendercv: write yaml:` | internal failure |

The caller distinguishes "this document is invalid" from "the renderer broke" by the error prefix (or by `errors.As` against `*rendercv.UserValidationError`). No partial PDF is presented on any error path.

### Behavioural invariants

- **Same input, same output format**: a `RendercvMaster` that rendered successfully before the migration still renders successfully after it (FR-002, SC-001). The PDF text and page count match the old output for every document in the comparison corpus, with any exception recorded under FR-013.
- **PDF-only output**: Markdown, HTML, and PNG are suppressed (FR-004). Only the `.typ` (intermediate, temp) and `.pdf` (final) are produced.
- **Page count**: `CountPages(pdfPath)` reads the returned PDF and returns its page count. The render method itself does not return a page count (unchanged — the page-fit loop calls `CountPages` separately).
- **Concurrency**: concurrent `Render` calls do not collide on output paths (temp-path-then-rename with unique temp suffixes).
- **Cancellation safety**: on `ctx.Done()`, the render is abandoned, temp files are best-effort deleted, and no `pdfPath` is returned. The underlying `rendercv-go` call may continue to completion in a leaked goroutine, but its output is discarded and its wazero module is reclaimed when the compile call returns.

## Page count function (UNCHANGED)

```go
func CountPages(pdfPath string) (int, error)
```

Opens the PDF at `pdfPath` via `github.com/ledongthuc/pdf` and returns `r.NumPage()`. Works on any valid PDF regardless of which engine produced it. Called by `defaultRenderDeps`/`exportRenderDeps` and stubbed in the eval harness.

## Callers (all UNCHANGED)

| Caller | File | Usage |
|---|---|---|
| Generation service (legacy job + ad-hoc) | `application/service.go` via `defaultRenderDeps` | `s.rendercv.Render` + `CountPages` in the page-fit loop |
| Workspace export | `application/workspace_export.go` via `exportRenderDeps` | `render` + `countPages` (render-once, no rewording) |
| Profile smoke render | `profile/application/service.go` `SaveConfig` | throwaway `RenderCvRenderer` in a temp dir; `Store` nil; `defer os.RemoveAll` |
| Live test | `infrastructure/rendercv_live_test.go` | renders the sample fixture to a temp dir (no longer needs `rendercv` on PATH) |

## Configuration

| Setting | Before | After |
|---|---|---|
| `DOCUMENTS_DIR` (env, default `./data/documents` or `/data/documents`) | read by `RenderCvRenderer` | unchanged |
| `RENDERCV_BIN` (env, default `rendercv`) | passed to `NewRenderCvRenderer` as `bin` | **obsolete** — loaded into config struct but not read; tolerated for one release (FR-010), then removed |