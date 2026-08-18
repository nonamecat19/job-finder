import type { Settings } from '@/shared/messages';

export const DEFAULT_SETTINGS: Settings = {
  apiBaseUrl: 'http://localhost:3000',
  debug: false,
};

export async function getSettings(): Promise<Settings> {
  const stored = (await chrome.storage.sync.get(DEFAULT_SETTINGS)) as Partial<Settings>;
  return {
    apiBaseUrl: normalizeBaseUrl(stored.apiBaseUrl ?? DEFAULT_SETTINGS.apiBaseUrl),
    debug: stored.debug ?? DEFAULT_SETTINGS.debug,
  };
}

export async function setSettings(settings: Settings): Promise<Settings> {
  const next: Settings = { apiBaseUrl: normalizeBaseUrl(settings.apiBaseUrl), debug: !!settings.debug };
  await chrome.storage.sync.set(next);
  return next;
}

export function normalizeBaseUrl(raw: string): string {
  return raw.trim().replace(/\/+$/, '');
}
