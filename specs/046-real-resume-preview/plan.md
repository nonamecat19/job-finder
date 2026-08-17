# Implementation Plan: Real Resume Preview on the Generate Page

**Branch**: `046-real-resume-preview` | **Date**: 2026-08-14 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/046-real-resume-preview/spec.md`

## Summary

`ResumePreviewPane` (`apps/dashboard/src/features/generate/components/ResumePreviewPane.tsx`) is today a hand-built HTML re-styling of `run.sections` — it says so in its own comment ("not a real PDF render"). The only way a user sees the true document is exporting (`POST /v1/generations/{runId}/export`, `apps/api/internal/generation/application/workspace_export.go`), which runs the full server-side pipeline: `domain.Assemble` → `rendercv-go` (Typst → PDF via an embedded WASI compiler, per `specs/045-rendercv-go-migration`) → blob upload → static download URL.

This feature makes the preview real by running the same rendering engine **in the browser**, driven from the same assembly step the export path already uses, so what the user sees while toggling sections matches the exported PDF pixel-for-pixel — without a server round trip per edit.

Two pieces make this possible, both already present as sibling code, neither previously wired to a browser:

1. **`rendercv-go`'s `pkg/rendercv`** (`ReadYAML` → `Build` → `GenerateTypst`) is pure Go with no CGO — a standard `GOOS=js GOARCH=wasm` build (Go's own WASM target, `wasm_exec.js` glue) can run YAML validation and Typst-source generation client-side.
2. **`tools/typstwasm`** in the `rendercv-go` repo is the Typst compiler itself, already built as a `wasm32-wasip1` binary — today driven server-side through `wazero` (a pure-Go WASI *host*), not through a browser. In the browser it needs a WASI shim instead of `wazero` (research.md, Decision 2) to turn `.typ` source into PDF bytes.

The one new thing on the server is small and additive: a preview-document endpoint that runs `domain.Assemble` (the same call `renderExport` already makes) and returns the resulting RenderCV YAML **without** rendering it — no new merge/assembly logic, no duplicate type (Constitution III), just stopping one step earlier than export does. The client debounces edits, calls this endpoint, and feeds the YAML through the two WASM modules above to produce a real PDF, shown in an embedded PDF viewer.

## Technical Context

**Language/Version**: Go 1.25+ compiled to `GOOS=js GOARCH=wasm` (rendercv-go's `pkg/rendercv`, mirrored per `specs/045-rendercv-go-migration`'s existing Go 1.26.5 `apps/api` toolchain); Rust `wasm32-wasip1` (existing `tools/typstwasm` in the `rendercv-go` repo, unchanged); TypeScript/React (dashboard, existing stack).

**Primary Dependencies**:
- `github.com/nonamecat19/rendercv-go` `pkg/rendercv` (`ReadYAML`, `Build`, `GenerateTypst`) — compiled to a browser WASM artifact separate from the server build; only the pure-Go schema/templating path is used, not `GeneratePDF` (which nests `wazero`+`typstwasm`, a server-side-only path — see research.md Decision 1).
- `tools/typstwasm` (Rust, existing) — the Typst compiler binary, loaded in-browser via a WASI shim instead of `wazero` (research.md Decision 2). Ships with its embedded fonts and the `@preview/fontawesome:0.6.0` package it needs (same two "not obvious" inputs the tool's own README calls out).
- A browser WASI polyfill (e.g. `@bjorn3/browser_wasi_shim` or equivalent) to satisfy `typstwasm`'s WASI preopens (`--root`, `--font-dir`, `--pkg`) from an in-memory virtual filesystem instead of a real one.
- Go's `wasm_exec.js` glue for the `pkg/rendercv` build, plus `memfs` to back `globalThis.fs` — `wasm_exec.js`'s own built-in `fs` stub returns `ENOSYS` for every operation, and `pkg/rendercv` does real file I/O for theme template partials and output (verified during implementation; see `internal/wasmpreview` in `../rendercv-go`).
- A PDF viewer for the resulting bytes — the browser's native PDF rendering via an `<iframe>`/`<embed>` on a `blob:` URL (no new dependency) is the default; `pdf.js` is a fallback if native embedding proves inconsistent across the target browsers (research.md Decision 4).
- Existing dashboard stack: React + Vite + TanStack Query.

**Storage**: N/A for anything persisted — this feature adds no database tables or columns. The two WASM binaries (~29 MB `typstwasm` + the `pkg/rendercv` build) and font/package assets are static files served by the dashboard and cached client-side via the browser's Cache API so they download once per browser, not once per preview.

**Testing**: `go test` for the new `GOOS=js GOARCH=wasm` build target of `pkg/rendercv` (build-tag-gated, run via Node or `wasmbrowsertest` in CI, matching how `rendercv-go` already gates its own conformance suite); `vitest` for the new preview-orchestration code in the dashboard (debounce, WASM lifecycle, error/fallback states) with the WASM calls mocked; a manual/e2e pass (existing dashboard e2e tooling) comparing a preview render against the exported PDF for the fixture documents `rendercv-go` already uses for parity (`testdata/golden`), reused rather than duplicated.

**Target Platform**: Web browser (the Generate page), via the existing Vite-bundled dashboard. Go API gets one new small read-only endpoint; no other backend change.

**Project Type**: Web application feature (dashboard-primary, one additive backend endpoint).

**Performance Goals**: Preview reflects an edit within 3 seconds for ≥95% of edits (spec SC-003), measured after the WASM modules are loaded and warm. First-load cost (downloading and instantiating ~30 MB of WASM + fonts) is paid once per session and hidden behind the pane's loading state (spec FR-005); it is not counted against the 3-second per-edit budget.

**Constraints**:
- No new per-edit backend round trip beyond the lightweight YAML-assembly endpoint (spec FR-002); no Typst/PDF work happens on the server for preview.
- The preview pane's failure MUST NOT affect the existing export/download flow (spec FR-007) — the two paths share the assembly step but nothing else, so a WASM crash in the browser cannot touch the server-side render.
- The ~29 MB `typstwasm` payload MUST be lazy-loaded (only fetched when the Generate page's preview pane is actually shown) and MUST NOT be added to the dashboard's main JS bundle.
- Browsers without WebAssembly or without the specific features the WASI shim needs get the FR-006/FR-007 fallback state, not a broken page.

**Scale/Scope**: One new backend endpoint (read-only, reuses `domain.Assemble`); one new dashboard module orchestrating the two WASM modules and the debounce/cache logic; `ResumePreviewPane.tsx` rewritten to render real PDF bytes instead of styled `div`s. No change to the export/download path, no change to `rendercv-go` itself beyond adding a browser build target for `pkg/rendercv`.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — N/A. This feature only changes what the user sees before they act; it adds no path that submits, sends, or acts on the user's behalf. Export/download still requires the same explicit user action it does today. **PASS.**
- **II. Grounded Generation** — N/A for new generation; the preview renders content the existing generation/toggle pipeline already produced and grounded. It introduces no new LLM call and no new content synthesis. **PASS.**
- **III. Typed Contracts Across Service Boundaries** — The one new API surface (preview-document endpoint) returns a string (RenderCV YAML) built by the exact same `domain.Assemble` call the export path already uses — no duplicate merge logic, no hand-maintained parallel type. The dashboard-side WASM orchestration consumes that YAML as an opaque string, the same shape the server already writes to disk today; no new shared TS type is needed since the YAML text is not machine-parsed on the client. **PASS**, with the constraint recorded above (reuse `domain.Assemble`, do not reimplement it in Go-WASM or TS).
- **IV. Test Discipline Per Language, Enforced at the Boundary** — The new Go WASM build gets its own `go test` coverage (build-tag-gated); the new dashboard orchestration gets `vitest` coverage; the endpoint gets the same `go test` coverage as any other HTTP handler. Nothing here crosses into `make test-integration` territory (no new Postgres/Redis interaction) beyond the endpoint's existing transaction pattern. **PASS.**
- **V. Self-Hosted Control Plane, Single Inference Path** — N/A. No AI/inference call of any kind is added; this is a rendering-only change. **PASS.**

No violations; Complexity Tracking is not needed.

## Project Structure

### Documentation (this feature)

```text
specs/046-real-resume-preview/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md         # Phase 1 output
├── contracts/
│   └── preview-document.md   # Phase 1 output
├── quickstart.md         # Phase 1 output
└── checklists/
    └── requirements.md   # /speckit.specify output
