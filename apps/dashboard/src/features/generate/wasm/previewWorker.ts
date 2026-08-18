

import { buildTypst, ensureRendercvWasmLoaded } from './rendercvWasm';
import { compilePdf, ensureTypstWasiLoaded } from './typstWasi';

export type PreviewWorkerRequest =
  | { id: number; kind: 'warm' }
  | { id: number; kind: 'render'; yaml: string };

export type PreviewWorkerResponse =
  | { id: number; ok: true; kind: 'warm' }
  | { id: number; ok: true; kind: 'render'; pdfBytes: Uint8Array }
  | { id: number; ok: false; message: string };

self.onmessage = async (event: MessageEvent<PreviewWorkerRequest>) => {
  const request = event.data;
  try {
    if (request.kind === 'warm') {
      await Promise.all([ensureRendercvWasmLoaded(), ensureTypstWasiLoaded()]);
      post({ id: request.id, ok: true, kind: 'warm' });
      return;
    }
    const typst = await buildTypst(request.yaml);
    const pdfBytes = await compilePdf(typst);

    post({ id: request.id, ok: true, kind: 'render', pdfBytes }, [pdfBytes.buffer as ArrayBuffer]);
  } catch (err) {
    post({ id: request.id, ok: false, message: err instanceof Error ? err.message : String(err) });
  }
};

function post(response: PreviewWorkerResponse, transfer: Transferable[] = []): void {
  (self as unknown as Worker).postMessage(response, transfer);
}
