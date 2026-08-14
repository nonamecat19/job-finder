# Phase 0 Research: rendercv-go migration

**Branch**: `045-rendercv-go-migration` | **Date**: 2026-08-14

Resolves every NEEDS CLARIFICATION and every integration unknown surfaced in the Technical Context of `plan.md`. Each entry states the decision, the rationale, and the alternatives considered.

Findings are grounded in the actual source of `rendercv-go` at `/home/nnc/Projects/rendercv-go/` (v1.0.0, tag `v1.0.0`) and the existing API code at `/home/nnc/Projects/job-finder/apps/api/`.

---

## R-001: Cancellation and time budget — the public API takes no `context.Context`

**Decision**: Wrap `rendercv.Build` + `rendercv.GenerateTypst` + `rendercv.GeneratePDF` in a goroutine and impose the time budget + cancellation at the call site, not inside the library. Use `context.AfterFunc` (Go 1.21+) on the caller's context to detect cancellation/timeout and abandon the render. The abandoned goroutine continues until the embedded Typst compiler finishes (it cannot be hard-killed through the public API), but it writes its output into a temp path that the caller never reads, so no half-written PDF is presented as complete.

Concretely, the rewritten `Render` method:
1. Derives `cmdCtx, cancel := context.WithTimeout(ctx, 120*time.Second)` (same 120 s budget as today).
2. Writes the YAML to the final `yamlPath` (unchanged — the YAML is the input artifact, always complete before render).
3. Builds into a **temp PDF path** (`<outDir>/<baseName>.pdf.tmp.<pid>`) via `BuildOptions{PDFPath: tmpPdfPath, TypstPath: tmpTypstPath, DontGenerateMarkdown: true, DontGenerateHTML: true, DontGeneratePNG: true, InputFilePath: yamlPath}`.
4. Runs `Build` → `GenerateTypst` → `GeneratePDF` in a goroutine, sending the result on a buffered channel.
5. `select`s between the channel and `<-cmdCtx.Done()`. On `Done()`, returns a `context.DeadlineExceeded`/`context.Canceled` error and best-effort deletes the temp files. The goroutine leaks until the compiler returns; its result is discarded. Since the shared wazero module is compiled once and each render instantiates a fresh module that is closed in a `defer` inside `generate.PDF`, a leaked goroutine's module is reclaimed when the compile call returns — no unbounded resource growth.
6. On success, atomically renames `tmpPdfPath` → `pdfPath` (the final path), then uploads. This is the "no half-written output presented as complete" guarantee (spec Edge Cases, FR-006).

**Rationale**: `rendercv-go`'s public API (`Build`, `GenerateTypst`, `GeneratePDF`) deliberately takes no `context.Context` — it mirrors upstream Python, which takes none. Internally the embedded Typst compiler *does* honor a context (`typstc.Compile(ctx, ...)`, `WithCloseOnContextDone(true)`), but `generate.PDF` hardcodes `context.Background()`. There is no public way to thread a context through. The abandon-and-rename pattern is the standard Go approach for wrapping a non-cancellable operation: the caller's contract (bounded time, cancellation, no partial output) is honoured even though the underlying call is not cancellable. The temp-path-then-rename is already how a safe file write works and adds no new concept.

**Alternatives considered**:
- *Fork `rendercv-go` to add `context.Context` parameters.* Rejected: the public API is frozen at v1.0.0 and the project is a separate module. A fork breaks the parity contract and the frozen-API guarantee, and creates an ongoing maintenance burden for a one-line integration concern.
- *Use `exec.CommandContext` to invoke the `rendercv-go` CLI binary.* Rejected by the spec (Assumption, line 128): the replacement is consumed as a library, not a subprocess — direct use is what removes the external-process cost. Re-introducing a subprocess forfeits the main benefit.
- *Hard-kill the goroutine via `runtime.Goexit`/`os.Process`.* Rejected: unsafe, affects the shared wazero runtime, and unnecessary — the temp-path-then-rename pattern already guarantees no partial output reaches the caller.

---

## R-002: Base image — what replaces `python:3.14-slim-bookworm`

**Decision**: Use `debian:bookworm-slim` as the runtime base. Keep `chromium` and `poppler-utils` (apt) and `fonts-liberation`. Drop `python3`, the venv, `pip`, the `rendercv[full]==2.8` install, and the `/opt/rendercv-venv` symlink. The Go binary is static (CGO disabled), so no Go runtime is needed in the image.

