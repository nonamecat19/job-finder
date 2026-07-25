import type { Entry } from '@job-finder/shared';
import { Field, Input } from '../../../../components/ui';

interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
}

// Raw key/value editor for entries whose section entryType didn't match any
// of the 9 known RenderCV shapes, or for fields an imported config carried
// that no typed form recognizes. Never drops the data (FR-009) — every key
// found under `unrecognized` stays editable as plain text.
export function UnrecognizedEntryFallbackForm({ entry, onChange }: EntryFormProps) {
  const data = entry.unrecognized ?? {};
  const keys = Object.keys(data);

  const setValue = (key: string, value: string) => {
    onChange({ ...entry, unrecognized: { ...data, [key]: value } });
  };

  if (keys.length === 0) {
    return <p className="text-sm text-faint">No data on this entry.</p>;
  }

  return (
    <div className="space-y-2">
      <p className="text-xs text-faint">
        This entry has fields the app doesn't recognize as a typed shape — nothing has been dropped, edit them here.
      </p>
      {keys.map((key) => (
        <Field key={key} label={key}>
          <Input
            value={typeof data[key] === 'string' ? (data[key] as string) : JSON.stringify(data[key])}
            onChange={(e) => setValue(key, e.target.value)}
          />
        </Field>
      ))}
    </div>
  );
}
