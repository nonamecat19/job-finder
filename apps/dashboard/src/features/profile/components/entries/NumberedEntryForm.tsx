import type { Entry } from '@job-finder/shared';
import { Field, Input } from '../../../../components/ui';
import { makeEntrySetter } from './entryFormUtils';

interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
}

// "numbered" entries are single-line items (e.g. patents).
export function NumberedEntryForm({ entry, onChange }: EntryFormProps) {
  const set = makeEntrySetter(entry, onChange);
  return (
    <Field label="Number">
      <Input value={entry.number ?? ''} onChange={(e) => set('number', e.target.value || undefined)} />
    </Field>
  );
}
