import type { Entry } from '@job-finder/shared';
import { Field, Input, Textarea } from '../../../../components/ui';
import { makeEntrySetter } from './entryFormUtils';
import { HighlightsField } from './HighlightsField';

interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
}

// "normal" entries are used for projects and similar name+url+dates items.
export function NormalEntryForm({ entry, onChange }: EntryFormProps) {
  const set = makeEntrySetter(entry, onChange);
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
      <Field label="Name">
        <Input value={entry.name ?? ''} onChange={(e) => set('name', e.target.value || undefined)} />
      </Field>
      <Field label="URL">
        <Input value={entry.url ?? ''} onChange={(e) => set('url', e.target.value || undefined)} />
      </Field>
      <Field label="Location">
        <Input value={entry.location ?? ''} onChange={(e) => set('location', e.target.value || undefined)} />
      </Field>
      <Field label="Date">
        <Input value={entry.date ?? ''} onChange={(e) => set('date', e.target.value || undefined)} placeholder="2023" />
      </Field>
      <Field label="Start date">
        <Input value={entry.startDate ?? ''} onChange={(e) => set('startDate', e.target.value || undefined)} />
      </Field>
      <Field label="End date">
        <Input value={entry.endDate ?? ''} onChange={(e) => set('endDate', e.target.value || undefined)} placeholder="present" />
      </Field>
      <Field label="Summary" className="sm:col-span-2">
        <Textarea value={entry.summary ?? ''} onChange={(e) => set('summary', e.target.value || undefined)} rows={2} />
      </Field>
      <HighlightsField
        highlights={entry.highlights}
        onChange={(highlights) => set('highlights', highlights)}
        className="sm:col-span-2"
      />
    </div>
  );
}
