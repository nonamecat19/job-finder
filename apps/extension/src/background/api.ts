import type { JobDto, JobListResponse, ManualAddResultDto } from '@job-finder/shared';

import { bytesToBase64 } from '@/shared/base64';
import type { DocumentSummary, JobSummary } from '@/shared/messages';
import { attempt, err, ok, type Result } from '@/shared/result';

/**
 * The ONLY module in the extension that calls fetch(). Two reasons:
 *
 *  - The vacancy pages are https and the API is http://localhost:3000. Only an
 *    extension context (worker/pages) is exempt from the page's mixed-content
 *    policy, and only it carries the host permission.
 *  - Keeping every request here means the page can never reach the local API
 *    through us, and adding auth later stays a one-file change.
 */

/** A base64 payload larger than this is refused rather than risking a silent port failure. */
const MAX_PDF_BYTES = 8 * 1024 * 1024;

export type PdfPayload = { name: string; base64: string };

/**
 * What the extension reads off a generated document. Structural on purpose:
 * the same document reaches us both standalone and embedded in a JobDto, and
 * those two arrive with slightly different optionality in the generated types.
 */
export type DocumentLike = {
  id: string;
  type: string;
  version: number;
  createdAt: string;
  company?: string | null;
  title?: string | null;
  content?: unknown;
};

/** What the extension reads off a job. */
export type JobLike = {
  id: string;
  title: string;
  company: string;
  url: string;
  status: string;
};

type RequestOpts = { method?: string; body?: unknown };

async function request<T>(baseUrl: string, path: string, opts: RequestOpts = {}): Promise<Result<T>> {
  let res: Response;
  try {
    res = await fetch(`${baseUrl}/api${path}`, {
      method: opts.method ?? 'GET',
      headers: opts.body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
    });
  } catch (e) {
    return err('api_unreachable', `Can't reach job-finder at ${baseUrl}`, e instanceof Error ? e.message : String(e));
  }
  if (res.status === 404) {
    return err('not_found', 'Not found in job-finder.', await res.text());
  }
  if (!res.ok) {
    return err('bad_request', `job-finder returned ${res.status}`, await res.text());
  }
  return ok((await res.json()) as T);
}

export function ping(baseUrl: string): Promise<Result<{ status?: string }>> {
  return request(baseUrl, '/health');
}

/**
 * Exact-URL lookup, backed by the ?url= filter on /api/jobs.
 *
 * includeHidden/includeApplied are not optional: a vacancy you are applying to
 * is often already 'applied' or 'hidden', and both are excluded by default.
 */
export async function findJobByUrl(baseUrl: string, url: string): Promise<Result<JobDto | null>> {
  const params = new URLSearchParams({
    url,
    includeHidden: 'true',
    includeApplied: 'true',
    pageSize: '5',
  });
  const res = await request<JobListResponse>(baseUrl, `/jobs?${params}`);
  if (!res.ok) return res;
  return ok(res.value.items[0] ?? null);
}

/** Free-text narrowing used only when the exact URL match misses. */
export async function searchJobs(baseUrl: string, q: string): Promise<Result<JobDto[]>> {
  const params = new URLSearchParams({
    q,
    includeHidden: 'true',
    includeApplied: 'true',
    pageSize: '100',
  });
  const res = await request<JobListResponse>(baseUrl, `/jobs?${params}`);
  if (!res.ok) return res;
  return ok(res.value.items);
}

/** One request for job + documents: /jobs/{id} embeds them already. */
export function getJob(baseUrl: string, jobId: string): Promise<Result<JobDto>> {
  return request<JobDto>(baseUrl, `/jobs/${encodeURIComponent(jobId)}`);
}

export function listDocuments(baseUrl: string, jobId: string): Promise<Result<DocumentLike[]>> {
  return request<DocumentLike[]>(baseUrl, `/jobs/${encodeURIComponent(jobId)}/documents`);
}

export function getDocument(baseUrl: string, documentId: string): Promise<Result<DocumentLike>> {
  return request<DocumentLike>(baseUrl, `/documents/${encodeURIComponent(documentId)}`);
}

/**
 * Writes. Scrapes the URL server-side and can create a job, so this is only
 * ever called from an explicit user click, never as part of resolution.
 */
export function addVacancy(baseUrl: string, url: string): Promise<Result<ManualAddResultDto>> {
  return request<ManualAddResultDto>(baseUrl, '/jobs/manual', { method: 'POST', body: { url } });
}

export async function getDocumentPdf(baseUrl: string, documentId: string, fallbackName: string): Promise<Result<PdfPayload>> {
  return attempt(async () => {
    let res: Response;
    try {
      res = await fetch(`${baseUrl}/api/documents/${encodeURIComponent(documentId)}/pdf`);
    } catch (e) {
      return err('api_unreachable', `Can't reach job-finder at ${baseUrl}`, e instanceof Error ? e.message : String(e));
    }
    // The handler 404s both when pdfPath is null and when the file is missing on
    // disk, so a non-null pdfPath in the DTO proves nothing — only this response does.
    if (res.status === 404) {
      return err('pdf_not_ready', "PDF hasn't been rendered yet — open it in the dashboard to render.", await res.text());
    }
    if (!res.ok) {
      return err('bad_request', `job-finder returned ${res.status}`, await res.text());
    }
    const buf = await res.arrayBuffer();
    if (buf.byteLength > MAX_PDF_BYTES) {
      return err('unknown', 'PDF is too large to attach.');
    }
    const name = filenameFromDisposition(res.headers.get('content-disposition')) ?? fallbackName;
    return ok({ name, base64: bytesToBase64(new Uint8Array(buf)) });
  });
}

/**
 * The server emits `filename*=UTF-8''...` alongside the plain `filename=` when
 * the name is non-ASCII (Cyrillic names are the common case), and the encoded
 * form is the accurate one.
 */
export function filenameFromDisposition(header: string | null): string | null {
  if (!header) return null;
  const extended = /filename\*=UTF-8''([^;]+)/i.exec(header);
  if (extended) {
    try {
      return decodeURIComponent(extended[1].trim());
    } catch {
      // fall through to the plain form
    }
  }
  const plain = /filename="([^"]+)"/i.exec(header) ?? /filename=([^;]+)/i.exec(header);
  return plain ? plain[1].trim() : null;
}

export function toJobSummary(job: JobLike): JobSummary {
  return { id: job.id, title: job.title, company: job.company, url: job.url, status: job.status };
}

export function toDocumentSummary(doc: DocumentLike): DocumentSummary {
  return {
    id: doc.id,
    type: doc.type as DocumentSummary['type'],
    version: doc.version,
    createdAt: doc.createdAt,
    company: doc.company ?? null,
    title: doc.title ?? null,
    hasText: documentText(doc) !== null,
  };
}

/** Cover letters carry their body as `content.text`, the same shape the dashboard reads. */
export function documentText(doc: DocumentLike): string | null {
  const content = doc.content as { text?: unknown } | null | undefined;
  if (content && typeof content.text === 'string' && content.text.trim() !== '') return content.text;
  return null;
}

/** Newest first: version dominates, createdAt breaks ties. */
export function sortDocuments(docs: DocumentSummary[]): DocumentSummary[] {
  return [...docs].sort((a, b) => b.version - a.version || Date.parse(b.createdAt) - Date.parse(a.createdAt));
}
