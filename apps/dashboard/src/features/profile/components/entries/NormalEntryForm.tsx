import type { Entry } from '@job-finder/shared';
import { Field, Input, Select, Textarea } from '../../../../components/ui';
import { makeEntrySetter } from './entryFormUtils';
import { HighlightsField } from './HighlightsField';

interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
  sectionName?: string;
}

export function NormalEntryForm({ entry, onChange, sectionName }: EntryFormProps) {
  const set = makeEntrySetter(entry, onChange);
  const isProjects = sectionName === 'projects';
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
      {isProjects ? (
        <Field label="How many bullets to show on a generated resume">
          <Select
            value={entry.projectLevel ?? 'auto'}
            onChange={(e) => set('projectLevel', e.target.value === 'auto' ? undefined : e.target.value)}
            aria-label="Project density"
          >
            <option value="auto">Auto — scaled to this project&apos;s bullet count</option>
            <option value="relevant">Only the bullets the job asks about</option>
            <option value="top3">3 bullets</option>
            <option value="top5">5 bullets</option>
            <option value="all">All bullets</option>
          </Select>
        </Field>
      ) : null}
      <HighlightsField
        highlights={entry.highlights}
        onChange={(highlights) => set('highlights', highlights)}
        className="sm:col-span-2"
      />
    </div>
  );
}
