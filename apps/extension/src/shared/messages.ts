import type { Result } from './result';

export type Settings = {
  apiBaseUrl: string;
  debug: boolean;
};

export type DocumentSummary = {
  id: string;
  type: 'resume' | 'cover_letter';
  version: number;
  createdAt: string;
  company: string | null;
  title: string | null;

  hasText: boolean;
};

export type JobSummary = {
  id: string;
  title: string;
  company: string;
  url: string;
  status: string;
};

export type FillTarget = 'file' | 'letter' | 'both';

export type PageHints = {
  title?: string | null;
  company?: string | null;
};

export type Capabilities = {
  host: string;
  adapter: 'djinni' | 'dou' | 'workua' | null;
  formOpen: boolean;

  canOpenForm: boolean;
  hasFileInput: boolean;
  hasLetterField: boolean;

  requiresLogin: boolean;
  hints: PageHints;
};

export type FillReport = {
  fileAttached: boolean;
  letterFilled: boolean;
  warnings: string[];
};

export type AddOutcome = 'created' | 'duplicate' | 'needs_fill_in' | 'failed';

export type BgRequest =
  | { kind: 'bg/getSettings' }
  | { kind: 'bg/setSettings'; settings: Settings }
  | { kind: 'bg/ping'; apiBaseUrl?: string }
  | { kind: 'bg/resolveJob'; url: string; hints?: PageHints }
  | { kind: 'bg/listDocuments'; jobId: string }
  | { kind: 'bg/addVacancy'; url: string }
  | { kind: 'bg/fill'; tabId: number; documentId: string; target: FillTarget };

export type BgResponse =
  | { kind: 'bg/settings'; settings: Settings }
  | { kind: 'bg/pong'; ok: boolean }
  | { kind: 'bg/job'; job: JobSummary; documents: DocumentSummary[] }
  | { kind: 'bg/documents'; documents: DocumentSummary[] }
  | { kind: 'bg/addResult'; outcome: AddOutcome; job?: JobSummary; reason?: string }
  | { kind: 'bg/filled'; report: FillReport };

export type CsRequest =
  | { kind: 'cs/probe' }
  | { kind: 'cs/openApplyForm' }
  | { kind: 'cs/fill'; payload: FillPayload };

export type FillPayload = {
  file?: { name: string; mime: 'application/pdf'; base64: string };
  letter?: string;
};

export type CsResponse =
  | { kind: 'cs/capabilities'; caps: Capabilities }
  | { kind: 'cs/opened'; caps: Capabilities }
  | { kind: 'cs/filled'; report: FillReport };

export function sendToBackground(req: BgRequest): Promise<Result<BgResponse>> {
  return chrome.runtime.sendMessage(req) as Promise<Result<BgResponse>>;
}

export function sendToTab(tabId: number, req: CsRequest): Promise<Result<CsResponse>> {
  return chrome.tabs.sendMessage(tabId, req) as Promise<Result<CsResponse>>;
}
