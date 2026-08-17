# Phase 0 Research: Real Resume Preview on the Generate Page

## Decision 1: Don't nest `rendercv-go`'s `GeneratePDF` in the browser — split at `GenerateTypst`

**Decision**: The browser build of `pkg/rendercv` calls only `ReadYAML`, `Build`, and `GenerateTypst`. `GeneratePDF` is not used client-side.

**Rationale**: `GeneratePDF` internally runs `typstwasm` (a Rust `wasm32-wasip1` binary) through `wazero`, a pure-Go WASI *host*. `wazero`'s fast path is a compiler that emits native machine code at call time; that requires executable memory, which a `GOOS=js GOARCH=wasm` binary running inside the browser's own WASM sandbox does not have. `wazero` would fall back to its interpreter, and the browser build would then be running an interpreter (Go-in-WASM) driving an interpreter (`wazero`) driving compiled WASM (`typstwasm`) — three layers deep for the step that matters most for the 3-second budget (spec SC-003). Stopping at `GenerateTypst` keeps the Go-WASM layer to schema validation and Pongo2/Lua templating (CPU-light, string-in/string-out), and lets the PDF compilation step talk to `typstwasm` directly (Decision 2) with only one interpretation layer, not three.

**Alternatives considered**:
- Ship `GeneratePDF` as-is in the browser build and accept `wazero`'s interpreter fallback. Rejected: the risk to the 3-second refresh target is large and unmeasured; `rendercv-go`'s own README already flags `GeneratePDF`'s wasm runtime as "a process-wide singleton" sized for a server process, not a browser tab.
- Render server-side per edit (skip client WASM entirely). Rejected by the spec itself — FR-002 requires the preview not depend on a full export round trip, and a render-per-keystroke server call is the exact latency/cost problem the feature exists to avoid.

## Decision 2: Run `typstwasm` in-browser via a WASI shim, not `wazero`

**Decision**: Load `tools/typstwasm`'s existing `wasm32-wasip1` binary directly with a small browser-side WASI polyfill (e.g. `@bjorn3/browser_wasi_shim`) instead of trying to run `wazero` itself as a `GOOS=js/wasm` binary.

**Rationale**: `typstwasm` is already a real, standard WASI binary (confirmed: `tools/typstwasm/README.md` — "built for `wasm32-wasip1` and driven through wazero"). Browsers can instantiate WASI binaries directly via `WebAssembly.instantiate` plus a WASI polyfill that implements the handful of syscalls (`fd_read`, `fd_write`, `path_open`, clock, random) the sandboxed compiler needs — this is a well-trodden pattern (browser WASI shims exist specifically for this). It avoids the triple-nesting of Decision 1 entirely: browser WASM host → `typstwasm`, one layer.

The binary expects three preopens (`--root`, `--font-dir`, `--pkg`) and two non-obvious inputs the tool's own README documents: the `@preview/fontawesome:0.6.0` package (not vendored, normally fetched from Typst Universe) and `typst_assets::fonts()` (an embedded third font source some themes need). All three must be bundled as static files under a virtual filesystem the WASI shim serves from memory/IndexedDB rather than a real disk.

**Alternatives considered**:
- Recompile Typst directly to `wasm32-unknown-unknown` (browser-native target, no WASI shim needed). Rejected for this feature: `typstwasm`'s `Cargo.toml` and `main.rs` are already built and parity-tested against `wasm32-wasip1`; retargeting it is `rendercv-go`-repo work outside this feature's scope and duplicates a compiler port the sibling repo already owns. Worth revisiting only if the WASI-shim path proves too slow or unreliable in practice.
- Server-side PDF compile, client-side everything else. Rejected: same round-trip problem as Decision 1's alternative.

## Decision 3: New preview-document endpoint reuses `domain.Assemble`, adds no new merge logic

**Decision**: Add one read-only backend endpoint that calls the same `domain.Assemble(master, sections)` the export path (`workspace_export.go:renderExport`) already calls, serializes the result with the same `domain.PrepareMasterForMarshal` + `yaml.Marshal` path `RenderCvRenderer.Render` already uses (`infrastructure/rendercv_renderer.go:39-44`), and returns the YAML text — without calling `ApplyFontSize`, `render`, or `countPages`.

**Rationale**: The client cannot accurately assemble the RenderCV document itself — the merge from master profile + current section toggles (`domain.Assemble`) is server-side domain logic with no client copy today, and duplicating it in TypeScript would violate Constitution III (typed contracts, single source of truth) and would drift from the export path the moment either copy changed. Stopping the existing assembly pipeline one step earlier than export does is the smallest correct change: same inputs, same merge function, no new type, no new YAML shape.

**Alternatives considered**:
- Return the pre-toggle master YAML once and apply toggles client-side. Rejected: toggle/selection semantics (`domain.ApplySectionToggles`, ranking, trimming) live in Go domain code the client would have to reimplement to stay accurate — the exact drift Constitution III exists to prevent.
- Skip the endpoint and have the client send its raw selection state to a full existing render endpoint, discarding the PDF, keeping only the YAML the server wrote to disk. Rejected: wasteful (compiles a PDF server-side just to throw it away) and slower than the feature's own 3-second budget allows.

## Decision 4: Native browser PDF embedding as the default preview surface

**Decision**: Display the rendered PDF bytes via a `blob:` URL in an `<iframe>` (or `<embed type="application/pdf">`), using the browser's built-in PDF renderer. Fall back to `pdf.js` only if manual testing shows inconsistent behavior across the dashboard's supported browsers.

**Rationale**: Every target browser (Chrome, Firefox, Safari, Edge) ships a native PDF viewer capable of rendering a `blob:` URL with no extra dependency, no extra bundle weight, and native support for the multi-page scroll spec FR-009 requires. Reaching for `pdf.js` first would add a second WASM-adjacent rendering pipeline for no proven benefit.

**Alternatives considered**:
- `pdf.js` from the start, for rendering consistency and finer control (page thumbnails, zoom). Deferred, not rejected outright — kept as the documented fallback (plan.md) if native embedding proves inconsistent.

## Decision 5: Debounce and coalesce edits before touching either WASM module

**Decision**: The preview pipeline (`previewPipeline.ts`) debounces section/content edits (industry-standard ~300–500 ms idle window) before calling the preview-document endpoint, and cancels/ignores any in-flight render whose input is now stale when a newer edit arrives.

**Rationale**: Satisfies FR-010 (no flicker, no falling behind on rapid edits) and keeps the 3-second SC-003 budget meaningful — it is measured from when the user stops editing, not from every keystroke. Standard "coalesce to latest" debounce, no new pattern needed for this codebase.

**Alternatives considered**: None seriously — this is a standard UI debounce problem with a well-known solution; no domain-specific wrinkle here.

## Open risk carried into implementation (not blocking planning)

`typstwasm` is ~29 MB uncompressed (`tools/typstwasm/README.md`). Lazy-loading it (plan.md's Constraints) and caching it via the browser's Cache API keeps this to a one-time-per-browser cost, but first-preview latency on a cold cache should be measured early in implementation against SC-003's 3-second target — that target is stated (and should be read) as applying to *warm* edits, per plan.md's Performance Goals, not to the very first render of a session.
