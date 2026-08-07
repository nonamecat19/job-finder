import type { ComponentType } from 'react';
import type { Entry, EntryType } from '@job-finder/shared';
import { EducationEntryForm } from './EducationEntryForm';
import { ExperienceEntryForm } from './ExperienceEntryForm';
import { NormalEntryForm } from './NormalEntryForm';
import { PublicationEntryForm } from './PublicationEntryForm';
import { OneLineEntryForm } from './OneLineEntryForm';
import { BulletEntryForm } from './BulletEntryForm';
import { NumberedEntryForm } from './NumberedEntryForm';
import { ReversedNumberedEntryForm } from './ReversedNumberedEntryForm';
import { TextEntryForm } from './TextEntryForm';

export interface EntryFormProps {
  entry: Entry;
  onChange: (entry: Entry) => void;
}

const REGISTRY: Record<EntryType, ComponentType<EntryFormProps>> = {
  education: EducationEntryForm,
  experience: ExperienceEntryForm,
  normal: NormalEntryForm,
  publication: PublicationEntryForm,
  one_line: OneLineEntryForm,
  bullet: BulletEntryForm,
  numbered: NumberedEntryForm,
  reversed_numbered: ReversedNumberedEntryForm,
  text: TextEntryForm,
};

export const ENTRY_TYPE_LABELS: Record<EntryType, string> = {
  education: 'Education',
  experience: 'Experience',
  normal: 'Project / normal',
  publication: 'Publication',
  one_line: 'One-line (e.g. skills)',
  bullet: 'Bullet (e.g. honors)',
  numbered: 'Numbered (e.g. patents)',
  reversed_numbered: 'Reversed-numbered (e.g. talks)',
  text: 'Text',
};

export function getEntryForm(entryType: EntryType): ComponentType<EntryFormProps> | null {
  return REGISTRY[entryType] ?? null;
}

export function emptyEntryFor(_entryType: EntryType): Entry {
  return {};
}
