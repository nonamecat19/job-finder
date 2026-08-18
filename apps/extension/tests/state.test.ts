import { describe, expect, it } from 'vitest';

import type { Capabilities, DocumentSummary } from '@/shared/messages';
import {
  blockedReason,
  canAttachFile,
  canPasteLetter,
  describeFill,
  shouldOfferOpenForm,
  splitDocuments,
} from '@/popup/state';

const CAPS: Capabilities = {
  host: 'djinni.co',
  adapter: 'djinni',
  formOpen: true,
  canOpenForm: true,
  hasFileInput: true,
  hasLetterField: true,
  requiresLogin: false,
  hints: {},
};

const doc = (over: Partial<DocumentSummary>): DocumentSummary => ({
  id: 'd', type: 'resume', version: 1, createdAt: '2026-01-01T00:00:00Z',
  company: null, title: null, hasText: false, ...over,
});

describe('popup state', () => {
  it('splits documents by type', () => {
    const { resumes, letters } = splitDocuments([doc({ id: 'a' }), doc({ id: 'b', type: 'cover_letter' })]);
    expect(resumes.map((d) => d.id)).toEqual(['a']);
    expect(letters.map((d) => d.id)).toEqual(['b']);
  });

  it('enables attaching only with an open form, a file field and a document', () => {
    expect(canAttachFile(CAPS, [doc({})])).toBe(true);
    expect(canAttachFile(CAPS, [])).toBe(false);
    expect(canAttachFile({ ...CAPS, formOpen: false }, [doc({})])).toBe(false);
    expect(canAttachFile({ ...CAPS, hasFileInput: false }, [doc({})])).toBe(false);
  });

  it('enables pasting only when a letter actually carries text', () => {
    expect(canPasteLetter(CAPS, [doc({ type: 'cover_letter', hasText: true })])).toBe(true);
    expect(canPasteLetter(CAPS, [doc({ type: 'cover_letter', hasText: false })])).toBe(false);
  });

  it('offers to open the form only when one can be opened', () => {
    expect(shouldOfferOpenForm({ ...CAPS, formOpen: false })).toBe(true);
    expect(shouldOfferOpenForm(CAPS)).toBe(false);
    expect(shouldOfferOpenForm({ ...CAPS, formOpen: false, canOpenForm: false })).toBe(false);
  });

  it('names the missing capability instead of failing generically', () => {
    expect(blockedReason(CAPS, [doc({})], [])).toBeNull();
    expect(blockedReason({ ...CAPS, requiresLogin: true, formOpen: false }, [], [])).toMatch(/log in/i);
    expect(blockedReason({ ...CAPS, formOpen: false, canOpenForm: false }, [], [])).toMatch(/apply by email/i);
    expect(blockedReason({ ...CAPS, hasFileInput: false }, [doc({})], [])).toMatch(/no file upload/i);
    expect(blockedReason({ ...CAPS, adapter: null }, [], [])).toMatch(/not supported/i);
  });

  it('always tells the user they still have to press Send', () => {
    expect(describeFill({ fileAttached: true, letterFilled: true, warnings: [] })).toMatch(/press Send yourself/);
  });
});
