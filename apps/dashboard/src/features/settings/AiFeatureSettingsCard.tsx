import { useEffect, useState } from 'react';
import type { AiFeatureKey, AiFeatureSettingDto } from '@job-finder/shared';
import { ApiError } from '../../lib/api';
import { Button, Checkbox, EmptyState, ErrorState, Field, Input, LoadingRegion, SkeletonBlock, SkeletonLine } from '../../components/ui';
import { useAiFeatureSettings, useUpdateAiFeatureSetting } from './hooks';

const FEATURE_LABELS: Record<AiFeatureKey, { title: string; description: string }> = {
  resume: {
    title: 'Resume generation',
    description: 'Auto-enqueue a tailored resume for a job as soon as its fit score reaches the threshold below.',
  },
  cover_letter: {
    title: 'Cover letter generation',
    description:
      'Auto-enqueue a cover letter for a job as soon as its fit score reaches the threshold below.',
  },
  salary_infer: {
    title: 'Salary inference',
    description:
      'Auto-infer a salary range for a job as soon as its fit score reaches the threshold below.',
  },
};

function FeatureRow({ setting }: { setting: AiFeatureSettingDto }) {
  const update = useUpdateAiFeatureSetting();
  const [enabled, setEnabled] = useState(setting.enabled);
  const [threshold, setThreshold] = useState(setting.threshold);

  useEffect(() => {
    setEnabled(setting.enabled);
    setThreshold(setting.threshold);
  }, [setting.enabled, setting.threshold]);

  const labels = FEATURE_LABELS[setting.feature as AiFeatureKey] ?? {
    title: setting.feature,
    description: '',
  };
  const dirty = setting.enabled !== enabled || setting.threshold !== threshold;

  return (
    <div className="border-b border-border py-4 last:border-0 last:pb-0 first:pt-0">
      <p className="mb-1 text-sm font-semibold text-foreground">{labels.title}</p>
      <p className="mb-3 text-sm text-muted">
        {labels.description} Below the threshold, or when disabled, it only runs on-demand.
      </p>
      <div className="flex flex-wrap items-end gap-4">
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Checkbox checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          Run automatically on high score
        </label>
        <Field label="Minimum match score (0-100)">
          <Input
            type="number"
            min={0}
            max={100}
            value={threshold}
            disabled={!enabled}
            onChange={(e) => setThreshold(Number(e.target.value))}
            className="w-24"
          />
        </Field>
        <Button
          onClick={() => update.mutate({ feature: setting.feature, enabled, threshold })}
          disabled={!dirty || update.isPending}
        >
          save
        </Button>
      </div>
      {update.error && update.variables?.feature === setting.feature ? (
        <div className="mt-3">
          <ErrorState error={update.error} />
        </div>
      ) : null}
    </div>
  );
}

export default function AiFeatureSettingsCard() {
  const settings = useAiFeatureSettings();

  if (settings.isPending) {
    return (
      <LoadingRegion label="Loading AI feature settings…" className="space-y-2">
        <SkeletonLine width="w-1/3" className="h-5" />
        <SkeletonBlock className="h-10 w-full" />
        <SkeletonBlock className="h-10 w-full" />
      </LoadingRegion>
    );
  }
  if (settings.error) {
    if (settings.error instanceof ApiError && settings.error.status === 404) {
      return (
        <EmptyState>AI feature settings aren&apos;t available on this server yet.</EmptyState>
      );
    }
    return (
      <ErrorState error={settings.error} />
    );
  }

  return (
    <>
      {settings.data?.map((s) => <FeatureRow key={s.feature} setting={s} />)}
    </>
  );
}
