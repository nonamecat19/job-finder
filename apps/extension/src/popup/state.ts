import type { AppError } from '@/shared/result';
import type { Capabilities, DocumentSummary, FillReport, JobSummary } from '@/shared/messages';

export type PopupState =
  | { phase: 'loading' }
  | { phase: 'unsupported'; host: string }
  | { phase: 'not_found'; url: string }
  | { phase: 'error'; error: AppError }
  | {
      phase: 'ready';
      job: JobSummary;
      resumes: DocumentSummary[];
      letters: DocumentSummary[];
      caps: Capabilities;
      report?: FillReport;
      notice?: string;
    };

export function splitDocuments(docs: DocumentSummary[]): { resumes: DocumentSummary[]; letters: DocumentSummary[] } {
  return {
    resumes: docs.filter((d) => d.type === 'resume'),
    letters: docs.filter((d) => d.type === 'cover_letter'),
  };
}

export function canAttachFile(caps: Capabilities, resumes: DocumentSummary[]): boolean {
  return caps.formOpen && caps.hasFileInput && resumes.length > 0;
}

export function canPasteLetter(caps: Capabilities, letters: DocumentSummary[]): boolean {
  return caps.formOpen && caps.hasLetterField && letters.some((d) => d.hasText);
}

export function shouldOfferOpenForm(caps: Capabilities): boolean {
  return !caps.formOpen && caps.canOpenForm;
}

export function blockedReason(caps: Capabilities, resumes: DocumentSummary[], letters: DocumentSummary[]): string | null {
  if (caps.adapter === null) return 'This site is not supported yet.';
  if (caps.requiresLogin) return 'Log in to this job board first — the apply form is behind a login.';
  if (!caps.formOpen && !caps.canOpenForm) {
    return 'No apply form found on this page. This vacancy may apply by email instead.';
  }
  if (!caps.formOpen) return null;
  if (!caps.hasFileInput && !caps.hasLetterField) return 'This apply form has neither a file field nor a letter field.';
  if (!caps.hasFileInput && resumes.length > 0) return 'This apply form takes no file upload — only the cover letter can be filled.';
  if (!caps.hasLetterField && letters.length > 0) return 'This apply form has no cover-letter field.';
  return null;
}

export function describeFill(report: FillReport): string {
  const done = [report.fileAttached ? 'CV attached' : null, report.letterFilled ? 'cover letter pasted' : null]
    .filter(Boolean)
    .join(' · ');
  return `${done} — review and press Send yourself.`;
}

export function formatDocLabel(doc: DocumentSummary): string {
  const date = new Date(doc.createdAt);
  const when = Number.isNaN(date.getTime()) ? doc.createdAt : date.toLocaleDateString();
  return `v${doc.version} · ${when}`;
}
