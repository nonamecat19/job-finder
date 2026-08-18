import type { JobDto, JobListResponse, ManualAddResultDto } from '@job-finder/shared';

import { bytesToBase64 } from '@/shared/base64';
import type { DocumentSummary, JobSummary } from '@/shared/messages';
import { attempt, err, ok, type Result } from '@/shared/result';

const MAX_PDF_BYTES = 8 * 1024 * 1024;

export type PdfPayload = { name: string; base64: string };

export type DocumentLike = {
  id: string;
  type: string;
  version: number;
  createdAt: string;
  company?: string | null;
  title?: string | null;
  content?: unknown;
};

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

export function getJob(baseUrl: string, jobId: string): Promise<Result<JobDto>> {
  return request<JobDto>(baseUrl, `/jobs/${encodeURIComponent(jobId)}`);
}

export function listDocuments(baseUrl: string, jobId: string): Promise<Result<DocumentLike[]>> {
  return request<DocumentLike[]>(baseUrl, `/jobs/${encodeURIComponent(jobId)}/documents`);
}

export function getDocument(baseUrl: string, documentId: string): Promise<Result<DocumentLike>> {
  return request<DocumentLike>(baseUrl, `/documents/${encodeURIComponent(documentId)}`);
}

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

export function filenameFromDisposition(header: string | null): string | null {
  if (!header) return null;
  const extended = /filename\*=UTF-8''([^;]+)/i.exec(header);
  if (extended) {
    try {
      return decodeURIComponent(extended[1].trim());
    } catch {
      ;
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

export function documentText(doc: DocumentLike): string | null {
  const content = doc.content as { text?: unknown } | null | undefined;
  if (content && typeof content.text === 'string' && content.text.trim() !== '') return content.text;
  return null;
}

export function sortDocuments(docs: DocumentSummary[]): DocumentSummary[] {
  return [...docs].sort((a, b) => b.version - a.version || Date.parse(b.createdAt) - Date.parse(a.createdAt));
}
