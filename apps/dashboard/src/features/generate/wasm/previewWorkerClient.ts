

import type { PreviewWorkerRequest, PreviewWorkerResponse } from './previewWorker';

type DistributiveOmit<T, K extends PropertyKey> = T extends unknown ? Omit<T, K> : never;

interface Pending {
  resolve: (response: Extract<PreviewWorkerResponse, { ok: true }>) => void;
  reject: (err: Error) => void;
}

let worker: Worker | null = null;
let sequence = 0;
const pending = new Map<number, Pending>();

function workerSupported(): boolean {
  return typeof Worker !== 'undefined';
}

function ensureWorker(): Worker {
  if (worker) return worker;
  worker = new Worker(new URL('./previewWorker.ts', import.meta.url), { type: 'module' });
  worker.onmessage = (event: MessageEvent<PreviewWorkerResponse>) => {
    const response = event.data;
    const entry = pending.get(response.id);
    if (!entry) return;
    pending.delete(response.id);
    if (response.ok) entry.resolve(response);
    else entry.reject(new Error(response.message));
  };

  worker.onerror = (event) => {
    const message = event.message || 'resume preview: the preview worker stopped unexpectedly';
    for (const entry of pending.values()) entry.reject(new Error(message));
    pending.clear();
    worker?.terminate();
    worker = null;
  };
  return worker;
}

function send(request: DistributiveOmit<PreviewWorkerRequest, 'id'>): Promise<Extract<PreviewWorkerResponse, { ok: true }>> {
  const id = ++sequence;
  const instance = ensureWorker();
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
    instance.postMessage({ ...request, id } as PreviewWorkerRequest);
  });
}

export async function renderPdfBytes(yaml: string): Promise<Uint8Array> {
  if (!workerSupported()) {

    const [{ buildTypst }, { compilePdf }] = await Promise.all([import('./rendercvWasm'), import('./typstWasi')]);
    return compilePdf(await buildTypst(yaml));
  }
  const response = await send({ kind: 'render', yaml });
  if (response.kind !== 'render') throw new Error('resume preview: unexpected worker response');
  return response.pdfBytes;
}

export async function warmUp(): Promise<void> {
  if (!workerSupported()) {
    const [{ ensureRendercvWasmLoaded }, { ensureTypstWasiLoaded }] = await Promise.all([
      import('./rendercvWasm'),
      import('./typstWasi'),
    ]);
    await Promise.all([ensureRendercvWasmLoaded(), ensureTypstWasiLoaded()]);
    return;
  }
  await send({ kind: 'warm' });
}