**Rationale**: `debian:bookworm-slim` is the minimal base that still has the `chromium` and `poppler-utils` apt packages the unrelated HTML-to-PDF and resume-import paths need (spec Assumption, line 133). `distroless/static` has no apt and no shell, so installing Chromium is impractical there. `alpine` uses musl, which can cause issues with the prebuilt Chromium binary and `chromedp`; the team already uses Debian-based images and the Dockerfile comments confirm `chromium` is the Debian package name that `chromedp` auto-detects. `debian:bookworm-slim` keeps the existing apt packages working with no change beyond removing the Python/venv block.

**Image-size expectation**: `python:3.14-slim-bookworm` + `rendercv[full]` (which pulls Typst, a ~100 MB binary) is on the order of 500–700 MB. `debian:bookworm-slim` + chromium + poppler + a ~30 MB Go binary (with the ~29 MB embedded `typst.wasm` and fonts baked into the binary) should be measurably smaller. SC-004 asks for "measurably smaller," not a target number; the removal of CPython + the venv + the Typst binary is the measurable reduction.

**Alternatives considered**:
- *`gcr.io/distroless/static`* — no apt, no shell; cannot install Chromium. Rejected.
- *`alpine:3.20`* — musl; risk with Chromium/chromedp. Rejected unless the team explicitly wants to switch the unrelated cover-letter path too.
- *`golang:1.26-bookworm-slim`* — carries the Go toolchain the runtime does not need. Rejected for size.

---

## R-003: Parity comparison — how to satisfy FR-013 / Story 1

**Decision**: Add `apps/api/internal/generation/infrastructure/rendercv_compare_test.go` (a normal `go test`, no build tag). It:
1. Loads the existing fixture corpus: `testdata/sample_rendercv.yaml` plus the eval `master.yaml` files under `evaldata/cases/*/master.yaml`.
2. For each document, marshals via `PrepareMasterForMarshal` (the same preparation the production `Render` method uses), then renders through the new `RenderCvRenderer.Render`.
3. Extracts the PDF text via `pdftotext` (poppler, already a dev/CI dependency for resume import) and the page count via `CountPages`.
4. Compares against a **golden set** captured once from the old Python engine (stored under `testdata/compare/golden/<case>.{txt,pages}`), asserting text equality and page-count equality.
5. Any mismatch is recorded in `testdata/compare/findings.md` with a decision to accept (e.g. font-driven pagination drift on a boundary document) or fix, per FR-013.

The golden files are generated by a one-time script that runs the current (Python) renderer over the corpus before the swap — they are committed so the comparison test is deterministic and does not need the old engine at run time. The test itself only needs the new engine + poppler's `pdftotext`, both available in the new image and on a dev machine with poppler installed.

**Rationale**: The spec requires a comparison (Story 1 Independent Test, FR-013, SC-001/SC-002) and an explicit accept-or-fix decision per difference. A Go test with committed goldens is the existing pattern in this codebase (the eval harness uses committed `case.yaml` + `master.yaml`). `pdftotext` is already a CI dependency. Comparing extracted text (not PDF bytes) is the same axis `rendercv-go`'s own conformance suite uses, because Typst embeds timestamps/IDs in PDF bytes that are not stable.

**Alternatives considered**:
- *Run both engines live in the test.* Rejected: requires the Python renderer to be installed at test time, violating FR-011 (tests run without an external renderer). The golden approach captures the old output once.
- *Compare only page count, not text.* Rejected: SC-002 requires extracted text to match.
- *Use the `rendercv-go` conformance corpus directly.* Rejected: that corpus targets upstream parity in general; this feature needs parity specifically for the documents this product produces (the `sample_rendercv.yaml` + eval masters), which use a subset of themes/locales/entry types.

---

## R-004: Page count — `rendercv-go` does not return it

**Decision**: Keep `CountPages(pdfPath) (int, error)` exactly as it is — `github.com/ledongthuc/pdf` opens the rendered PDF and returns `r.NumPage()`. The new `Render` method writes the PDF to the final path (via temp-then-rename, see R-001), and `CountPages` reads it back. No change to the page-fit loop, `renderDeps.countPages`, or any caller.

**Rationale**: `rendercv-go`'s `GeneratePDF` returns only `(path, error)`; the page count is computed internally by the Typst compiler (`typstc.Result.Pages`) but not exposed through the public API. The existing `ledongthuc/pdf` approach works on any valid PDF file regardless of who produced it, so it continues to work on a `rendercv-go`-produced PDF. Keeping `CountPages` unchanged means `defaultRenderDeps`, `exportRenderDeps`, the eval stubs, and the page-fit loop are all untouched — the change is isolated to `Render`.

