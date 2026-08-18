import type { Capabilities, PageHints } from '@/shared/messages';

export type AdapterId = 'djinni' | 'dou' | 'workua';

export interface SiteAdapter {
  id: AdapterId;

  hosts: string[];

  hints(): PageHints;

  requiresLogin(): boolean;

  isFormOpen(): boolean;

  applyTrigger(): HTMLElement | null;
  findFileInput(): HTMLInputElement | null;
  findLetterField(): HTMLElement | null;
}

export function capabilitiesOf(adapter: SiteAdapter | null, host: string): Capabilities {
  if (!adapter) {
    return {
      host,
      adapter: null,
      formOpen: false,
      canOpenForm: false,
      hasFileInput: false,
      hasLetterField: false,
      requiresLogin: false,
      hints: {},
    };
  }
  return {
    host,
    adapter: adapter.id,
    formOpen: adapter.isFormOpen(),
    canOpenForm: adapter.applyTrigger() !== null,
    hasFileInput: adapter.findFileInput() !== null,
    hasLetterField: adapter.findLetterField() !== null,
    requiresLogin: adapter.requiresLogin(),
    hints: adapter.hints(),
  };
}
