# Data Model: rendercv-go migration

**Branch**: `045-rendercv-go-migration` | **Date**: 2026-08-14

This feature does **not** introduce, modify, or remove any persisted entity. There is no database migration, no new table, no new column, no sqlc query change, and no DTO change. The `RendercvMaster` document type, the `GeneratedDocument` row, and the profile's `rendercvConfig`/`rendercvYaml` columns are all held fixed — the spec (FR-002) requires that stored documents and uploaded configs remain renderable without migration.

What follows is the in-memory data model of the rendering path: the types that cross the boundary between the API and the `rendercv-go` library, and the error types that classify failures. These are the contracts the implementation must honour.

---

## 1. The input contract — `domain.RendercvMaster` (UNCHANGED)

**Type**: `apps/api/internal/generation/domain/rendercv.go:11`
```go
type RendercvMaster map[string]any
```

A normalized `map[string]any` representing the RenderCV YAML document. Deliberately not a rigid struct so uploaded configs with unknown fields round-trip without data loss (spec FR-002). Keys include `cv` (name, headline, location, email, phone, website, photo, social_networks, sections), `design` (theme, page, typography, colors, margins, layout), and `settings` (current_date, render_command, bold_keywords).

**Preparation**: Before rendering, `domain.PrepareMasterForMarshal(master)` (`prepare_marshal.go:60`) deep-clones, deletes `_order`, embeds entry URLs into names, sorts sections by canonical order (summary, experience, skills, projects, education, certifications, publications), wraps sections in an `OrderedYAMLMap` to preserve order on marshal, and hides the footer (`design.page.show_footer = false`). This function is **unchanged** — it runs before the YAML is handed to the new renderer exactly as it runs before the YAML is handed to the Python CLI today.

**Validation (pre-render)**: `domain.ParseRendercv(yamlText)` validates the `cv` block and `cv.name` exist and preserves section order. The smoke render calls this before `Render`. The generation pipeline calls it during profile/master ingestion. **Unchanged.**

---

## 2. The rendering boundary — `RenderCvRenderer` (signature UNCHANGED, implementation rewritten)

**Type**: `apps/api/internal/generation/infrastructure/rendercv_renderer.go`

The `Render` method signature is held fixed so no caller changes:

```go
func (r *RenderCvRenderer) Render(ctx context.Context, master domain.RendercvMaster, baseName string) (yamlPath, pdfPath string, err error)
```

**Constructor** — signature changes (drops the `bin` parameter):
```go
// BEFORE
func NewRenderCvRenderer(outDir, bin string) *RenderCvRenderer
// AFTER
func NewRenderCvRenderer(outDir string) *RenderCvRenderer
```

**Struct fields** — `bin` removed, `outDir` and `Store` retained:
```go
type RenderCvRenderer struct {
    outDir string
    Store  storage.Blobstore
}
```

**Page count** — package function, UNCHANGED:
```go
func CountPages(pdfPath string) (int, error)
```

---

## 3. The `rendercv-go` types used inside the adapter (internal to `infrastructure`)

These types are **not exported through the `generation` facade**. They live inside `rendercv_renderer.go` and are not named by any caller outside the `infrastructure` package.

### `rendercv.BuildOptions`

**Source**: `github.com/nonamecat19/rendercv-go/pkg/rendercv/options.go:7`

The rewritten `Render` constructs one of these per call:
```go
opts := rendercv.BuildOptions{
    InputFilePath:        yamlPath,           // relative paths resolve against outDir (R-005)
    PDFPath:              tmpPdfPath,          // temp path; renamed to final on success (R-001)
    TypstPath:            tmpTypstPath,        // temp path; intermediate artifact
    DontGenerateMarkdown: true,                // suppress .md (FR-004)
    DontGenerateHTML:     true,                // suppress .html (FR-004)
    DontGeneratePNG:      true,                // suppress .png (FR-004)
    // DontGenerateTypst left false — PDF needs the .typ on disk
    // DontGeneratePDF  left false — we want the PDF
}
```

### `rendercv.Model` (opaque)

**Source**: `pkg/rendercv/types.go:62`

Returned by `rendercv.Build`. Has no exported fields; the only exported method is `Name() string`. Passed to `rendercv.GenerateTypst` and `rendercv.GeneratePDF`. Not stored on the `RenderCvRenderer` — a fresh `Model` is built per `Render` call.

### `rendercv.Document`

**Source**: `pkg/rendercv/types.go:17` — alias for `internal/schema/yamldoc.Node` (the parsed YAML tree). Returned as the first value of `Build`; not used by the renderer (only the `Model` is needed for generation).

