import type { Entry } from '@job-finder/shared';
import { Field, Input } from '../../../../components/ui';
import { makeEntrySetter } from './entryFormUtils';

interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
}

// "one_line" entries are used for skill groups (label: details).
export function OneLineEntryForm({ entry, onChange }: EntryFormProps) {
  const set = makeEntrySetter(entry, onChange);
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
    </div>
  );
}
