import { useEffect, useState } from 'react';
import type { ResumeShapeConfigDto } from '@job-finder/shared';
import { ApiError } from '../../lib/api';
import {
  Button,
  Checkbox,
  EmptyState,
  ErrorState,
  Field,
  Input,
  LoadingRegion,
  SkeletonBlock,
  SkeletonLine,
} from '../../components/ui';
import { useResetResumeShape, useResumeShape, useUpdateResumeShape } from './hooks';

type NumericKey = Exclude<
  keyof ResumeShapeConfigDto & string,
  'skillsEnabled' | 'projectsEnabled' | 'certificationsEnabled'
>;

// One row per configurable value, each carrying its allowed range and what it
// does — this card is the single place the whole resume shape is visible.
const NUMERIC_FIELDS: { key: NumericKey; label: string; min: number; max: number; description: string }[] = [
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
    key: 'experienceBulletsMin',
    label: 'Min bullets per job',
    min: 1,
    max: 20,
    description: 'Target floor per experience entry. A job with fewer available bullets keeps what it has — nothing is invented.',
  },
  {
    key: 'experienceBulletsMax',
    label: 'Max bullets per job',
    min: 1,
    max: 20,
    description: 'Hard cap per experience entry; extra bullets are dropped.',
  },
  {
    key: 'targetPages',
    label: 'Target pages',
    min: 1,
    max: 3,
    description: 'Page count the render loop aims for. It overrides the section lengths above when they conflict.',
  },
  {
    key: 'projectsMin',
    label: 'Min projects',
    min: 0,
    max: 20,
    description: 'Target floor for the projects section. 0 means no minimum; requires projects to be enabled.',
  },
  {
    key: 'projectsMax',
    label: 'Max projects',
    min: 0,
    max: 20,
    description: 'Hard cap on projects, kept in the order they appear in your master resume. 0 includes all of them.',
  },
  {
    key: 'projectBulletsMax',
    label: 'Max bullets per project',
    min: 0,
    max: 10,
    description: 'Hard cap per project. 0 keeps every bullet.',
  },
  {
    key: 'certificationsMin',
    label: 'Min certifications',
    min: 0,
    max: 20,
    description: 'Target floor for the certifications section. 0 means no minimum; requires certifications to be enabled.',
  },
  {
    key: 'certificationsMax',
    label: 'Max certifications',
    min: 0,
    max: 20,
    description: 'Hard cap on certifications, kept in the order they appear in your master resume. 0 includes all of them.',
  },
];

function ResumeShapeForm({ config }: { config: ResumeShapeConfigDto }) {
  const update = useUpdateResumeShape();
  const reset = useResetResumeShape();
  const [draft, setDraft] = useState<ResumeShapeConfigDto>(config);

  useEffect(() => {
    // Re-syncs the editable draft when the server-confirmed config changes
    // (after a save or a reset) — reacting to a query resolving, not state
    // derivable from props during render.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDraft(config);
  }, [config]);

  const dirty = NUMERIC_FIELDS.some((f) => draft[f.key] !== config[f.key])
    || draft.skillsEnabled !== config.skillsEnabled
    || draft.projectsEnabled !== config.projectsEnabled
    || draft.certificationsEnabled !== config.certificationsEnabled;

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
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        {NUMERIC_FIELDS.map((f) => (
          <div key={f.key}>
            <Field label={`${f.label} (${f.min}-${f.max})`}>
              <Input
                type="number"
                min={f.min}
                max={f.max}
                value={draft[f.key]}
                aria-label={f.label}
                onChange={(e) => setDraft({ ...draft, [f.key]: Number(e.target.value) })}
                className="w-24"
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
