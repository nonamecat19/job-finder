import type { Entry } from '@job-finder/shared';
import { Field, Textarea } from '../../../../components/ui';
import { makeEntrySetter } from './entryFormUtils';

interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
}

// "text" entries are plain markdown strings (e.g. an intro/welcome section).
export function TextEntryForm({ entry, onChange }: EntryFormProps) {
  const set = makeEntrySetter(entry, onChange);
  return (
    <Field label="Text">
      <Textarea value={entry.text ?? ''} onChange={(e) => set('text', e.target.value || undefined)} rows={2} />
    </Field>
  );
}
