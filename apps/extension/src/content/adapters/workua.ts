import type { PageHints } from '@/shared/messages';

import { isVisible, queryFirst, textOf } from '../dom';
import { findByText, guessApplyForm, guessFileInput, guessLetterField } from './generic';
import type { SiteAdapter } from './types';

/**
 * work.ua.
 *
 * Applying usually means picking a resume already hosted on work.ua, and such
 * vacancies expose no file input at all — the capability flags report that
 * honestly so the popup can offer only the cover letter.
 */
const FORM = ['#apply-form', 'form[action*="apply"]', 'form#form-apply', 'form[id*="apply"]'];
const FILE = ['input[type="file"][accept*="pdf"]', '#apply-form input[type="file"]', 'input[type="file"]'];
const LETTER = [
  '#apply-form textarea',
  'textarea[name*="letter"]',
  'textarea[name*="message"]',
  'textarea#add_text',
  'textarea',
];
const TRIGGER_TEXT = ['відгукнутися', 'відгукнутись', 'подати заявку', 'apply'];

export const workua: SiteAdapter = {
  id: 'workua',
  hosts: ['work.ua'],

  hints(): PageHints {
    return {
      title: textOf(document, ['h1#h1-name', 'h1']),
      company: textOf(document, ['.dropdown a[href*="/jobs/by-company"]', 'a[href*="/company"]', '.company-name']),
    };
  },

  requiresLogin(): boolean {
    if (this.isFormOpen()) return false;
    return findByText(['увійти', 'вхід', 'log in'], ['a[href*="login"]', 'a[href*="signin"]']) !== null;
  },

  isFormOpen(): boolean {
    const form = queryFirst<HTMLElement>(document, FORM);
    if (form && isVisible(form)) return true;
    return this.findLetterField() !== null;
  },

  applyTrigger(): HTMLElement | null {
    return queryFirst<HTMLElement>(document, ['a[href*="apply"]', 'button[data-target*="apply"]']) ?? findByText(TRIGGER_TEXT);
  },

  findFileInput(): HTMLInputElement | null {
    const scope = queryFirst<HTMLElement>(document, FORM) ?? guessApplyForm();
    return queryFirst<HTMLInputElement>(document, FILE) ?? guessFileInput(scope);
  },

  findLetterField(): HTMLElement | null {
    const direct = queryFirst<HTMLElement>(document, LETTER);
    if (direct && isVisible(direct)) return direct;
    return guessLetterField(queryFirst<HTMLElement>(document, FORM) ?? guessApplyForm());
  },
};
