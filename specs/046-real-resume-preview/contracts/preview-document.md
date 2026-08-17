# Contract: Preview Document Endpoint

**Feature**: 046-real-resume-preview
**Location (new)**: `apps/api/internal/generation/application/workspace_preview.go`, routed in `apps/api/internal/generation/interfaces/http/generations.go`

This is the one new backend surface this feature adds. It exists solely to give the browser-side renderer the same RenderCV YAML the export path already produces, one step earlier in the same pipeline — no PDF is compiled server-side for this call.

## Endpoint

```
GET /v1/generations/{runId}/preview-document
```

Read-only; no request body. Returns the current server-known selection state for the run — the same `sections` a subsequent `POST /v1/generations/{runId}/export` would assemble from.

### Response — 200 OK

```json
{
  "yaml": "cv:\n  name: Jane Doe\n  ...",
  "sectionsHash": "sha256:…"
}
```

| Field | Type | Contract |
|---|---|---|
| `yaml` | string | The RenderCV document, assembled via `domain.Assemble(master, sections)` and serialized via `domain.PrepareMasterForMarshal` + `yaml.Marshal` — byte-for-byte the same pipeline `RenderCvRenderer.Render` uses before it writes the export YAML to disk, minus the write. |
| `sectionsHash` | string | A content hash of the section-selection state used to assemble `yaml`. Lets the client skip a redundant WASM re-render if two rapid edits happened to resolve to the same effective document (e.g. toggle off then on before the debounce window closes). |

### Response — 404 Not Found

Run ID does not exist (`apperr.NotFound`, same convention as `ExportGenerationRun`).

### Response — 409/422 (blocked precondition)

Mirrors the existing precondition checks `ExportGenerationRun` already applies before assembling (e.g. profile has no RenderCV config) — same error shape, so the dashboard can reuse its existing export-error handling for the preview path.

## What this endpoint does NOT do

- Does not call `ApplyFontSize`, `render`, or `countPages` — no Typst/PDF compilation happens on the server for this call.
- Does not mutate the generation run, the section selection, or any persisted state — purely a read projection of current state, safe to call on every debounced edit.
- Does not accept an edited selection in the request body. The workspace's section toggles are already persisted server-side as the user interacts with the workspace (existing behavior, unchanged by this feature); this endpoint reads that current state rather than accepting an ad hoc override. If a future iteration needs unsaved, not-yet-persisted edits reflected in the preview, that is a new requirement, not covered here (see spec Assumptions — this feature reuses existing selection persistence).

## Caller

`apps/dashboard/src/features/generate/wasm/previewPipeline.ts` — calls this endpoint after its debounce window closes, feeds `yaml` to the browser-side `pkg/rendercv` (`GenerateTypst`) → `typstwasm` pipeline (research.md Decisions 1–2), and renders the resulting PDF bytes in `ResumePreviewPane.tsx`.
