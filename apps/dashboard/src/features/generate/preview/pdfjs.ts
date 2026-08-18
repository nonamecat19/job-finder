// pdfjs.ts — the pdf.js plumbing behind PdfPreviewCanvas: a lazily loaded
// module (its worker and DOM assumptions only load on a page that actually
// shows a preview) and the one piece of geometry work worth keeping out of
// the component — turning a page's text layer into normalized rectangles.
import type { PdfTextPiece } from './blockMap';

export interface PdfViewportLike {
  width: number;
  height: number;
  transform: number[];
}

export interface PdfPageLike {
  getViewport(params: { scale: number }): PdfViewportLike;
  getTextContent(): Promise<{ items: unknown[] }>;
  render(params: unknown): { promise: Promise<void>; cancel: () => void };
}

export interface PdfDocumentLike {
  numPages: number;
  getPage(pageNumber: number): Promise<PdfPageLike>;
}

/**
 * Tearing a document down is the loading task's job, not the document's:
 * pdf.js 6 dropped `PDFDocumentProxy.destroy()`, so the task returned by
 * getDocument is what has to be kept around to release the worker.
 */
export interface PdfLoadingTaskLike {
  promise: Promise<PdfDocumentLike>;
  destroy(): Promise<void>;
}

export interface PdfjsLike {
  getDocument(params: { data: Uint8Array }): PdfLoadingTaskLike;
  Util: { transform(a: number[], b: number[]): number[] };
}

let pdfjsPromise: Promise<PdfjsLike> | null = null;

export async function loadPdfjs(): Promise<PdfjsLike> {
  if (!pdfjsPromise) {
    pdfjsPromise = (async () => {
      const pdfjs = await import('pdfjs-dist');
      const worker = await import('pdfjs-dist/build/pdf.worker.min.mjs?url');
      pdfjs.GlobalWorkerOptions.workerSrc = worker.default;
      return pdfjs as unknown as PdfjsLike;
    })();
  }
  return pdfjsPromise;
}

/**
 * The text layer in normalized page coordinates. pdf.js reports a text item's
 * origin on its baseline in PDF space (y up); composing it with the viewport
 * transform puts it in top-left screen space, which is where the rectangles
 * the overlay draws live.
 */
export function buildPieces(pdfjs: PdfjsLike, items: unknown[], viewport: PdfViewportLike): PdfTextPiece[] {
  const pieces: PdfTextPiece[] = [];
  for (const raw of items) {
    const item = raw as { str?: string; transform?: number[]; width?: number; height?: number };
    if (!item.str || !item.transform) continue;
    const tx = pdfjs.Util.transform(viewport.transform, item.transform);
    const height = item.height ?? Math.hypot(tx[2], tx[3]);
    const width = item.width ?? 0;
    const x = tx[4];
    const y = tx[5] - height;
    pieces.push({
      str: item.str,
      rect: {
        x: x / viewport.width,
        y: y / viewport.height,
        w: width / viewport.width,
        h: Math.max(height, 1) / viewport.height,
      },
    });
  }
  return pieces;
}
