import { useEffect, useState } from 'react';
import type { ResumeShapeConfigDto } from '@job-finder/shared';
import { ApiError } from '../../lib/api';
import {
  Button,
  Checkbox,
  EmptyState,
  ErrorState,
  Field,
  LoadingRegion,
  RangeSlider,
  SkeletonBlock,
  SkeletonLine,
  Stepper,
} from '../../components/ui';
import { useResetResumeShape, useResumeShape, useUpdateResumeShape } from './hooks';

type NumericKey = Exclude<
  keyof ResumeShapeConfigDto & string,
  | 'summaryEnabled'
  | 'skillsEnabled'
  | 'experienceEnabled'
  | 'projectsEnabled'
  | 'certificationsEnabled'
  | 'educationEnabled'
>;

const STEPPER_FIELDS: { key: NumericKey; label: string; min: number; max: number; description: string }[] = [
  {
    key: 'summaryLines',
    label: 'Summary sentences',
    min: 1,
    max: 12,
    description: 'Approximate length of the professional summary.',
  },
  {
    key: 'skillsMaxGroups',
    label: 'Max skill groups',
    min: 0,
    max: 20,
    description: 'How many skill groups to keep. 0 keeps all of them.',
  },
  {
    key: 'targetPages',
    label: 'Target pages',
    min: 1,
    max: 3,
    description: 'Page count the render loop aims for. It overrides the section lengths above when they conflict.',
  },
  {
    key: 'projectBulletsMax',
    label: 'Max bullets per project',
    min: 0,
    max: 10,
    description: 'Hard cap per project. 0 keeps every bullet.',
  },
  {
    key: 'fontSize',
    label: 'Body font size (pt)',
    min: 8,
    max: 14,
    description: 'Body text size. Name scales to 3x, headline and connections match body.',
  },
];

const RANGE_FIELDS: {
  minKey: NumericKey;
  maxKey: NumericKey;
  label: string;
  min: number;
  max: number;
  description: string;
}[] = [
  {
    minKey: 'experienceBulletsMin',
    maxKey: 'experienceBulletsMax',
    label: 'Bullets per job',
    min: 1,
    max: 20,
    description:
      'Target range per experience entry. A job with fewer available bullets keeps what it has — nothing is invented; extras beyond the cap are dropped.',
  },
  {
    minKey: 'projectsMin',
    maxKey: 'projectsMax',
    label: 'Projects',
    min: 0,
    max: 10,
    description:
      'Target range for the projects section. 0 on either end means no floor / no cap; requires projects to be enabled.',
  },
  {
    minKey: 'certificationsMin',
    maxKey: 'certificationsMax',
    label: 'Certifications',
    min: 0,
    max: 10,
    description:
      'Target range for the certifications section, kept in the order they appear in your master resume. 0 on either end means no floor / no cap.',
  },
];

const ALL_NUMERIC_KEYS: NumericKey[] = [
  ...STEPPER_FIELDS.map((f) => f.key),
  ...RANGE_FIELDS.flatMap((f) => [f.minKey, f.maxKey]),
];

function ResumeShapeForm({ config }: { config: ResumeShapeConfigDto }) {
  const update = useUpdateResumeShape();
  const reset = useResetResumeShape();
  const [draft, setDraft] = useState<ResumeShapeConfigDto>(config);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDraft(config);
  }, [config]);

  const dirty = ALL_NUMERIC_KEYS.some((key) => draft[key] !== config[key])
    || draft.summaryEnabled !== config.summaryEnabled
    || draft.skillsEnabled !== config.skillsEnabled
    || draft.experienceEnabled !== config.experienceEnabled
    || draft.projectsEnabled !== config.projectsEnabled
    || draft.certificationsEnabled !== config.certificationsEnabled
    || draft.educationEnabled !== config.educationEnabled;

  const pending = update.isPending || reset.isPending;

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted">
        Controls the shape of every resume generated after you save. A generation already running finishes with the
        settings it started with.
      </p>

      <div className="flex flex-wrap gap-4">
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Checkbox
            checked={draft.summaryEnabled}
            onChange={(e) => setDraft({ ...draft, summaryEnabled: e.target.checked })}
          />
          Include summary section
        </label>
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Checkbox
            checked={draft.experienceEnabled}
            onChange={(e) => setDraft({ ...draft, experienceEnabled: e.target.checked })}
          />
          Include experience section
        </label>
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Checkbox
            checked={draft.skillsEnabled}
            onChange={(e) => setDraft({ ...draft, skillsEnabled: e.target.checked })}
          />
          Include skills section
        </label>
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Checkbox
            checked={draft.projectsEnabled}
            onChange={(e) => setDraft({ ...draft, projectsEnabled: e.target.checked })}
          />
          Include projects section
        </label>
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Checkbox
            checked={draft.certificationsEnabled}
            onChange={(e) => setDraft({ ...draft, certificationsEnabled: e.target.checked })}
          />
          Include certifications section
        </label>
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Checkbox
            checked={draft.educationEnabled}
            onChange={(e) => setDraft({ ...draft, educationEnabled: e.target.checked })}
          />
          Include education section
        </label>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        {STEPPER_FIELDS.map((f) => (
          <div key={f.key}>
            <Field label={`${f.label} (${f.min}-${f.max})`}>
              <Stepper
                min={f.min}
                max={f.max}
                value={draft[f.key]}
                aria-label={f.label}
                onChange={(value) => setDraft({ ...draft, [f.key]: value })}
              />
            </Field>
            <p className="mt-1 text-xs text-muted">{f.description}</p>
          </div>
        ))}
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        {RANGE_FIELDS.map((f) => (
          <div key={f.minKey}>
            <Field label={`${f.label} (${f.min}-${f.max})`}>
              <RangeSlider
                min={f.min}
                max={f.max}
                valueMin={draft[f.minKey]}
                valueMax={draft[f.maxKey]}
                labelMin={`Min ${f.label}`}
                labelMax={`Max ${f.label}`}
                onChangeMin={(value) => setDraft({ ...draft, [f.minKey]: value })}
                onChangeMax={(value) => setDraft({ ...draft, [f.maxKey]: value })}
              />
            </Field>
            <p className="mt-1 text-xs text-muted">{f.description}</p>
          </div>
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Button onClick={() => update.mutate(draft)} disabled={!dirty || pending}>
          save
        </Button>
        <Button variant="secondary" onClick={() => reset.mutate()} disabled={pending}>
          reset to defaults
        </Button>
      </div>

      {update.error ? <ErrorState error={update.error} /> : null}
      {reset.error ? <ErrorState error={reset.error} /> : null}
    </div>
  );
}

export default function ResumeShapeCard() {
  const shape = useResumeShape();

  if (shape.isPending) {
    return (
      <LoadingRegion label="Loading resume shape settings…" className="space-y-2">
        <SkeletonLine width="w-1/3" className="h-5" />
        <SkeletonBlock className="h-10 w-full" />
        <SkeletonBlock className="h-10 w-full" />
      </LoadingRegion>
    );
  }
  if (shape.error) {
    if (shape.error instanceof ApiError && shape.error.status === 404) {
      return <EmptyState>Resume shape settings aren&apos;t available on this server yet.</EmptyState>;
    }
    return <ErrorState error={shape.error} />;
  }
  if (!shape.data) {
    return <EmptyState>No resume shape settings found.</EmptyState>;
  }

  return <ResumeShapeForm config={shape.data} />;
}
