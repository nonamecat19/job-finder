import type { Entry } from '@job-finder/shared';
import { Field, Input } from '../../../../components/ui';
import { makeEntrySetter } from './entryFormUtils';

interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
}

// "reversed_numbered" entries are single-line items (e.g. invited talks).
// Note: RenderCV's field is named "reversed_number" but its value is a talk
// title/description, not a number — a RenderCV naming quirk we preserve.
export function ReversedNumberedEntryForm({ entry, onChange }: EntryFormProps) {
  const set = makeEntrySetter(entry, onChange);
  return (
    <Field label="Description">
      <Input value={entry.reversedNumber ?? ''} onChange={(e) => set('reversedNumber', e.target.value || undefined)} />
    </Field>
  );
}