**Alternatives considered**:
- *Add a `PageCount` method to the `rendercv-go` public API.* Rejected: the API is frozen; a feature request to `rendercv-go` is out of scope for this feature and not needed since `ledongthuc/pdf` already does the job.
- *Parse the page count from the Typst compiler shim's stdout.* Rejected: that is internal to `rendercv-go` and not reachable through the public API.

---

## R-005: Relative path resolution — photo and `fonts/` paths

**Decision**: Pass `InputFilePath: yamlPath` in `BuildOptions`. `rendercv-go` resolves every relative path in the document (e.g. `cv.photo`, custom `fonts/` folder) against the directory of `InputFilePath`, which is `outDir` (where the YAML is written). This matches the current behaviour: today the Python `rendercv` CLI is invoked with `yamlPath` as the input and `cmd.Dir = outDir`, so relative paths resolve against `outDir`. The new approach resolves against `filepath.Dir(yamlPath)` which is `outDir` — identical.

**Rationale**: `rendercv-go`'s `BuildOptions.InputFilePath` is documented as "not merely informational: every relative path in a document resolves against this file's directory" (`options.go:8-16`). `generate.InputDirFor` uses lexical parent, not `filepath.Dir`, but for the absolute `yamlPath` we write, the result is the same directory. The photo is copied next to the `.typ` by `generate.copyPhotoNextToTypst`, and a `<inputdir>/fonts/` folder is searched as an extra font source — both resolve against the YAML's directory, which is where they resolved before.

**Alternatives considered**:
- *Pass `InputFilePath: ""` (resolve against CWD).* Rejected: would break photo and font resolution for documents that use relative paths (spec Edge Cases, line 85).
- *Copy the photo/fonts into a temp dir and point `InputFilePath` there.* Rejected: unnecessary complexity; the YAML is already written to `outDir`, so `outDir` is the correct base.

---

## R-006: Font drift — embedded fixed set vs system fonts

**Decision**: Accept the fixed embedded font set (15 families: EB Garamond, Font Awesome 7, Fontin, Gentium Book Plus, Lato, Mukta, Noto Sans, Open Sans, Open Sauce Sans, Poppins, Raleway, Roboto, Source Sans 3, Ubuntu, XCharter). Documents referencing a font not in this set will fall back to the default the document's theme specifies, exactly as `rendercv-go`'s conformance suite already validates. Any pagination drift on a boundary document is caught by R-003's comparison and recorded per FR-013.

**Rationale**: Today the image installs `fonts-liberation` via apt and the Python `rendercv` ships its own font handling; the actual fonts a document uses are whatever the Typst compiler finds. `rendercv-go` embeds a fixed set vendored into the binary (`internal/renderer/typstc/assets/fonts/`, `//go:embed all:fonts`) and does no system font discovery. The `sample_rendercv.yaml` fixture uses `EB Garamond` and `Source Sans 3` (both embedded); the eval masters are derived from it. The spec Assumption (line 131) explicitly accepts minor typographic differences provided content, section order, and page counts hold — and R-003's comparison is the mechanism that detects a page-count change so it can be recorded rather than silently accepted.

**Alternatives considered**:
- *Install additional fonts into the image and point `rendercv-go` at them.* Rejected unless the comparison finds a document that paginates differently and the decision is to fix it by restoring the font. That is a follow-up finding, not a design decision now.
- *Keep `fonts-liberation` in the apt install.* Kept — it is already in the Dockerfile and is harmless; `rendercv-go` won't discover it, but removing it could affect the unrelated chromedp/Chromium path if Chromium uses it. Leave apt fonts as-is.

---

## R-007: Error classification — invalid document vs internal failure (FR-005)

**Decision**: In the rewritten `Render`, classify the error returned by `rendercv.Build` (the only step that validates) using `errors.As`:
- `*rendercv.UserValidationError` → invalid document. Extract every `ValidationError` record's `SchemaLocation` (a `[]string` field path) and `Message`, and return a wrapped error whose message names the offending field(s), e.g. `fmt.Errorf("rendercv: invalid document: %s: %s", joinLocation(rec.SchemaLocation), rec.Message)`. The caller (generation service / profile smoke render) surfaces this as a user-facing validation failure.
- `*rendercv.UserError` → user-facing failure without a location (rare). Return as a distinct "invalid document" class.
- `*rendercv.InternalError` → renderer broke. Return as `fmt.Errorf("rendercv: internal error: %w", err)` — a different prefix the caller can distinguish.
- Any other error (e.g. from `GenerateTypst`/`GeneratePDF`, or a filesystem error) → treated as an internal failure.

