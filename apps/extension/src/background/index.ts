import {
  type BgRequest,
  type BgResponse,
  type DocumentSummary,
  type FillPayload,
  type FillReport,
} from '@/shared/messages';
import { setDebug } from '@/shared/log';
import { attempt, err, ok, type Result } from '@/shared/result';

import * as api from './api';
import { setCachedJobId } from './cache';
import { resolveJob } from './resolve';
import { getSettings, setSettings } from './settings';

chrome.runtime.onMessage.addListener((req: BgRequest, _sender, sendResponse) => {
  // The listener must return true synchronously to keep the port open for the
  // async reply; attempt() guarantees the promise resolves rather than rejects.
  attempt(() => handle(req)).then(sendResponse);
  return true;
});

async function handle(req: BgRequest): Promise<Result<BgResponse>> {
  const settings = await getSettings();
  setDebug(settings.debug);
  const base = settings.apiBaseUrl;

  switch (req.kind) {
    case 'bg/getSettings':
      return ok({ kind: 'bg/settings', settings });

    case 'bg/setSettings':
      return ok({ kind: 'bg/settings', settings: await setSettings(req.settings) });

    case 'bg/ping': {
      const res = await api.ping(req.apiBaseUrl ?? base);
      if (!res.ok) return res;
      return ok({ kind: 'bg/pong', ok: true });
    }

    case 'bg/resolveJob': {
      const job = await resolveJob(base, req.url, req.hints);
      if (!job.ok) return job;
      // The list endpoint omits documents; /jobs/{id} embeds them.
      const full = await api.getJob(base, job.value.id);
      if (!full.ok) return full;
      return ok({
        kind: 'bg/job',
        job: api.toJobSummary(full.value),
        documents: summarize(full.value.documents ?? []),
      });
    }

    case 'bg/listDocuments': {
      const docs = await api.listDocuments(base, req.jobId);
      if (!docs.ok) return docs;
      return ok({ kind: 'bg/documents', documents: summarize(docs.value) });
    }

    case 'bg/addVacancy': {
      const res = await api.addVacancy(base, req.url);
      if (!res.ok) return res;
      const result = res.value;
      const job = result.job ? api.toJobSummary(result.job) : undefined;
      if (job) await setCachedJobId(req.url, job.id);
      return ok({
        kind: 'bg/addResult',
        outcome: result.outcome as 'created' | 'duplicate' | 'needs_fill_in' | 'failed',
        job,
        reason: result.reason ?? undefined,
      });
    }

    case 'bg/fill':
      return fill(base, req.tabId, req.documentId, req.target);
  }
}

async function fill(
  base: string,
  tabId: number,
  documentId: string,
  target: 'file' | 'letter' | 'both',
): Promise<Result<BgResponse>> {
  const doc = await api.getDocument(base, documentId);
  if (!doc.ok) return doc;

  const payload: FillPayload = {};

  if (target === 'file' || target === 'both') {
    const fallbackName = `${doc.value.company ?? 'job-finder'}-${doc.value.type}.pdf`.replace(/\s+/g, '_');
    const pdf = await api.getDocumentPdf(base, documentId, fallbackName);
    if (!pdf.ok) return pdf;
    payload.file = { name: pdf.value.name, mime: 'application/pdf', base64: pdf.value.base64 };
  }

  if (target === 'letter' || target === 'both') {
    const text = api.documentText(doc.value);
    if (text === null) return err('bad_request', 'This document has no plain text to paste.');
    payload.letter = text;
  }

  const res = (await chrome.tabs.sendMessage(tabId, { kind: 'cs/fill', payload })) as
    | Result<{ kind: 'cs/filled'; report: FillReport }>
    | undefined;
  if (!res) return err('unknown', 'The page did not respond. Reload the vacancy page and try again.');
  if (!res.ok) return res;
  return ok({ kind: 'bg/filled', report: res.value.report });
}

function summarize(docs: api.DocumentLike[]): DocumentSummary[] {
  return api.sortDocuments(docs.map(api.toDocumentSummary));
}
