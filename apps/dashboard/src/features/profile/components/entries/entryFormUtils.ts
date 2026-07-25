import type { Entry } from '@job-finder/shared';

// Shared helpers used by each per-entry-type form component. Kept tiny and
// dependency-free so every EntryXForm.tsx stays a simple controlled form.
export type EntryFieldSetter = <K extends keyof Entry>(key: K, value: Entry[K]) => void;

export function makeEntrySetter(entry: Entry, onChange: (entry: Entry) => void): EntryFieldSetter {
  return (key, value) => onChange({ ...entry, [key]: value });
}

export function listToText(items?: string[]): string {
  return (items ?? []).join(', ');
}

export function textToList(text: string): string[] | undefined {
  const items = text
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
  return items.length > 0 ? items : undefined;
}
