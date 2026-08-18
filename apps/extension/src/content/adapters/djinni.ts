import type { PageHints } from '@/shared/messages';

import { isVisible, queryFirst, textOf } from '../dom';
import { findByText, guessApplyForm, guessFileInput, guessLetterField } from './generic';
import type { SiteAdapter } from './types';

const FORM = ['#apply-form', 'form[action*="apply"]', 'form#job-apply', 'form[id*="apply"]'];
const FILE = [
  'input[type="file"][name*="file"]',
  'input[type="file"][accept*="pdf"]',
  '#apply-form input[type="file"]',
];
const LETTER = [
  '#apply-form textarea',
  'textarea[name="message"]',
  'textarea[name*="cover"]',
  'textarea#id_message',
];
const TRIGGER_TEXT = ['відгукнутися', 'відгукнутись', 'apply', 'откликнуться'];

export const djinni: SiteAdapter = {
  id: 'djinni',
  hosts: ['djinni.co'],

  hints(): PageHints {
    return {
      title: textOf(document, ['h1', '.detail--title', '[data-testid="vacancy-title"]']),
      company: textOf(document, ['.job-details--title', 'a[href*="/jobs/?company="]', '.company-name']),
    };
  },

  requiresLogin(): boolean {
    if (this.isFormOpen()) return false;
    return findByText(['log in', 'увійти', 'sign in'], ['a[href*="login"]', 'a[href*="signup"]']) !== null;
  },

  isFormOpen(): boolean {
    const form = queryFirst<HTMLElement>(document, FORM);
    if (form && isVisible(form)) return true;
    return this.findLetterField() !== null;
  },

  applyTrigger(): HTMLElement | null {
    return (
      queryFirst<HTMLElement>(document, ['[data-toggle="apply"]', 'a[href="#apply-form"]', 'button[data-target*="apply"]']) ??
      findByText(TRIGGER_TEXT)
    );
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