The distinction "this document is invalid" vs "the renderer broke" (spec Story 2, acceptance scenario 2) is made on the error type, not on string matching. The generation service already wraps and logs errors; the smoke render returns the wrapped error to the profile save path which reports it as a profile validation problem.

**Rationale**: `rendercv-go` returns typed errors aliased in `pkg/rendercv/types.go` (`UserValidationError`, `UserError`, `InternalError`, `ValidationError`), all reachable via `errors.As` (the example test `Example_validationErrors` demonstrates the pattern). `UserValidationError.Errors` is a `[]ValidationError` where each record carries `SchemaLocation []string` and `Message string` — exactly the "offending field and reason" FR-005 requires. The current Python renderer returns an opaque stderr string that does not distinguish the two cases reliably; this is a strict improvement.

**Alternatives considered**:
- *Define a sentinel error or a custom error type in the API to carry the classification.* Rejected as over-engineering for now: the wrapped error's prefix (`"rendercv: invalid document:"` vs `"rendercv: internal error:"`) is enough for the caller to distinguish, and the full `*rendercv.UserValidationError` is preserved in the error chain via `%w` so a future caller can `errors.As` it directly. If the generation service later needs programmatic access to the field list, it can `errors.As` the wrapped error — no information is lost.

---

## R-008: Concurrency — concurrent renders must not collide (spec Edge Cases)

**Decision**: The temp-path-then-rename pattern from R-001 uses a per-render unique temp suffix (`<baseName>.pdf.tmp.<pid>.<n>` or `os.CreateTemp` in `outDir`), so concurrent renders of the same `baseName` (rare but possible if two workers race) write to distinct temp files and the rename is atomic per render. The shared wazero Typst module is compiled once per process (`sync.OnceValues`) and each render instantiates a fresh module that is closed in its own `defer` — no shared mutable state across concurrent renders. `Build` and the generators operate on the passed-in `Model`/options, which are per-call.

**Rationale**: The spec Edge Cases (line 86) requires that several resumes rendering at once do not collide over output paths or shared temporary state. The current `exec.CommandContext` approach avoids collision because each subprocess writes to `<baseName>.pdf` and concurrent renders of the same base name would actually collide today (a latent bug). The temp-then-rename approach is strictly safer. `rendercv-go`'s design (once-compiled module, per-call instantiation) is built for concurrency, though it does not contractually document goroutine-safety — the absence of shared mutable state in the `Build`/`Generate*` path is the basis for treating it as safe.

**Alternatives considered**:
- *Mutex around `Render`.* Rejected: unnecessary; the temp-path pattern already prevents file collisions and the library has no shared mutable state.
- *Pre-check for an existing `<baseName>.pdf`.* Rejected: the production code already generates unique `baseName`s with version/timestamp suffixes (`service.go:219-226`), so same-name collisions are already rare in the generation path; the temp pattern is the safety net for the smoke render and any edge case.

---

## R-009: The `RENDERCV_BIN` setting — retire but tolerate (FR-010)

**Decision**: Keep the `RendercvBin` field in `config.Config` and the `RENDERCV_BIN` default in `defaults.go` for one release. The rewritten `NewRenderCvRenderer` constructor drops the `bin` parameter (the library has no binary to invoke), so `cfg.RendercvBin` is simply not passed. An operator whose env still sets `RENDERCV_BIN` sees it loaded into the config struct (viper `AutomaticEnv` binds it) but never read — startup succeeds normally. Remove `RENDERCV_BIN` from `docker-compose.prod.yml`'s explicit env allowlist and annotate it as obsolete in `.env.example` and `configuration-reference.md`. After one release, remove the field and default entirely.

**Rationale**: FR-010 requires that an environment still supplying the obsolete setting starts normally. Viper loads it into the struct; nothing reads it; no error. The constructor signature change is the natural place to drop the parameter. Keeping the field for one release is the minimal-cost tolerance mechanism.

