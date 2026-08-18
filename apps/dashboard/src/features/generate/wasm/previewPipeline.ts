

import { api } from '../../../lib/api';
import { renderPdfBytes, warmUp } from './previewWorkerClient';

export type PreviewState =
  | { status: 'idle' }
  | { status: 'loading' }

  | { status: 'ready'; pdfUrl: string; pdfBytes: Uint8Array; sectionsHash: string }
  | { status: 'error'; message: string };

export async function warmUpPreviewPipeline(): Promise<void> {
  await warmUp();
}

export function isPreviewSupported(): boolean {
  return typeof WebAssembly !== 'undefined' && typeof WebAssembly.instantiate === 'function';
}

export async function renderPreview(
  runId: string,
): Promise<{ pdfUrl: string; pdfBytes: Uint8Array; sectionsHash: string }> {
  const { yaml, sectionsHash } = await api.generations.previewDocument(runId);
  const pdfBytes = await renderPdfBytes(yaml);

  const blob = new Blob([pdfBytes as BlobPart], { type: 'application/pdf' });
  return { pdfUrl: URL.createObjectURL(blob), pdfBytes, sectionsHash };
}

const DEFAULT_DEBOUNCE_MS = 400;

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
      if (generation !== this.#generation) return;

      if (sectionsHash === this.#lastSectionsHash && this.#lastPdfUrl && this.#lastPdfBytes) {

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
    this.#generation++;
    if (this.#lastPdfUrl) URL.revokeObjectURL(this.#lastPdfUrl);
    this.#lastPdfUrl = null;
    this.#lastPdfBytes = null;
  }
}
