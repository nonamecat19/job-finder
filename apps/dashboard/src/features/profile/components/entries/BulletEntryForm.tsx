import type { Entry } from '@job-finder/shared';
import { Field, Input } from '../../../../components/ui';
import { makeEntrySetter } from './entryFormUtils';

interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
}

// "bullet" entries are single-line items (e.g. honors, awards).
export function BulletEntryForm({ entry, onChange }: EntryFormProps) {
  const set = makeEntrySetter(entry, onChange);
  return (
    <Field label="Bullet">
      <Input value={entry.bullet ?? ''} onChange={(e) => set('bullet', e.target.value || undefined)} />
    </Field>
  );
}