---

## 4. Error types — classification (FR-005)

The `Render` method classifies the error from `rendercv.Build` and wraps it so the caller can distinguish "invalid document" from "renderer broke".

### `*rendercv.UserValidationError` — invalid document

**Source**: `pkg/rendercv/types.go:38` (alias of `schemaerr.UserValidationError`)

Carries `Errors []ValidationError`, each with:
- `SchemaLocation []string` — the field path, e.g. `["cv", "name"]`
- `Message string` — the reason, e.g. `"Input should be a valid string."`
- `Input string` — the offending value as text
- `YamlSource` — which of the four input files (Main/Design/Locale/Settings)

**Wrapping in `Render`**: returned as `fmt.Errorf("rendercv: invalid document: %s", detail)` where `detail` joins each record's location + message. The original `*rendercv.UserValidationError` is preserved in the chain via `%w` so a caller can `errors.As` it for programmatic access to the field list.

### `*rendercv.UserError` — user-facing failure, no location

**Source**: `pkg/rendercv/types.go:33`. Wrapped as `"rendercv: invalid document: <message>"` (same class as `UserValidationError`).

### `*rendercv.InternalError` — the renderer broke

**Source**: `pkg/rendercv/types.go:46`. Wrapped as `fmt.Errorf("rendercv: internal error: %w", err)` — a distinct prefix the caller uses to distinguish from an invalid document.

### Other errors

Any error from `GenerateTypst`, `GeneratePDF`, filesystem operations, or the temp-rename that is not one of the above is treated as an internal failure and wrapped with the `"rendercv: internal error:"` prefix. A `context.DeadlineExceeded`/`context.Canceled` from the abandon-and-rename pattern (R-001) is returned as `fmt.Errorf("rendercv: render cancelled: %w", ctx.Err())`.

### Classification summary

| Error source | Type | Prefix | Caller treats as |
|---|---|---|---|
| `rendercv.Build` → `*UserValidationError` | invalid document | `rendercv: invalid document:` | user-facing validation failure |
| `rendercv.Build` → `*UserError` | invalid document | `rendercv: invalid document:` | user-facing validation failure |
| `rendercv.Build` → `*InternalError` | renderer broke | `rendercv: internal error:` | internal failure (500) |
| `GenerateTypst`/`GeneratePDF` non-typed error | renderer broke | `rendercv: internal error:` | internal failure (500) |
| `context.DeadlineExceeded` / `context.Canceled` | cancelled/timed out | `rendercv: render cancelled:` | cancelled/timed-out failure |
| Filesystem error | renderer broke | `rendercv: internal error:` | internal failure (500) |

---

## 5. State transitions

There are no entity state transitions in this feature. The `GeneratedDocument` row's lifecycle (created → rendered → downloaded) is unchanged. The profile's `rendercvConfig`/`rendercvYaml` persistence is unchanged. The only "state" is the render operation itself, which has the same lifecycle as before:

```
RenderCvRenderer.Render(ctx, master, baseName)
  → ensureOutDir
  → PrepareMasterForMarshal
  → write YAML to <outDir>/<baseName>.yaml
  → rendercv.Build(yaml, opts)              [validates + resolves]
  → rendercv.GenerateTypst(model)           [writes .typ to temp path]
  → rendercv.GeneratePDF(model, typstPath)  [writes .pdf to temp path]
  → rename temp.pdf → <outDir>/<baseName>.pdf
  → (optional) blob upload YAML + PDF
  → return yamlPath, pdfPath
```

On any error or cancellation, the temp files are best-effort deleted and no `pdfPath` is returned.

---

## 6. Entities held fixed (for reference)

| Entity | Location | Change |
|---|---|---|
| `RendercvMaster` | `domain/rendercv.go:11` | none |
| `GeneratedDocument` (DB row, `pdf_path` column) | `internal/db/queries/*.sql` | none |
| `Profile.rendercvConfig` / `rendercvYaml` (DB columns) | schema migration | none |
| `storage.Blobstore` interface | `internal/platform/storage/domain/port.go` | none |
| `renderDeps` / `defaultRenderDeps` / `exportRenderDeps` | `application/service.go`, `workspace_export.go` | none (they call `Render` + `CountPages`, both unchanged in signature) |
| `ParseRendercv` / `PrepareMasterForMarshal` | `domain/` | none |

The feature is, by design, a zero-migration, zero-DTO-change engine swap.