**Alternatives considered**:
- *Keep `NewRenderCvRenderer(outDir, bin string)` and ignore `bin` internally.* Rejected: a silent unused parameter is worse than removing it; callers (`compose.go`, `profile/application/service.go`) are few and in-repo, so updating them is trivial and the signature stays honest.
- *Fail fast if `RENDERCV_BIN` is set.* Rejected: violates FR-010.

---

## R-010: The smoke render — temp dir and cleanup (FR-008)

**Decision**: The profile smoke render (`profile/application/service.go` `SaveConfig`) is unchanged in structure: it creates a temp dir, constructs a throwaway `RenderCvRenderer` pointed at it, calls `Render`, and `defer os.RemoveAll(tempDir)`. The only change is the constructor call drops the `bin` argument (`NewRenderCvRenderer(tempDir)` instead of `NewRenderCvRenderer(tempDir, s.rendercvBin)`). The `rendercvBin` field on the profile service becomes unused and is removed from `NewService`'s signature (or kept and ignored for one release if call-site churn is a concern — but the call sites are in-repo and few).

**Rationale**: The smoke render's contract (FR-008: run with the new engine, remove temp output afterwards) is satisfied by the existing `defer os.RemoveAll` plus the new engine running in-process. No temp files are left behind because `Render`'s temp-then-rename cleans its own temp files on both success and cancellation (R-001), and the smoke render's temp dir is removed wholesale on return. `Store` is nil on the smoke renderer (already the case), so nothing is uploaded.

**Alternatives considered**:
- *Skip the smoke render and rely on `ParseRendercv` for validation.* Rejected: the spec (Story 3) explicitly keeps the smoke render as a safeguard; `ParseRendercv` validates schema but not renderability (e.g. a photo path that does not resolve).

---

## R-011: The `rendercv-go` dependency — how it enters the build

**Decision**: Add `github.com/nonamecat19/rendercv-go v1.0.0` to `apps/api/go.mod` via `go get github.com/nonamecat19/rendercv-go@v1.0.0` from the `apps/api` module directory. The module is at a frozen, tagged release (v1.0.0, tag exists). No `replace` directive is needed — it is a normal remote dependency. `go mod tidy` updates `go.sum`. The embedded `typst.wasm` (~29 MB) and fonts are compiled into the Go binary via `//go:embed`, so the binary grows but no external file is needed at runtime.

**Rationale**: The spec Assumption (line 134) says "the replacement module is reachable as a normal dependency from the application's build, at its frozen released version." v1.0.0 is that version (tagged, `CHANGELOG.md` confirms the freeze). A `replace` directive pointing at the local `../rendercv-go` checkout would work for development but must not ship — a normal `require` is correct for the committed `go.mod`.

**Alternatives considered**:
- *`replace github.com/nonamecat19/rendercv-go => ../rendercv-go`* — only for local dev; must not be committed. Rejected for the committed `go.mod`.
- *Vendor the module into `apps/api/vendor`.* Rejected: the project does not use vendoring today (no `vendor/` directory); introducing it for one dependency is inconsistent.

---

## R-012: Documentation updates (FR-012)

**Decision**: Update the following to match the new arrangement:
- `docs/docs/operations/local-development.md`: remove `rendercv` from the prerequisites table (a dev no longer needs it on PATH).
- `docs/docs/operations/configuration-reference.md`: mark `RENDERCV_BIN` as obsolete/ignored, keep the row for one release.
- `docs/docs/ai/generation.md`: change "shells out to the `RENDERCV_BIN` binary" to "renders in-process via `rendercv-go`" (the `RenderCvRenderer` section, ~line 130-139).
- `docs/docs/operations/testing.md`: the live test no longer needs the real binary on PATH.
- `specs/domains/resume-generation.md` §7.1: update the description from "shells out to the Typst-based `rendercv` binary" to "renders in-process via the `rendercv-go` library; the embedded WASI Typst compiler and a fixed font set are compiled into the Go binary."
- `.env.example`: annotate `RENDERCV_BIN` as deprecated/ignored.
- `README.md`: no change needed (it does not mention rendercv today).
- `docker-compose.prod.yml`: remove the `RENDERCV_BIN` env line (or leave it; tolerated).

**Rationale**: FR-012 requires docs match the new arrangement. The domain spec `resume-generation.md` §7.1 is the durable record of how the page fitter shipped and explicitly describes the subprocess; it must be updated when the feature ships (the feature's own `spec.md` directory is deleted on ship, per the workflow).

**Alternatives considered**: none — the list is exhaustive for the docs that mention rendering.