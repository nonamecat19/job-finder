# Implementation Plan: Replace the Python RenderCV dependency with rendercv-go

**Branch**: `045-rendercv-go-migration` | **Date**: 2026-08-14 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/045-rendercv-go-migration/spec.md`

## Summary

The API renders resume PDFs today by shelling out to the upstream Python `rendercv` CLI (`rendercv[full]==2.8`), which is the sole reason the deployed image carries a Python runtime, a virtual environment, and the Typst toolchain. `rendercv-go` (`github.com/nonamecat19/rendercv-go`, frozen at v1.0.0) is a parity-tested Go reimplementation of the same tool that runs natively in-process: an embedded WASI Typst compiler (wazero, pure Go, no CGO) and a fixed embedded font set remove every external dependency.

This feature replaces the `RenderCvRenderer` infrastructure adapter so it calls `rendercv.Build` + `rendercv.GenerateTypst` + `rendercv.GeneratePDF` directly instead of `exec.CommandContext`, removes the Python base image and venv install from `apps/api/Dockerfile`, retires the `RENDERCV_BIN` setting (tolerated, not enforced, for one release), and updates the docs and domain spec to match. Inputs, outputs, the page-count path, the smoke render, blob upload, and the page-fit loop are all held fixed — only the rendering engine's implementation changes.

## Technical Context

**Language/Version**: Go 1.26.5 (`apps/api/go.mod`; the `rendercv-go` module requires Go 1.25.0, compatible).

**Primary Dependencies**:
- `github.com/nonamecat19/rendercv-go` v1.0.0 — the new rendering library (added). Public API in `pkg/rendercv`: `Build`, `GenerateTypst`, `GeneratePDF`, `GenerateMarkdown`, `GenerateHTML`, `GeneratePNG`, `ReadYAML`. Embeds the Typst compiler (`typst.wasm`, ~29 MB) and 15 font families via `//go:embed`. Pure Go, no CGO, no external binary.
- `github.com/ledongthuc/pdf` — retained for `CountPages` (reads the rendered PDF's page count directly; unchanged).
- `gopkg.in/yaml.v3` — retained for writing the YAML document to disk (unchanged).
- Removed at the Docker level: `rendercv[full]==2.8`, the `python:3.14-slim-bookworm` base image, the `/opt/rendercv-venv` virtual environment, and the transitively-shipped Typst binary.

**Storage**: PostgreSQL (document metadata, `pdf_path` column), MinIO blob store (YAML + PDF upload, unchanged), and the local filesystem at `DOCUMENTS_DIR` (default `/data/documents`, unchanged). The rendered PDF and the YAML document continue to be written to disk and uploaded to blob storage where configured.

**Testing**: `go test` for the API; `vitest` for the dashboard (unchanged — the dashboard does not touch rendering). The live test (`//go:build live`) in `rendercv_live_test.go` switches from requiring the Python `rendercv` binary on PATH to requiring nothing extra — `rendercv-go` is a normal Go dependency, so the live test runs on a clean checkout. The eval harness already runs with no render toolchain (stubs cover `render` + `countPages`); that property is preserved. A new comparison test renders the existing fixture corpus through the old engine and the new one and compares text + page count (spec Story 1, FR-013).

**Target Platform**: Linux server (Docker Compose, `docker-compose.prod.yml`). The API image changes from `python:3.14-slim-bookworm` to a minimal Go-runtime base (e.g. `debian:bookworm-slim` or `gcr.io/distroless/static` — see research.md). Chromium stays (for the chromedp cover-letter HTML→PDF path), poppler stays (for `pdftotext` resume import).

**Project Type**: Web service (Go HTTP API + asynq workers).

**Performance Goals**: A single resume render completes within the existing 120-second budget. The embedded WASI Typst compiler compiles its module once per process (`sync.OnceValues`) and each render instantiates a fresh module, so steady-state renders avoid the cold-start cost. No measured increase in rendering failures or timeouts (SC-003).

**Constraints**:
- No external rendering process may be invoked (FR-001, FR-009).
- A render must be abortable when the initiating request is cancelled and must be bounded by a time budget (FR-006). **The `rendercv-go` public API does not accept a `context.Context`** — this is the single most important integration constraint and is resolved in research.md (goroutine + `context.AfterFunc` / abandon-and-cleanup pattern).
- A rendering failure caused by an invalid document must be distinguishable from an internal renderer failure, with the offending field and reason included (FR-005). `rendercv-go` returns typed errors: `*rendercv.UserValidationError` (carries `[]ValidationError` with `SchemaLocation []string` + `Message`) for invalid documents, `*rendercv.InternalError` for renderer bugs, `*rendercv.UserError` for user-facing failures without a location.
- Tests must run without an external renderer installed (FR-011). Already true for the eval harness; the live test becomes toolchain-free too.

**Scale/Scope**: ~1 adapter file rewritten (`rendercv_renderer.go`, 86 lines), 1 Dockerfile rewritten (runtime stage), ~4 config/doc files updated, 1 new comparison test. The `domain.RendercvMaster` type, `PrepareMasterForMarshal`, the page-fit loop, the smoke render, and blob upload are untouched — the change is localized to the infrastructure adapter and the deployment surface.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Checked against `/home/nnc/Projects/job-finder/.specify/memory/constitution.md` v2.0.0.

### I. No Auto-Apply, Ever — PASS
Rendering produces a PDF for a human to review and manually submit. The engine swap changes nothing about who submits an application. No code path in the renderer submits, messages, or acts on a listing.

### II. Grounded Generation — PASS
The renderer is downstream of grounding; it turns an already-verified `RendercvMaster` into a PDF. The verifiers (`VerifyHighlightGrounding`, `VerifyStructureIntegrity`, `VerifyRendercvGrounding`) run before `Render` is called and are untouched. The engine swap cannot introduce fabricated content — it renders exactly the document it is given.

### III. Typed Contracts Across Service Boundaries — PASS
This is an in-process Go-to-Go change. No new cross-language boundary is introduced. The `RendercvMaster` type and the `Render` method signature are held fixed, so the facade re-exports in `generation.go` and all callers (generation service, workspace export, profile smoke render) compile unchanged. The `rendercv-go` public API types (`Model`, `BuildOptions`, error aliases) stay inside the infrastructure adapter and are not leaked through the facade.

### IV. Test Discipline Per Language, Enforced at the Boundary — PASS
The change touches only `apps/api` (Go). `make test-lint` (lint-go + test-go) is the gate. The dashboard is not touched, so `test-react` is unaffected but still runs as part of `test-lint`. Integration/e2e tests that exercise real Postgres/Redis are not affected by the renderer change (they stub the render path). The new comparison test is a Go unit test, not an integration test.

### V. Self-Hosted Control Plane, Single Inference Path — PASS
Rendering is not an inference task. The swap removes a third-party runtime (CPython) from the deployment and replaces it with a vendored Go library whose compiler and fonts are embedded — strictly more self-hosted, not less. No provider credential or external service is involved.

### Technology & Architecture Constraints — PASS
- Go + sqlc + goose: unaffected (no DB schema change).
- React + Vite + Tailwind: unaffected.
- `packages/shared`: unaffected (no DTO change — `RendercvMaster` is internal to the API).
- asynq on Redis: unaffected.
- Docker Compose: the API service image changes; the compose files need only the `RENDERCV_BIN` env line removed from `docker-compose.prod.yml` (and tolerated if still present per FR-010).

### Development Workflow & Quality Gates — PASS
- `pnpm --filter @job-finder/shared build` first: unaffected (shared not touched).
- `make` targets as canonical entry points: `make run-backend` no longer needs `rendercv` on PATH; `make images` builds the new image; `make test-lint` is the gate.
- Plan doc at `specs/045-rendercv-go-migration/plan.md`: this is that doc.
- `make test-lint` before merge: required (the change touches `apps/api`).

### Governance — PASS
No deviation from the five Core Principles. No Complexity Tracking entries needed.

**Gate result: PASS — no violations. Proceed to Phase 0.**

## Project Structure

### Documentation (this feature)

```text
specs/045-rendercv-go-migration/
├── plan.md              # This file
├── research.md          # Phase 0 output — resolves cancellation, base-image, parity-comparison, page-count, relative-path, font-drift unknowns
├── data-model.md        # Phase 1 output — the RendercvMaster → rendercv-go Model mapping and error types
├── quickstart.md        # Phase 1 output — how to validate the feature end-to-end
├── contracts/
│   └── renderer.md      # Phase 1 output — the RenderCvRenderer interface contract (held fixed)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
apps/api/
├── cmd/server/
│   └── compose.go                      # composeGeneration: wire NewRenderCvRenderer (drop bin arg)
├── internal/
│   ├── config/
│   │   ├── config.go                   # RendercvBin field kept (tolerated, FR-010), no longer read by renderer
│   │   └── defaults.go                 # RENDERCV_BIN default kept for one release (ignored, not error)
│   ├── generation/
│   │   ├── domain/
│   │   │   ├── rendercv.go             # RendercvMaster — UNCHANGED
│   │   │   └── prepare_marshal.go      # PrepareMasterForMarshal — UNCHANGED
│   │   ├── application/
│   │   │   ├── service.go              # renderDeps / defaultRenderDeps — UNCHANGED (calls Render + CountPages)
│   │   │   └── workspace_export.go     # exportRenderDeps — UNCHANGED
│   │   └── infrastructure/
│   │       ├── rendercv_renderer.go    # REWRITTEN: rendercv-go library calls instead of exec.CommandContext
│   │       ├── rendercv_live_test.go   # UPDATED: no longer needs rendercv on PATH
│   │       ├── rendercv_compare_test.go # NEW: old-vs-new corpus comparison (FR-013)
│   │       └── outdir.go               # UNCHANGED
│   └── profile/
│       └── application/service.go      # SaveConfig smoke render — signature unchanged, bin arg becomes no-op
├── Dockerfile                          # REWRITTEN runtime stage: drop python base + venv, keep chromium + poppler
├── go.mod                              # ADD: github.com/nonamecat19/rendercv-go v1.0.0
└── go.sum                              # updated by `go mod tidy`

docker-compose.prod.yml                 # RENDERCV_BIN line removed (or left; tolerated per FR-010)
.env.example                            # RENDERCV_BIN line annotated as deprecated/ignored
docs/docs/operations/local-development.md   # rendercv prerequisite removed
docs/docs/operations/configuration-reference.md  # RENDERCV_BIN marked obsolete
docs/docs/ai/generation.md              # "shells out to RENDERCV_BIN" → "renders in-process via rendercv-go"
specs/domains/resume-generation.md      # §7.1 updated: in-process renderer, no Typst binary
```

**Structure Decision**: The change is localized to the `generation/infrastructure` adapter (one file rewritten, one new test), its wiring in `cmd/server/compose.go`, the `Dockerfile` runtime stage, and documentation. No new packages, no new modules, no DB migration. The DDD layering (domain/application/infrastructure/interfaces) and the facade in `generation.go` are preserved exactly — the `RenderCvRenderer` type and its `Render` method signature stay the same so no caller changes.

## Complexity Tracking

> No Constitution Check violations. Table intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |