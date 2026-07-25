import type { Entry } from '@job-finder/shared';
import { Field, Input, Textarea } from '../../../../components/ui';
import { makeEntrySetter, listToText, textToList } from './entryFormUtils';

interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
}

export function PublicationEntryForm({ entry, onChange }: EntryFormProps) {
  const set = makeEntrySetter(entry, onChange);
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
      <Field label="Title" className="sm:col-span-2">
        <Input value={entry.title ?? ''} onChange={(e) => set('title', e.target.value || undefined)} />
      </Field>
      <Field label="Authors (comma-separated)" className="sm:col-span-2">
        <Input value={listToText(entry.authors)} onChange={(e) => set('authors', textToList(e.target.value))} />
      </Field>
      <Field label="Journal">
        <Input value={entry.journal ?? ''} onChange={(e) => set('journal', e.target.value || undefined)} />
      </Field>
      <Field label="Date">
        <Input value={entry.date ?? ''} onChange={(e) => set('date', e.target.value || undefined)} placeholder="2023-07" />
      </Field>
      <Field label="DOI">
        <Input value={entry.doi ?? ''} onChange={(e) => set('doi', e.target.value || undefined)} />
      </Field>
      <Field label="URL">
        <Input value={entry.url ?? ''} onChange={(e) => set('url', e.target.value || undefined)} />
      </Field>
      <Field label="Summary" className="sm:col-span-2">
        <Textarea value={entry.summary ?? ''} onChange={(e) => set('summary', e.target.value || undefined)} rows={2} />
      </Field>
    </div>
  );
}
