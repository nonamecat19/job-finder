import type { Entry } from '@job-finder/shared';
import { Field, Input, Select } from '../../../../components/ui';
import { makeEntrySetter } from './entryFormUtils';

interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
  sectionName?: string;
}

export function OneLineEntryForm({ entry, onChange, sectionName }: EntryFormProps) {
  const set = makeEntrySetter(entry, onChange);
  const isSkills = sectionName === 'skills';
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
      <Field label="Label">
        <Input value={entry.label ?? ''} onChange={(e) => set('label', e.target.value || undefined)} placeholder="Languages" />
      </Field>
      <Field label="Details">
        <Input
          value={entry.details ?? ''}
          onChange={(e) => set('details', e.target.value || undefined)}
          placeholder="Python, Go, Rust"
        />
      </Field>
      {isSkills ? (
        <Field label="How many to show on a generated resume">
          <Select
            value={entry.skillLevel ?? 'all'}
            onChange={(e) => set('skillLevel', e.target.value === 'all' ? undefined : e.target.value)}
            aria-label="Skill density"
          >
            <option value="all">All skills</option>
            <option value="medium">Half — most relevant first</option>
            <option value="relevant">Only the skills the job asks for</option>
          </Select>
        </Field>
      ) : null}
    </div>
  );
}
