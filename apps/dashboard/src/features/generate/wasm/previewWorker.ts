// previewWorker.ts — the resume preview's WASM work, off the main thread.
//
// Both halves of the pipeline block the thread they run on: rendercv's Go
// runtime does its YAML -> Typst pass synchronously, and typstwasm's WASI
// entrypoint (`wasi.start`) is a straight synchronous call that returns only
// once the whole document has been compiled. On the main thread that is a
// multi-hundred-millisecond freeze per render — the browser's own "long task"
// warning. Here it costs the user nothing: the page keeps painting and the
// only thing that crosses back is the finished PDF's bytes.
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
    // Transferred, not copied: the bytes are megabytes and this worker has no
    // further use for them.
    post({ id: request.id, ok: true, kind: 'render', pdfBytes }, [pdfBytes.buffer as ArrayBuffer]);
  } catch (err) {
    post({ id: request.id, ok: false, message: err instanceof Error ? err.message : String(err) });
  }
};

function post(response: PreviewWorkerResponse, transfer: Transferable[] = []): void {
  (self as unknown as Worker).postMessage(response, transfer);
}
