import { useEffect, useState } from 'react';
import { Button, Checkbox, ErrorState, Field, Input, Spinner, Surface } from '../../components/ui';
import { useAutoGenerateSettings, useUpdateAutoGenerateSettings } from './hooks';

export default function AutoGenerateSettingsCard() {
  const settings = useAutoGenerateSettings();
  const update = useUpdateAutoGenerateSettings();

  const [enabled, setEnabled] = useState(false);
  const [threshold, setThreshold] = useState(90);

  useEffect(() => {
    if (settings.data) {
      setEnabled(settings.data.enabled);
      setThreshold(settings.data.threshold);
    }
  }, [settings.data]);

  if (settings.isPending) {
    return (
      <Surface>
        <Spinner label="Loading auto-generate settings…" />
      </Surface>
    );
  }
  if (settings.error) {
    return (
      <Surface>
        <ErrorState error={settings.error} />
      </Surface>
    );
  }

  const dirty = settings.data && (settings.data.enabled !== enabled || settings.data.threshold !== threshold);

  return (
    <Surface>
      <p className="mb-3 text-sm text-muted">
        Automatically enqueue a resume for a job as soon as its fit score reaches the threshold
        below.
      </p>
      <div className="flex flex-wrap items-end gap-4">
        <label className="flex items-center gap-2 text-sm text-fg">
          <Checkbox checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          Auto-generate resume on high score
        </label>
        <Field label="Threshold (0-100)">
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
          onClick={() => update.mutate({ enabled, threshold })}
          disabled={!dirty || update.isPending}
        >
          save
        </Button>
      </div>
      {update.error ? (
        <div className="mt-3">
          <ErrorState error={update.error} />
        </div>
      ) : null}
    </Surface>
  );
}
