# Phase 1 Data Model: Real Resume Preview on the Generate Page

This feature adds no database table, column, or migration. Every entity below is either an existing persisted entity (reused as-is) or an ephemeral, client-side-only value that is never stored.

## Existing entities (unchanged, reused)

- **Generation Run** (`sqlcgen.GenerationRun`, existing) — the workspace session on the Generate page. This feature reads its current section-selection state; it does not add fields to it.
- **Master Profile / `domain.RendercvMaster`** (existing) — the source-of-truth resume content the preview and the export both assemble from via `domain.Assemble`. Unchanged.

## New ephemeral entities (not persisted)

- **Preview Document** — the RenderCV YAML text returned by the new preview-document endpoint (`contracts/preview-document.md`). Produced fresh on each request from `domain.Assemble` + existing marshal logic; held only in the HTTP response and, transiently, in the browser's memory during one preview render. Never written to disk or blob storage (unlike the export path's `.yaml`/`.pdf` files) — that is the deliberate difference between preview and export.
- **Preview Render** — the PDF bytes produced client-side by the browser WASM pipeline (`GenerateTypst` → `typstwasm`) from one Preview Document. Held only as a `blob:` URL in the browser tab for as long as the current preview is displayed; superseded and discarded on the next debounced edit (research.md Decision 5). Never uploaded or sent back to the server.
- **`sectionsHash`** — a content hash of the section-selection state, carried alongside a Preview Document (contract's response) so the client can detect when two edits resolved to the same effective document and skip a redundant WASM render. Computed per-request, not stored.

## State transitions

None of the above have a persisted lifecycle. The only "transition" worth naming is the client-side preview pipeline's own states, which are UI state (not data model), already covered by the functional requirements:

`idle → loading (FR-005) → rendered | error (FR-006)`, re-entering `loading` on the next debounced edit (FR-003, FR-010), independent of and non-blocking to the existing `export` state machine (FR-007).
