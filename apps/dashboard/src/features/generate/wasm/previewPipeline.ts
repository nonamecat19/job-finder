// previewPipeline.ts — orchestrates the real, in-browser resume preview:
// api.generations.previewDocument -> rendercvWasm.buildTypst ->
// typstWasi.compilePdf -> a blob: URL for ResumePreviewPane.tsx to embed.
// See specs/046-real-resume-preview/plan.md and research.md.
import { api } from '../../../lib/api';
import { renderPdfBytes, warmUp } from './previewWorkerClient';

export type PreviewState =
  | { status: 'idle' }
  | { status: 'loading' }
  // pdfBytes is the same document as pdfUrl, kept alongside it because the
  // in-page canvas viewer (PdfPreviewCanvas) needs the bytes to read the
  // text layer — a blob: URL in an <iframe> is opaque to us.
  | { status: 'ready'; pdfUrl: string; pdfBytes: Uint8Array; sectionsHash: string }
  | { status: 'error'; message: string };

/** Pre-fetches both WASM modules and their assets, ahead of the first render. */
export async function warmUpPreviewPipeline(): Promise<void> {
  await warmUp();
}

/**
 * Whether this browser can run the preview pipeline at all — checked up
 * front so an unsupported browser short-circuits straight to the fallback
 * state (spec FR-007 / Acceptance Scenario 2) instead of attempting a load
 * that was always going to fail.
 */
export function isPreviewSupported(): boolean {
  return typeof WebAssembly !== 'undefined' && typeof WebAssembly.instantiate === 'function';
}

/**
 * Renders the given run's current selection to a PDF blob: URL. The caller
 * owns the returned URL's lifetime — revoke the previous one (see
 * ResumePreviewPane.tsx) once a new render replaces it, so blob: URLs don't
 * accumulate across edits.
 */
export async function renderPreview(
  runId: string,
): Promise<{ pdfUrl: string; pdfBytes: Uint8Array; sectionsHash: string }> {
  const { yaml, sectionsHash } = await api.generations.previewDocument(runId);
  const pdfBytes = await renderPdfBytes(yaml);
  // TS's DOM lib types Uint8Array's backing buffer as ArrayBufferLike (which
  // includes SharedArrayBuffer), stricter than BlobPart actually requires at
  // runtime — a Uint8Array is always a valid BlobPart.
  const blob = new Blob([pdfBytes as BlobPart], { type: 'application/pdf' });
  return { pdfUrl: URL.createObjectURL(blob), pdfBytes, sectionsHash };
}

const DEFAULT_DEBOUNCE_MS = 400;

/**
 * A debounced, coalescing scheduler around renderPreview (spec FR-010,
 * research.md Decision 5): rapid successive edits collapse into one render
 * of the latest state, and a render whose input is stale by the time it
 * would resolve is dropped rather than delivered out of order.
 *
 * Call schedule(runId) on every edit; only the last call within the debounce
 * window actually triggers a render. Returns a disposer that cancels any
 * pending timer/in-flight delivery — call it on unmount.
 */
export class PreviewScheduler {
  #debounceMs: number;
  #onChange: (state: PreviewState) => void;
  #timer: ReturnType<typeof setTimeout> | null = null;
  #generation = 0;
  #lastSectionsHash: string | null = null;
  #lastPdfUrl: string | null = null;
  #lastPdfBytes: Uint8Array | null = null;

  constructor(onChange: (state: PreviewState) => void, debounceMs = DEFAULT_DEBOUNCE_MS) {
    this.#onChange = onChange;
    this.#debounceMs = debounceMs;
  }

  schedule(runId: string): void {
    if (!isPreviewSupported()) {
      if (this.#timer !== null) clearTimeout(this.#timer);
      this.#onChange({
        status: 'error',
        message: "Your browser doesn't support the in-browser preview. Export still works below.",
      });
      return;
    }
    if (this.#timer !== null) clearTimeout(this.#timer);
    const generation = ++this.#generation;
    this.#timer = setTimeout(() => {
      this.#timer = null;
      void this.#run(runId, generation);
    }, this.#debounceMs);
  }

  async #run(runId: string, generation: number): Promise<void> {
    this.#onChange({ status: 'loading' });
    try {
      const { yaml, sectionsHash } = await api.generations.previewDocument(runId);
      if (generation !== this.#generation) return; // superseded while awaiting the network

      if (sectionsHash === this.#lastSectionsHash && this.#lastPdfUrl && this.#lastPdfBytes) {
        // Two edits resolved to the same effective document (e.g. a toggle
        // flipped off then back on before the debounce window closed) — the
        // existing render is still correct, skip a redundant WASM pass.
        this.#onChange({
          status: 'ready',
          pdfUrl: this.#lastPdfUrl,
          pdfBytes: this.#lastPdfBytes,
          sectionsHash,
        });
        return;
      }

      const pdfBytes = await renderPdfBytes(yaml);
      if (generation !== this.#generation) return;

      const blob = new Blob([pdfBytes as BlobPart], { type: 'application/pdf' });
      const pdfUrl = URL.createObjectURL(blob);
      if (this.#lastPdfUrl) URL.revokeObjectURL(this.#lastPdfUrl);
      this.#lastPdfUrl = pdfUrl;
      this.#lastPdfBytes = pdfBytes;
      this.#lastSectionsHash = sectionsHash;
      this.#onChange({ status: 'ready', pdfUrl, pdfBytes, sectionsHash });
    } catch (err) {
      if (generation !== this.#generation) return;
      this.#onChange({ status: 'error', message: err instanceof Error ? err.message : String(err) });
    }
  }

  dispose(): void {
    if (this.#timer !== null) clearTimeout(this.#timer);
    this.#generation++; // orphan any in-flight run so its result is dropped
    if (this.#lastPdfUrl) URL.revokeObjectURL(this.#lastPdfUrl);
    this.#lastPdfUrl = null;
    this.#lastPdfBytes = null;
  }
}
