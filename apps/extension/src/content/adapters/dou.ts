import type { PageHints } from '@/shared/messages';

import { isVisible, queryFirst, textOf } from '../dom';
import { findByText, guessApplyForm, guessFileInput, guessLetterField } from './generic';
import type { SiteAdapter } from './types';

/**
 * jobs.dou.ua.
 *
 * Some vacancies apply through an on-site form, others only publish a contact
 * email. When there is no form and no trigger the popup says so rather than
 * failing generically — that is why applyTrigger() is allowed to return null.
 */
const FORM = ['#reply-form', 'form.reply', 'form[action*="vacancy"]', 'form[id*="reply"]'];
const FILE = ['#reply-form input[type="file"]', 'input[type="file"][name*="cv"]', 'input[type="file"]'];
const LETTER = ['#reply-form textarea', 'textarea[name="text"]', 'textarea[name*="comment"]', 'textarea'];
const TRIGGER_TEXT = ['відгукнутись', 'відгукнутися', 'надіслати резюме', 'apply', 'відповісти'];

export const dou: SiteAdapter = {
  id: 'dou',
  hosts: ['dou.ua'],

  hints(): PageHints {
    return {
      title: textOf(document, ['h1.g-h2', 'h1', '.b-vacancy h1']),
      company: textOf(document, ['.l-n a', '.b-compinfo .info a', 'a[href*="/companies/"]']),
    };
  },

  requiresLogin(): boolean {
    if (this.isFormOpen()) return false;
    return findByText(['увійти', 'log in', 'вхід'], ['a[href*="login"]', 'a[href*="auth"]']) !== null;
  },

  isFormOpen(): boolean {
    const form = queryFirst<HTMLElement>(document, FORM);
    if (form && isVisible(form)) return true;
    return this.findLetterField() !== null;
  },

  applyTrigger(): HTMLElement | null {
    return (
      queryFirst<HTMLElement>(document, ['.reply-link', 'a[href="#reply"]', 'a[href*="reply"]']) ??
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
