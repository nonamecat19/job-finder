import type { Entry } from '@job-finder/shared';
import { Field, Input, Textarea } from '../../../../components/ui';
import { makeEntrySetter } from './entryFormUtils';
import { HighlightsField } from './HighlightsField';

interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
}

export function EducationEntryForm({ entry, onChange }: EntryFormProps) {
  const set = makeEntrySetter(entry, onChange);
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
      <Field label="Institution">
        <Input value={entry.institution ?? ''} onChange={(e) => set('institution', e.target.value || undefined)} />
      </Field>
      <Field label="Degree">
        <Input value={entry.degree ?? ''} onChange={(e) => set('degree', e.target.value || undefined)} />
      </Field>
      <Field label="Area">
        <Input value={entry.area ?? ''} onChange={(e) => set('area', e.target.value || undefined)} />
      </Field>
      <Field label="Location">
        <Input value={entry.location ?? ''} onChange={(e) => set('location', e.target.value || undefined)} />
      </Field>
      <Field label="Start date">
        <Input
          value={entry.startDate ?? ''}
          onChange={(e) => set('startDate', e.target.value || undefined)}
          placeholder="2018-09"
        />
      </Field>
      <Field label="End date">
        <Input
          value={entry.endDate ?? ''}
          onChange={(e) => set('endDate', e.target.value || undefined)}
          placeholder="present"
        />
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
