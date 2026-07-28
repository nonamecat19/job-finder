import type { LlmTaskSettingDto } from '@job-finder/shared';
import { ApiError } from '../../lib/api';
import { Button, EmptyState, ErrorState, LoadingRegion, Select, SkeletonBlock, SkeletonLine } from '../../components/ui';
import { useLlmModels, useLlmSettings, useUpdateLlmSettings } from './hooks';

const TASK_LABELS: Record<string, string> = {
  match: 'Job matching / scoring',
  generation: 'Resume & cover letter generation',
  rephrase: 'Keyword rephrase suggestions',
  ghost: 'Ghost-job detection',
  default: 'Other (recruiter, outreach, salary)',
};

type Provider = 'ollama' | 'cerebras';

function taskLabel(taskKey: string): string {
  return TASK_LABELS[taskKey] ?? taskKey;
}

export default function LlmSettingsCard() {
  const settings = useLlmSettings();
  const models = useLlmModels();
  const update = useUpdateLlmSettings();

  if (settings.isPending) {
    return (
      <LoadingRegion label="Loading AI model settings…" className="space-y-2">
        <SkeletonLine width="w-1/3" className="h-5" />
        <SkeletonBlock className="h-10 w-full" />
        <SkeletonBlock className="h-10 w-full" />
      </LoadingRegion>
    );
  }
  if (settings.error) {
    if (settings.error instanceof ApiError && settings.error.status === 404) {
      return (
        <EmptyState>AI model settings aren&apos;t available on this server yet.</EmptyState>
      );
    }
    return (
      <ErrorState error={settings.error} />
    );
  }

  const { credentialConfigured, tasks } = settings.data;
  const cerebrasModels = models.data?.cerebras ?? [];
  const anyCerebrasSelected = tasks.some((t) => t.provider === 'cerebras');

  function modelsFor(provider: string) {
    if (provider === 'cerebras') return cerebrasModels;
    return [];
  }

  function switchAll(provider: Provider) {
    const next: LlmTaskSettingDto[] = tasks.map((t) => ({ taskKey: t.taskKey, provider, model: '' }));
    update.mutate(next);
  }

  function setTask(taskKey: string, patch: Partial<LlmTaskSettingDto>) {
    const current = tasks.find((t) => t.taskKey === taskKey);
    if (!current) return;
    update.mutate([{ taskKey, provider: current.provider, model: current.model, ...patch }]);
  }

  return (
    <>
      <div className="flex items-center justify-between gap-4">
        <p className="text-sm text-muted">
          Choose which chat tasks run on the local Ollama model versus a Cerebras
          free-tier model. Embeddings always stay on Ollama.
        </p>
        <div className="flex shrink-0 gap-2">
          <Button variant="secondary" onClick={() => switchAll('ollama')} disabled={update.isPending}>
            Switch all to Ollama
          </Button>
          <Button variant="secondary" onClick={() => switchAll('cerebras')} disabled={update.isPending}>
            Switch all to Cerebras
          </Button>
        </div>
      </div>

      {!credentialConfigured && anyCerebrasSelected ? (
        <div
          data-testid="cerebras-credential-banner"
          className="mt-4 rounded-lg border border-warning/30 bg-warning-soft px-3 py-2 text-sm text-warning"
        >
          Cerebras credential not configured (CEREBRAS_API_KEY). Tasks set to Cerebras will keep
          running on Ollama until a key is added.
        </div>
      ) : null}

      {update.error ? (
        <div className="mt-4">
          <ErrorState error={update.error} />
        </div>
      ) : null}

      <div className="mt-5 divide-y divide-border">
        {tasks.map((task) => (
          <div key={task.taskKey} className="flex items-center gap-4 py-3">
            <span className="flex-1 text-sm font-medium text-foreground">{taskLabel(task.taskKey)}</span>
            <Select
              aria-label={`${taskLabel(task.taskKey)} provider`}
              className="w-40"
              value={task.provider}
              disabled={update.isPending}
              onChange={(e) => setTask(task.taskKey, { provider: e.target.value, model: '' })}
            >
              <option value="ollama">Ollama</option>
              <option value="cerebras">Cerebras</option>
            </Select>
            <Select
              aria-label={`${taskLabel(task.taskKey)} model`}
              className="w-64"
              value={task.model}
              disabled={update.isPending || task.provider === 'ollama'}
              onChange={(e) => setTask(task.taskKey, { model: e.target.value })}
            >
              <option value="">Default</option>
              {modelsFor(task.provider).map((m) => (
                <option key={m.id} value={m.id}>
                  {m.label}
                </option>
              ))}
            </Select>
          </div>
        ))}
      </div>
    </>
  );
}
