import type { Entry } from '@job-finder/shared';
import { Field, Input, Select, TagInput } from '../../../../components/ui';
import { listToText, makeEntrySetter, textToList } from './entryFormUtils';

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
      {isSkills ? (
        <Field label="Details">
          <TagInput
            values={textToList(entry.details ?? '') ?? []}
            onChange={(values) => set('details', values.length > 0 ? listToText(values) : undefined)}
            placeholder="Python, Go, Rust"
            aria-label="Skill details"
          />
        </Field>
      ) : (
        <Field label="Details">
          <Input
            value={entry.details ?? ''}
            onChange={(e) => set('details', e.target.value || undefined)}
            placeholder="Python, Go, Rust"
          />
        </Field>
      )}
      {isSkills ? (
        <Field label="How many to show on a generated resume">
          <Select
            value={entry.skillLevel ?? 'auto'}
            onChange={(e) => set('skillLevel', e.target.value === 'auto' ? undefined : e.target.value)}
            aria-label="Skill density"
          >
            <option value="auto">Auto — scaled to this group&apos;s size</option>
            <option value="relevant">Only the skills the job asks for</option>
            <option value="top5">5 by relevance</option>
            <option value="top10">~10 by relevance</option>
            <option value="top15">~15 by relevance</option>
            <option value="top20">~20 by relevance</option>
            <option value="all">All skills</option>
          </Select>
        </Field>
      ) : null}
    </div>
  );
}
