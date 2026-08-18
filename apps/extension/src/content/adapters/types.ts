import type { Capabilities, PageHints } from '@/shared/messages';

export type AdapterId = 'djinni' | 'dou' | 'workua';

export interface SiteAdapter {
  id: AdapterId;
  /** Hosts this adapter claims, already stripped of "www.". */
  hosts: string[];
  /** Title/company read off the page, used only to narrow a fallback search. */
  hints(): PageHints;
  /** True when a login wall stands where the apply form should be. */
  requiresLogin(): boolean;
  /** True when the apply form is on screen and fillable right now. */
  isFormOpen(): boolean;
  /** The button that reveals the apply form, if the page has one. */
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
