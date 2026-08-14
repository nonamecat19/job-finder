# Tasks: Replace the Python RenderCV dependency with rendercv-go

**Input**: Design documents from `/specs/045-rendercv-go-migration/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/renderer.md, quickstart.md

**Tests**: Included — FR-011 mandates automated tests covering a successful render, a validation failure, and the page-count path, runnable without an external renderer.

**Organization**: Tasks are grouped by user story. The core engine swap is in the Foundational phase (it is the blocking prerequisite for all stories — the constructor signature change forces the `Render` method to change, so the full `rendercv_renderer.go` rewrite happens there). User story phases contain story-specific validation tests and deployment work.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

## Path Conventions

- Go backend: `apps/api/` (module `github.com/job-finder/api`)
- Renderer adapter: `apps/api/internal/generation/infrastructure/`
- Generation facade: `apps/api/internal/generation/generation.go`
- Composition root: `apps/api/cmd/server/compose.go`
- Profile service: `apps/api/internal/profile/application/service.go`
- Config: `apps/api/internal/config/`
- Dockerfile: `apps/api/Dockerfile`
- Docs: `docs/docs/`
- Domain specs: `specs/domains/`

---

## Phase 1: Setup

**Purpose**: Add the `rendercv-go` library as a dependency so the Foundational phase can import it.

- [X] T001 Add `github.com/nonamecat19/rendercv-go@v1.0.0` to `apps/api/go.mod` and run `go mod tidy` to update `apps/api/go.sum` (run `go get github.com/nonamecat19/rendercv-go@v1.0.0` from the `apps/api/` module directory). Verify the module resolves to the tagged v1.0.0 release with no `replace` directive.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The core engine swap — rewrite `RenderCvRenderer` to call `rendercv-go` in-process instead of shelling out to the Python `rendercv` CLI. This is the blocking prerequisite for ALL user stories because the constructor signature change (dropping `bin`) forces the `Render` method to change (it references `r.bin`), and all callers must be updated for the code to compile.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T002 Rewrite `RenderCvRenderer` in `apps/api/internal/generation/infrastructure/rendercv_renderer.go`:
  - Remove the `bin` field from the struct; change `NewRenderCvRenderer(outDir string) *RenderCvRenderer` (drop the `bin` parameter).
  - Rewrite `Render` to call `rendercv.Build` → `rendercv.GenerateTypst` → `rendercv.GeneratePDF` instead of `exec.CommandContext`. Pass `BuildOptions{InputFilePath: yamlPath, PDFPath: tmpPdfPath, TypstPath: tmpTypstPath, DontGenerateMarkdown: true, DontGenerateHTML: true, DontGeneratePNG: true}` (research.md R-001, R-005).
  - Impose the 120 s budget + cancellation via goroutine + `context.AfterFunc` on the caller's `ctx`: `select` between the render result channel and `<-cmdCtx.Done()`; on `Done()`, best-effort delete temp files and return `fmt.Errorf("rendercv: render cancelled: %w", ctx.Err())` (R-001).
  - Write the PDF to a temp path (`<outDir>/<baseName>.pdf.tmp.<unique>`) then atomically `os.Rename` to the final `pdfPath` on success — no half-written output on any error/cancel path (R-001, R-008).
  - Classify errors via `errors.As`: `*rendercv.UserValidationError` / `*rendercv.UserError` → `fmt.Errorf("rendercv: invalid document: %s", detail)` (join each `ValidationError.SchemaLocation` + `Message`); `*rendercv.InternalError` and all other errors → `fmt.Errorf("rendercv: internal error: %w", err)`; preserve the original typed error via `%w` (R-007, data-model.md §4).
  - Keep the YAML write (`PrepareMasterForMarshal` → `yaml.Marshal` → `os.WriteFile`) and blob upload (`r.Store.Upload`) logic unchanged (FR-003).
  - Keep `CountPages` unchanged (R-004).
  - Remove the `os/exec`, `bytes`, and `time` imports only if no longer used; add `errors` and `github.com/nonamecat19/rendercv-go/pkg/rendercv` imports.
- [X] T003 Update the `NewRenderCvRenderer` re-export in `apps/api/internal/generation/generation.go` (line 53) to match the new single-arg constructor signature from T002.
- [X] T004 Update the renderer wiring in `apps/api/cmd/server/compose.go` (line 404): change `generation.NewRenderCvRenderer(cfg.DocumentsDir, cfg.RendercvBin)` to `generation.NewRenderCvRenderer(cfg.DocumentsDir)`. Leave `cfg.RendercvBin` in the config struct (tolerated, FR-010, R-009).
- [X] T005 Update the profile service in `apps/api/internal/profile/application/service.go`: remove the `rendercvBin` field (line 27) and the parameter from `NewService` (lines 30-31); change the smoke render constructor call in `SaveConfig` (line 242) from `generation.NewRenderCvRenderer(tempDir, s.rendercvBin)` to `generation.NewRenderCvRenderer(tempDir)`. Update ALL call sites of `profile.NewService` to drop the `rendercvBin` argument: `apps/api/cmd/server/compose.go`, `apps/api/internal/matching/application/integration_test.go:69`, `apps/api/internal/matching/application/embedding_provenance_test.go:149`, and any other callers found via `grep -rn "profile.NewService" apps/api/`.

**Checkpoint**: Foundation ready — the API compiles with `rendercv-go` as a library, no `exec.CommandContext` for rendering, no `rendercvBin` parameter threading. `go build ./...` from `apps/api/` succeeds. User story validation can now begin.

---

## Phase 3: User Story 1 — Generating a tailored resume still produces a correct PDF (Priority: P1) 🎯 MVP

**Goal**: Prove that every resume document in the existing corpus renders successfully with the new engine, producing the same text and page count as before (FR-001, FR-002, FR-004, FR-007, FR-013, SC-001, SC-002).

**Independent Test**: Render the fixture corpus through the new `RenderCvRenderer`, extract text via `pdftotext` and page count via `CountPages`, and compare against golden output captured from the old Python engine (quickstart.md Scenario 2).

### Tests for User Story 1

- [X] T006 [P] [US1] Capture golden output from the old Python `rendercv` engine for the comparison corpus into `apps/api/internal/generation/infrastructure/testdata/compare/golden/`: render `testdata/sample_rendercv.yaml` and each `evaldata/cases/*/master.yaml` through the current (pre-rewrite) `RenderCvRenderer` with the Python `rendercv` binary on PATH, extract text via `pdftotext -layout` and page count via `CountPages`, and write `<case>.txt` + `<case>.pages` golden files. This is a one-time data-gathering step that must run before the Foundational rewrite (T002) changes the renderer, or using the old CLI installed separately. Commit the goldens.
- [X] T007 [US1] Write the comparison test in `apps/api/internal/generation/infrastructure/rendercv_compare_test.go`: for each corpus document, run `PrepareMasterForMarshal` → `Render` (new engine) → `pdftotext` → `CountPages`, and assert the extracted text equals the golden `.txt` and the page count equals the golden `.pages` (FR-013, SC-001, SC-002). On mismatch, fail with a diff. This test must not require the old engine at run time — only the new engine + `pdftotext` (FR-011). (depends on T002, T006)
- [X] T008 [P] [US1] Update the live render test in `apps/api/internal/generation/infrastructure/rendercv_live_test.go`: change `NewRenderCvRenderer(outDir, "")` to `NewRenderCvRenderer(outDir)` (line 28). The test now runs on a clean checkout with no `rendercv` binary on PATH (FR-011, quickstart.md Scenario 1).
- [X] T009 [P] [US1] Write the page-count test in `apps/api/internal/generation/infrastructure/rendercv_renderer_test.go`: render a known document (e.g. `testdata/sample_rendercv.yaml`) via `Render`, call `CountPages(pdfPath)`, and assert the page count is positive and matches the golden value (FR-007, FR-011, quickstart.md Scenario 4).

**Checkpoint**: User Story 1 is fully functional — resumes render correctly with the new engine, the comparison test passes, and the live test runs with no external renderer.

---

## Phase 4: User Story 2 — Invalid documents fail with a usable explanation (Priority: P1)

**Goal**: Prove that a malformed document surfaces a field-level validation error distinguishable from an internal renderer failure, and that cancellation produces no partial output (FR-005, FR-006, SC-006).

**Independent Test**: Submit documents with known defects and assert each surfaces a message naming the offending field; assert an internal-error path is distinguishable; assert cancellation leaves no half-written PDF (quickstart.md Scenarios 3, 6).

### Tests for User Story 2

- [X] T010 [P] [US2] Write the validation-failure test in `apps/api/internal/generation/infrastructure/rendercv_renderer_test.go`: construct a `RendercvMaster` with a known defect (e.g. `cv.name` set to an integer), call `Render`, and assert the error message contains `"rendercv: invalid document:"` and names the offending field path (`[cv name]`). Also assert the original `*rendercv.UserValidationError` is reachable via `errors.As` and carries `SchemaLocation` + `Message` (FR-005, quickstart.md Scenario 3).
- [X] T011 [P] [US2] Write the internal-error test in `apps/api/internal/generation/infrastructure/rendercv_renderer_test.go`: trigger an internal-error path (e.g. an unwritable `outDir` or a corrupted `RendercvMaster` that passes `PrepareMasterForMarshal` but fails `rendercv.Build` with a non-validation error), and assert the error message contains `"rendercv: internal error:"` — distinguishable from the invalid-document prefix (FR-005 acceptance scenario 2).
- [X] T012 [P] [US2] Write the cancellation test in `apps/api/internal/generation/infrastructure/rendercv_renderer_test.go`: create a `context.WithCancel`, cancel it immediately, call `Render`, and assert the error wraps `context.Canceled` and that no file exists at the expected `pdfPath` (FR-006, Edge Cases, quickstart.md Scenario 6).

**Checkpoint**: User Story 2 is fully functional — invalid documents, internal failures, and cancellation are all correctly classified and produce no partial output.

---

## Phase 5: User Story 3 — The profile smoke render keeps guarding bad profiles (Priority: P2)

**Goal**: Prove the profile smoke render runs with the new engine, succeeds for valid profiles, fails with validation detail for invalid ones, and leaves no stray artifacts (FR-008).

**Independent Test**: Save a profile that yields an unrenderable document and confirm the save path reports the problem; save a valid one and confirm it succeeds and leaves no stray temp artifacts (quickstart.md Scenario 5).

### Tests for User Story 3

- [X] T013 [US3] Write the smoke render test in `apps/api/internal/profile/application/service_test.go` (or a new `apps/api/internal/profile/application/smoke_render_test.go`): call `SaveConfig` with a valid RenderCV YAML and assert success; call with an unrenderable YAML (e.g. missing `cv` block) and assert the error contains the validation detail. After each case, assert no `rendercv-smoke-*` temp directories remain in `os.TempDir()` (FR-008, quickstart.md Scenario 5). The test must not require the old `rendercv` binary (FR-011).

**Checkpoint**: User Story 3 is fully functional — the smoke render guards profiles with the new engine and cleans up.

---

## Phase 6: User Story 4 — Deployments no longer carry a Python runtime (Priority: P2)

**Goal**: The deployed image contains no Python interpreter, no virtual environment, and no separately installed Typst toolchain; rendering works identically in that image; the obsolete `RENDERCV_BIN` setting is tolerated (FR-009, FR-010, SC-004).

**Independent Test**: Build the image, confirm the Python-based renderer install is absent, run a render inside the container, and confirm a valid PDF comes out (quickstart.md Scenarios 7, 8, 10).

### Implementation for User Story 4

- [X] T014 [P] [US4] Rewrite the runtime stage of `apps/api/Dockerfile`: change `FROM python:3.14-slim-bookworm` to `FROM debian:bookworm-slim` (R-002); remove the `python3 -m venv /opt/rendercv-venv` + `pip install "rendercv[full]==2.8"` + symlink block (lines 26-33); keep the `apt-get install chromium poppler-utils ca-certificates fonts-liberation curl` block; keep `COPY --from=build /out/server /usr/local/bin/server`; remove `ENV RENDERCV_BIN=/usr/local/bin/rendercv` (line 38); keep `ENV DOCUMENTS_DIR=/data/documents`; keep `EXPOSE 3000` and `CMD`.
- [X] T015 [P] [US4] Remove the `RENDERCV_BIN` env line from the `api` service's explicit env allowlist in `docker-compose.prod.yml` (line 129). Leave `DOCUMENTS_DIR`, `RESUME_GROUNDING_LEVEL`, `RESUME_MASTER_PATH`, and `MINIO_*` unchanged.
- [X] T016 [P] [US4] Annotate `RENDERCV_BIN` as deprecated/ignored in `.env.example` (line 117): change the line to `# RENDERCV_BIN — obsolete, ignored (rendercv-go runs in-process); kept for one release for compatibility`.
- [X] T017 [P] [US4] Remove `rendercv` from the prerequisites table in `docs/docs/operations/local-development.md` (line 19): a developer no longer needs it on PATH.
- [X] T018 [P] [US4] Mark `RENDERCV_BIN` as obsolete/ignored in `docs/docs/operations/configuration-reference.md` (line 162): update the description to note the setting is loaded but not read; rendering is now in-process via `rendercv-go`.
- [X] T019 [P] [US4] Update the renderer description in `docs/docs/ai/generation.md` (lines 130-139): change "shells out to the `RENDERCV_BIN` binary" to "renders in-process via the `rendercv-go` library (embedded WASI Typst compiler + fixed font set, no external runtime)".
- [X] T020 [P] [US4] Update `docs/docs/operations/testing.md` (lines 110-111): the live render test no longer requires the real `rendercv` binary on PATH — it runs on a clean checkout.

**Checkpoint**: User Story 4 is fully functional — the image has no Python/venv/Typst, rendering works in-container, and the obsolete setting is tolerated.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Domain spec update, merge gate, image verification, and parity findings.

- [X] T021 [P] Update `specs/domains/resume-generation.md` §7.1 (lines 736-775): change the description from "shells out to the Typst-based `rendercv` binary (`RENDERCV_BIN`)" to "renders in-process via the `rendercv-go` library; the embedded WASI Typst compiler and a fixed font set are compiled into the Go binary; no external rendering process or runtime is invoked". Update the `CountPages` description to note it reads a `rendercv-go`-produced PDF (FR-012).
- [X] T022 Run `make test-lint` from the repo root and confirm it passes (lint-go + lint-web + test-go + test-react). This is the merge gate per constitution Principle IV and `AGENTS.md`. Fix any lint or test failures before proceeding.
- [X] T023 Build the API image and verify no Python/venv/Typst is present: run `make images`, then `docker run --rm job-finder-api:local-check sh -c 'which python3 python pip typst; test $? -ne 0 && echo OK'` and `docker run --rm job-finder-api:local-check sh -c 'ls /opt/rendercv-venv 2>/dev/null && echo FAIL || echo OK'`. Record the image size via `docker image inspect job-finder-api:local-check --format '{{.Size}}'` and confirm it is measurably smaller than the pre-migration image (FR-009, SC-004, quickstart.md Scenarios 8, 10).
- [X] T024 If the comparison test (T007) found any text or page-count differences between the old and new engine, record each finding with an accept-or-fix decision in `apps/api/internal/generation/infrastructure/testdata/compare/findings.md` (FR-013). If no differences were found, note that explicitly. This task is conditional — only required if T007 surfaced differences.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately.
- **Foundational (Phase 2)**: Depends on T001 (the `rendercv-go` dependency must be resolvable). **BLOCKS all user stories** — the constructor signature change forces a compile-breaking change across multiple files.
- **User Story 1 (Phase 3)**: Depends on Phase 2. T006 (golden capture) should ideally run before T002 (the rewrite) changes the renderer, or using the old CLI installed separately; T007 depends on T002 and T006.
- **User Story 2 (Phase 4)**: Depends on Phase 2 (the error classification logic is in the rewritten `Render` method from T002). All US2 tests can run in parallel.
- **User Story 3 (Phase 5)**: Depends on Phase 2 (T005 updated the profile service constructor). The test uses the already-rewritten renderer.
- **User Story 4 (Phase 6)**: Depends on Phase 2 (the code must compile before the image can be built). All US4 tasks are independent of each other and can run in parallel.
- **Polish (Phase 7)**: T021 can run in parallel with US4. T022 depends on all implementation and test tasks (Phases 2–6). T023 depends on T014 (Dockerfile rewrite) and T022. T024 depends on T007.

### User Story Dependencies

- **User Story 1 (P1)**: Depends on Foundational. No dependencies on other stories. 🎯 MVP — deliver this first.
- **User Story 2 (P1)**: Depends on Foundational (the error classification is in T002's `Render` rewrite). No dependencies on US1 — can proceed in parallel with US1 after Foundational.
- **User Story 3 (P2)**: Depends on Foundational (T005). No dependencies on US1/US2.
- **User Story 4 (P2)**: Depends on Foundational (code must compile). No dependencies on US1/US2/US3 — can proceed in parallel.

### Within Each User Story

- Tests validate the already-implemented behavior from the Foundational phase (the `Render` rewrite in T002 includes both the happy path and error classification, since they are the same function).
- US1's golden capture (T006) is a data-gathering prerequisite for the comparison test (T007).
- All test tasks marked [P] can run in parallel within a story.

### Parallel Opportunities

- **Phase 2**: T002–T005 are sequential (compile-breaking change across files — must land together).
- **Phase 3**: T006, T008, T009 can run in parallel (different files). T007 depends on T002 + T006.
- **Phase 4**: T010, T011, T012 can all run in parallel (same test file but different test functions — no conflict if written together; mark [P] for parallel dispatch).
- **Phase 5**: T013 is a single task.
- **Phase 6**: T014–T020 can all run in parallel (different files).
- **Phase 7**: T021 can run in parallel with Phase 6. T022–T024 are sequential.

---

## Parallel Example: User Story 1

```bash
# Launch golden capture + live test update + page-count test together:
Task: "Capture golden output from old engine in testdata/compare/golden/"
Task: "Update live test in rendercv_live_test.go"
Task: "Write page-count test in rendercv_renderer_test.go"

# After T002 (rewrite) and T006 (goldens) complete:
Task: "Write comparison test in rendercv_compare_test.go"
```

## Parallel Example: User Story 4

```bash
# Launch all deployment tasks together (all different files):
Task: "Rewrite Dockerfile runtime stage in apps/api/Dockerfile"
Task: "Remove RENDERCV_BIN from docker-compose.prod.yml"
Task: "Annotate RENDERCV_BIN in .env.example"
Task: "Update local-development.md"
Task: "Update configuration-reference.md"
Task: "Update generation.md"
Task: "Update testing.md"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001 — add dependency)
2. Complete Phase 2: Foundational (T002–T005 — engine swap + wiring) — **CRITICAL, blocks all stories**
3. Complete Phase 3: User Story 1 (T006–T009 — golden capture + comparison test + live test + page-count test)
4. **STOP and VALIDATE**: Run `go test ./internal/generation/infrastructure/ -run "TestLive|TestRenderCvCompare|TestCountPages" -v` and confirm the happy path works
5. If the comparison test passes, the core migration is proven — deploy/demo if ready

### Incremental Delivery

1. Setup + Foundational → engine swap compiles, old Python subprocess is gone from the code
2. Add User Story 1 → comparison proves parity → **MVP** (the primary product output is validated)
3. Add User Story 2 → error classification is validated → invalid documents and cancellation are safe
4. Add User Story 3 → smoke render is validated → profile saves are guarded
5. Add User Story 4 → image is rebuilt, docs updated → deployment payoff (SC-004)
6. Polish → merge gate, image verification, findings recorded

### Parallel Team Strategy

With multiple developers after Foundational (Phase 2) completes:
- Developer A: User Story 1 (comparison test — the critical parity proof)
- Developer B: User Story 2 (error/cancellation tests)
- Developer C: User Story 4 (Dockerfile + docs — all independent files)
- User Story 3 is small enough to pick up by anyone finishing early

---

## Notes

- The core engine swap (T002) is in the Foundational phase, not a user story phase, because the constructor signature change is a compile-breaking prerequisite for all stories. The user story phases validate the swap against each story's acceptance criteria.
- The `Render` method rewrite (T002) includes both the happy path (US1) and error classification (US2) because they are the same function — you cannot write `Render` without handling both success and error from `rendercv.Build`. US2's tests validate the error behaviour that T002 already implements.
- Golden capture (T006) is a one-time data-gathering step. It can run before or after the code rewrite (the old `rendercv` CLI is an external tool that can be installed independently), but the comparison test (T007) needs both the goldens and the rewritten renderer.
- The `RENDERCV_BIN` config field and default are intentionally **kept** for one release (FR-010, R-009). They are loaded by viper but never read by the renderer. Do not remove `config.go:61` or `defaults.go:16` in this feature.
- No DB migration, no DTO change, no sqlc/tygo regeneration is needed — the `RendercvMaster` type and `Render` signature are held fixed (data-model.md §1, §6).
- Commit after each task or logical group. Use conventional commit format: `feat:`, `fix:`, `chore:`, `docs:`.
- Stop at any checkpoint to validate a story independently.