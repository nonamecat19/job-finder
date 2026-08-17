# Tasks: Real Resume Preview on the Generate Page

**Input**: Design documents from `/specs/046-real-resume-preview/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/preview-document.md, quickstart.md

**Tests**: Included — quickstart.md already prescribes concrete test commands/scenarios per story; those are turned into task-level test items below.

**Organization**: Tasks are grouped by user story (spec.md: US1 preview matches final PDF, US2 preview updates live, US3 graceful failure/fallback) to enable independent implementation and testing of each.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)

## Path Conventions

Web app split per plan.md: `apps/api/` (Go backend, one new endpoint), `apps/dashboard/` (React frontend, primary surface), plus one build-target addition in the sibling `../rendercv-go/` repo (outside this repo, referenced by absolute sibling path per plan.md's Project Structure).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Scaffolding for the two WASM artifacts and the dashboard module that will orchestrate them. Nothing here is functional yet.

- [X] T001 Create `apps/dashboard/src/features/generate/wasm/` with placeholder files `rendercvWasm.ts`, `typstWasi.ts`, `previewPipeline.ts`, `assetCache.ts`
- [X] T002 [P] Add a browser WASI shim dependency (e.g. `@bjorn3/browser_wasi_shim`) to `apps/dashboard/package.json` and run `pnpm install`
- [X] T003 [P] Create `apps/dashboard/public/wasm/` for the lazy-loaded static assets (`rendercv.wasm`, `wasm_exec.js`, `typstwasm.wasm`, `fonts/`, `fontawesome-package/`), with a short README noting these are build outputs, not committed binaries
- [X] T004 [P] Add a `GOOS=js GOARCH=wasm` build entrypoint for `pkg/rendercv` in `../rendercv-go/cmd/wasm/main.go`, exposing `ReadYAML`/`Build`/`GenerateTypst` via `syscall/js` (per plan.md Primary Dependencies; deliberately excludes `GeneratePDF` — research.md Decision 1)
- [X] T005 [P] Add a build script (`apps/dashboard/scripts/build-wasm.sh` or a Makefile target) that builds `rendercv.wasm` from `../rendercv-go` and copies `wasm_exec.js` plus `typstwasm.wasm` + its fonts/`fontawesome-package` assets into `apps/dashboard/public/wasm/`

**Checkpoint**: Build tooling exists; no runtime behavior yet.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The backend endpoint and both WASM loaders every user story depends on. No user story can be implemented before this phase is done.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T006 Implement `PreviewDocument(ctx, runID) (dto.PreviewDocumentDto, error)` in `apps/api/internal/generation/application/workspace_preview.go`: reuse `domain.Assemble` + `domain.PrepareMasterForMarshal` + `yaml.Marshal` (the same pipeline `RenderCvRenderer.Render` uses, per contracts/preview-document.md) to build the YAML text, plus a content hash of the current section-selection state — no `ApplyFontSize`, no `render`, no `countPages`
- [X] T007 Define `dto.PreviewDocumentDto{Yaml string; SectionsHash string}` in `apps/api/internal/dto/generation_workspace.go` (tygo-annotated like its neighboring DTOs), then run the repo's tygo generation step so `packages/shared` gets the typed TS interface — **Constitution III**: this is the cross-language boundary type; T012's dashboard client MUST consume the generated type, not a hand-written one
- [X] T008 Register `GET /v1/generations/{runId}/preview-document` in `apps/api/internal/generation/interfaces/http/generations.go`, wired to `PreviewDocument`, returning `dto.PreviewDocumentDto` and reusing the same 404/precondition error handling `ExportGenerationRun` already applies
- [X] T009 [P] Implement `apps/dashboard/src/features/generate/wasm/rendercvWasm.ts`: loads `wasm_exec.js` + `rendercv.wasm` once as a singleton, exposes `async buildTypst(yaml: string): Promise<string>`
- [X] T010 [P] Implement `apps/dashboard/src/features/generate/wasm/typstWasi.ts`: instantiates `typstwasm.wasm` via the WASI shim (T002) with virtual-FS preopens for root/font-dir/pkg backed by the bundled fonts + `fontawesome-package` (T003/T005), exposes `async compilePdf(typstSource: string): Promise<Uint8Array>`
- [X] T011 Implement `apps/dashboard/src/features/generate/wasm/assetCache.ts`: a Cache-API-backed loader shared by T009/T010 so the ~29 MB `typstwasm` + fonts download once per browser and are fetched only when the Generate page's preview pane mounts (plan.md Constraints, research.md Open risk)
- [X] T012 Add `previewDocument(runId)` to `apps/dashboard/src/lib/api.ts`'s `generations` client, calling `GET /v1/generations/{runId}/preview-document` and typed against the `packages/shared` type T007 generated

**Checkpoint**: Foundation ready — endpoint returns real YAML, both WASM loaders can be invoked in isolation. User story implementation can now begin.

---

## Phase 3: User Story 1 - Preview matches the final PDF (Priority: P1) 🎯 MVP

**Goal**: The preview pane shows a real rendering of the resume document that matches the exported PDF — same content, section order, pagination, theme, locale.

**Independent Test**: Open a generation run, let the preview render once, export/download the same run, and compare the two PDFs for content/layout/pagination equivalence (quickstart.md Scenario 3, steps 1–2 and 4–5).

### Tests for User Story 1

- [X] T013 [P] [US1] `TestPreviewDocument_MatchesExportAssembly` in `apps/api/internal/generation/application/workspace_preview_test.go`: asserts `PreviewDocument`'s YAML matches `renderExport`'s pre-`ApplyFontSize` assembly for the same run/sections fixture (quickstart.md Scenario 1)
- [X] T014 [P] [US1] `TestWasmBuild_GenerateTypst_MatchesServerBuild` in `../rendercv-go`: compares the `GOOS=js/wasm` build's `GenerateTypst` output against the existing server build's output for the `testdata/golden` fixtures (quickstart.md Scenario 2)

### Implementation for User Story 1

- [X] T015 [US1] Implement the happy-path pipeline in `apps/dashboard/src/features/generate/wasm/previewPipeline.ts`: `api.generations.previewDocument(runId)` → `rendercvWasm.buildTypst(yaml)` → `typstWasi.compilePdf(typst)` → a `blob:` URL
- [X] T016 [US1] Rewrite `apps/dashboard/src/features/generate/components/ResumePreviewPane.tsx` to call `previewPipeline` on mount/run load and render the returned PDF `blob:` URL in an `<iframe>`/`<embed>`, replacing the styled-`div` approximation (research.md Decision 4)
- [X] T017 [US1] Ensure multi-page PDFs are fully viewable (scroll/page through) in `ResumePreviewPane.tsx`'s embedded viewer (FR-009)
- [X] T018 [US1] Run quickstart.md Scenario 3 (steps 1–2, 4–5) manually and record the comparison result

**Checkpoint**: User Story 1 is fully functional and independently testable — a one-shot real preview matching the exported PDF.

---

## Phase 4: User Story 2 - Preview updates as the user edits (Priority: P2)

**Goal**: Toggling sections or editing content updates the preview automatically, without exporting first, without flicker on rapid edits.

**Independent Test**: Toggle a section on/off in the workspace and confirm the preview re-renders to reflect the change within a few seconds, without a manual refresh step and without flicker (quickstart.md Scenario 3, step 3).

### Tests for User Story 2

- [X] T019 [P] [US2] Debounce/coalesce unit test in `apps/dashboard/src/features/generate/wasm/previewPipeline.test.ts`: rapid successive edits collapse into one render of the latest state (FR-010)

### Implementation for User Story 2

- [X] T020 [US2] Add a debounce (~300–500 ms idle window) with in-flight-render cancellation to `previewPipeline.ts`, keyed off section/content edit events (research.md Decision 5)
- [X] T021 [US2] Use the `sectionsHash` from T008's response in `previewPipeline.ts` to skip a redundant WASM re-render when consecutive edits resolve to the same effective document
- [X] T022 [US2] Wire the generation workspace's existing section-toggle/content-edit handlers (`apps/dashboard/src/features/generate/hooks.ts`) to trigger a debounced `previewPipeline` re-render, leaving existing export triggers untouched
- [X] T023 [US2] Run quickstart.md Scenario 3 step 3 manually (toggle off/on, confirm update within a few seconds, no flicker) and record the result

**Checkpoint**: User Stories 1 and 2 both work — the preview stays live and accurate while editing.

---

## Phase 5: User Story 3 - Preview stays usable when rendering fails or is unsupported (Priority: P3)

**Goal**: A rendering failure or unsupported browser shows a clear fallback state; the rest of the page, including export/download, keeps working.

**Independent Test**: Force a preview rendering failure and confirm a clear error state appears (not blank/stale content) while Export/Download still succeeds (quickstart.md Scenario 4).

### Tests for User Story 3

- [X] T024 [P] [US3] Error-injection unit test in `previewPipeline.test.ts`: a WASM load/compile failure surfaces a distinct error state, never blank or stale content passed off as current (FR-006)

### Implementation for User Story 3

- [X] T025 [US3] Add a loading state to `ResumePreviewPane.tsx`, shown while WASM assets are fetching or a render is in flight (FR-005)
- [X] T026 [US3] Add an error/fallback state to `ResumePreviewPane.tsx`, distinct from loading, shown on WASM init/compile failure or an unsupported-browser detection (FR-006, FR-007)
- [X] T027 [US3] Detect WebAssembly/WASI-shim-unsupported browsers up front in `previewPipeline.ts` and short-circuit straight to the fallback state instead of attempting a load (FR-007)
- [X] T028 [US3] Add/verify a test that Export/Download succeeds independent of `previewPipeline` state (`apps/dashboard/src/features/generate/hooks.test.ts` or equivalent) (FR-007)
- [X] T029 [US3] Run quickstart.md Scenario 4 manually (force failure, confirm error state, confirm export still works) and record the result

**Checkpoint**: All three user stories are independently functional — the feature is complete and safe to ship incrementally at any checkpoint.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Validation and hygiene that spans all three stories.

- [X] T030 [P] Run quickstart.md Scenario 5 (cold vs. warm Cache-API load timing) and record results against SC-003 and research.md's Open risk note — measured against the Vite dev server (`localhost:5173`) with `caches.keys()`/`caches.delete()` clearing the `resume-preview-wasm-v1` Cache-API store: cold load (empty cache) to first rendered preview ≈12s, dominated by the ~29 MB `typstwasm` + font download as expected; warm reload (cache populated) ≈7.5s — no re-download occurred (confirmed via Cache API), but this is well above "near-instant" because the dev server serves unminified/unbundled JS and re-runs Vite's module graph on every reload, which is dev-only overhead not present in a production build. A same-session section-toggle edit did trigger a new `blob:` PDF re-render (confirmed by URL change) without a full reload, but sub-3-second precision (SC-003) could not be reliably measured client-side in dev mode because synchronous WASM compute blocks the JS event loop the polling script itself runs on. Recommendation: re-verify SC-003's per-edit budget against a production (`vite build`) bundle before final sign-off.
- [X] T031 [P] Document the new WASM build step (`../rendercv-go` browser target + asset copy, T004/T005) in the dashboard's contributor docs
- [X] T032 Run `make test-lint` across `apps/api` and `apps/dashboard` for the full feature diff
- [X] T033 [P] Verify the bundle-size constraint: confirm `rendercv.wasm`/`typstwasm.wasm`/fonts are excluded from the dashboard's main JS bundle and only fetched lazily (plan.md Constraints)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately.
- **Foundational (Phase 2)**: Depends on Setup (T001–T005) completion — BLOCKS all user stories.
- **User Stories (Phase 3–5)**: All depend on Foundational (Phase 2) completion.
  - US1 has no dependency on US2/US3.
  - US2 builds on the pipeline US1 establishes (`previewPipeline.ts`, `ResumePreviewPane.tsx`) but is independently testable once present.
  - US3 wraps US1's pipeline with loading/error states; independently testable once US1 exists.
  - Recommended order given the dependency shape above: **US1 → US2 → US3** (matches priority order).
- **Polish (Phase 6)**: Depends on all three user stories being complete.

### Within Each User Story

- Tests (T013/T014, T019, T024) before or alongside their story's implementation tasks.
- Backend/library tasks before dashboard tasks that consume them.
- Story complete and checkpointed before moving to the next priority.

### Parallel Opportunities

- T002, T003, T004, T005 (Setup) can all run in parallel — different files/repos.
- T009, T010, T011 (Foundational) can run in parallel once T006/T007/T008 exist — different files, no dependency on the DTO/route work.
- T013 and T014 (US1 tests) can run in parallel — different repos.
- T019 (US2) and T024 (US3) can be written in parallel once US1's `previewPipeline.ts` exists, since they target different code paths in the same file.

---

## Parallel Example: Foundational Phase

```bash
# After T006/T007/T008 land, launch together:
Task: "Implement rendercvWasm.ts loader in apps/dashboard/src/features/generate/wasm/rendercvWasm.ts"
Task: "Implement typstWasi.ts loader in apps/dashboard/src/features/generate/wasm/typstWasi.ts"
Task: "Implement assetCache.ts in apps/dashboard/src/features/generate/wasm/assetCache.ts"
```

## Parallel Example: User Story 1 Tests

```bash
Task: "TestPreviewDocument_MatchesExportAssembly in apps/api/internal/generation/application/workspace_preview_test.go"
Task: "TestWasmBuild_GenerateTypst_MatchesServerBuild in ../rendercv-go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (critical — blocks everything else)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: quickstart.md Scenario 3, steps 1–2 and 4–5 — real preview matches exported PDF
5. Ship: users get a real, accurate (if not yet live-updating) preview

### Incremental Delivery

1. Setup + Foundational → real rendering pipeline exists end-to-end
2. + User Story 1 → accurate one-shot preview (MVP)
3. + User Story 2 → preview stays live while editing
4. + User Story 3 → graceful degradation, safe to ship to all browsers
5. + Polish → performance validated, bundle-size constraint verified

### Notes

- [P] tasks touch different files/repos and have no dependency on incomplete tasks.
- Commit after each task or logical group.
- Stop at any checkpoint to validate a story independently before continuing.
- The `../rendercv-go` build-target tasks (T004, T014) live in a sibling repository, not this one — coordinate the PR there separately if it has its own review process.
