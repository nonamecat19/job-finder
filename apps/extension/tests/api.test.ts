import { beforeEach, describe, expect, it, vi } from 'vitest';

import { filenameFromDisposition, findJobByUrl, getDocumentPdf, ping, sortDocuments } from '@/background/api';
import type { DocumentSummary } from '@/shared/messages';

import { jsonResponse, stubFetch, urlOf } from './fetch-mock';

const BASE = 'http://localhost:3000';

describe('api client', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('joins the base URL with the /api prefix', async () => {
    const fetchMock = stubFetch(async () => jsonResponse({ status: 'ok' }));

    await ping(BASE);

    expect(String(fetchMock.mock.calls[0][0])).toBe('http://localhost:3000/api/health');
  });

  it('always asks for hidden and applied jobs, or an applied vacancy resolves to nothing', async () => {
    const fetchMock = stubFetch(async () => jsonResponse({ items: [], total: 0, page: 1, pageSize: 5 }));

    await findJobByUrl(BASE, 'https://djinni.co/jobs/1-x');

    const url = urlOf(fetchMock, 0);
    expect(url.searchParams.get('url')).toBe('https://djinni.co/jobs/1-x');
    expect(url.searchParams.get('includeHidden')).toBe('true');
    expect(url.searchParams.get('includeApplied')).toBe('true');
  });

  it('reports an unreachable API rather than throwing', async () => {
    stubFetch(async () => { throw new TypeError('Failed to fetch'); });

    const res = await ping(BASE);

    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error.code).toBe('api_unreachable');
  });

  it('maps a 404 from the PDF endpoint to pdf_not_ready', async () => {
    stubFetch(async () => new Response('PDF not rendered yet', { status: 404 }));

    const res = await getDocumentPdf(BASE, 'doc-1', 'fallback.pdf');

    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error.code).toBe('pdf_not_ready');
  });

  it('refuses a PDF too large to survive the message port', async () => {
    const big = new Uint8Array(9 * 1024 * 1024);
    stubFetch(async () => new Response(big, { status: 200 }));

    const res = await getDocumentPdf(BASE, 'doc-1', 'fallback.pdf');

    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error.code).toBe('unknown');
  });

  it('falls back to the supplied name when the server sends no disposition', async () => {
    stubFetch(async () => new Response(new Uint8Array([1, 2, 3]), { status: 200 }));

    const res = await getDocumentPdf(BASE, 'doc-1', 'fallback.pdf');

    expect(res.ok).toBe(true);
    if (res.ok) expect(res.value.name).toBe('fallback.pdf');
  });
});

describe('filenameFromDisposition', () => {
  it('reads the plain ASCII form', () => {
    expect(filenameFromDisposition('attachment; filename="CV_John_Doe.pdf"')).toBe('CV_John_Doe.pdf');
  });

  it('prefers the RFC 5987 form, which is the accurate one for non-ASCII names', () => {
    const header = "attachment; filename=\"CV.pdf\"; filename*=UTF-8''CV_%D0%9E%D0%BB%D0%B5%D0%BA%D1%81%D0%B0%D0%BD%D0%B4%D1%80.pdf";
    expect(filenameFromDisposition(header)).toBe('CV_Олександр.pdf');
  });

  it('returns null when the header is missing', () => {
    expect(filenameFromDisposition(null)).toBeNull();
  });
});

describe('sortDocuments', () => {
  it('puts the newest version first', () => {
    const docs: DocumentSummary[] = [
      { id: 'a', type: 'resume', version: 1, createdAt: '2026-01-01T00:00:00Z', company: null, title: null, hasText: false },
      { id: 'b', type: 'resume', version: 3, createdAt: '2026-01-02T00:00:00Z', company: null, title: null, hasText: false },
      { id: 'c', type: 'resume', version: 3, createdAt: '2026-02-02T00:00:00Z', company: null, title: null, hasText: false },
    ];

    expect(sortDocuments(docs).map((d) => d.id)).toEqual(['c', 'b', 'a']);
  });
});