```

### Source Code (repository root)

```text
apps/api/
├── internal/generation/
│   ├── application/
│   │   └── workspace_preview.go       # NEW — preview-document use case, wraps domain.Assemble
│   ├── domain/
│   │   └── rendercv.go                 # UNCHANGED — Assemble/MarshalYAML already exist, reused
│   └── interfaces/http/
│       └── generations.go              # + one new route: GET/POST .../preview-document

apps/dashboard/
├── src/features/generate/
│   ├── components/
│   │   └── ResumePreviewPane.tsx       # REWRITTEN — renders real PDF bytes, not styled divs
│   └── wasm/                            # NEW
│       ├── rendercvWasm.ts              # loads/calls the pkg/rendercv GOOS=js/wasm build
│       ├── typstWasi.ts                 # loads/calls typstwasm via a browser WASI shim
│       └── previewPipeline.ts           # debounce → fetch YAML → rendercv-wasm → typst-wasi → blob URL
├── public/wasm/                          # NEW, lazy-loaded static assets
│   ├── rendercv.wasm  wasm_exec.js
│   └── typstwasm.wasm  fonts/  fontawesome-package/
└── package.json                          # + WASI shim dependency

(sibling repo, unchanged in this feature except a new build target)
../rendercv-go/
└── (adds a `GOOS=js GOARCH=wasm` build recipe for pkg/rendercv only — no change to pkg/rendercv's API)
```

**Structure Decision**: This is a dashboard feature with one small, additive API endpoint — the existing `apps/api` + `apps/dashboard` split (Constitution's "Technology & Architecture Constraints") is used as-is. No new app or package is created; the two WASM artifacts are dashboard-owned static assets, not a new backend service.

## Complexity Tracking

*No Constitution violations — this section is intentionally empty.*
