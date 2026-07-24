import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { RotateCw, X } from 'lucide-react';
import type { ActivityOp, ActivityRunDto } from '@job-finder/shared';
import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import { Button, Chip, EmptyState, ErrorState, Spinner, Surface } from '../../components/ui';
import { useActivity, useCancelActivity, useCancelAllActivity, useRetryActivity } from './hooks';

const OP_LABELS: Record<ActivityOp, string> = {
  ingest: 'Ingest',
  match: 'Match',
  generate: 'Generate',
  enrich: 'Enrich',
  ghost_score: 'Ghost score',
  salary_infer: 'Salary infer',
};

const OP_TONES: Record<ActivityOp, 'green' | 'red' | 'slate'> = {
  ingest: 'slate',
  match: 'slate',
  generate: 'green',
  enrich: 'slate',
  ghost_score: 'slate',
  salary_infer: 'slate',
};

export default function StatusPage() {
  const { data, isLoading, error } = useActivity(100);
  const retry = useRetryActivity();
  const cancelAll = useCancelAllActivity();

  const failed = data?.recent.filter((r) => r.state === 'failed' || r.state === 'cancelled') ?? [];
  const failedByOp = failed.reduce<Record<string, number>>((acc, r) => {
    acc[r.op] = (acc[r.op] ?? 0) + 1;
    return acc;
  }, {});
  const anyCancelled = failed.some((r) => r.state === 'cancelled');

  return (
    <div>
      <PageHeader
        title="Status"
        description="Live and recent activity across scraping, matching, generation and enrichment."
      />

      {isLoading ? <Spinner label="loading activity…" /> : null}
      {error ? <ErrorState error={error} /> : null}

      {failed.length > 0 ? (
        <Surface className="mb-8">
          <div className="mb-3 flex items-center justify-between">
            <SectionTitle>Failed / cancelled ({failed.length})</SectionTitle>
            <Button
              variant="secondary"
              onClick={() => retry.mutate(undefined)}
              disabled={retry.isPending}
            >
              <RotateCw className="h-3 w-3" /> retry all
            </Button>
          </div>
          {anyCancelled ? (
            <p className="mb-3 text-xs text-muted">
              Some of these were cancelled, not failed — an upstream provider (Cerebras or
              OpenRouter) hit its rate limit, so the rest of that batch was skipped instead of
              also erroring out. Retry once the limit resets.
            </p>
          ) : null}
          <ul className="flex flex-wrap gap-2">
            {Object.entries(failedByOp).map(([op, count]) => (
              <li key={op} className="flex items-center gap-2 rounded-lg border border-border px-3 py-1.5">
                <Chip tone={OP_TONES[op as ActivityOp] ?? 'slate'}>
                  {OP_LABELS[op as ActivityOp] ?? op}
                </Chip>
                <span className="text-sm text-muted">{count}</span>
                <Button
                  variant="ghost"
                  onClick={() => retry.mutate(op)}
                  disabled={retry.isPending}
                >
                  <RotateCw className="h-3 w-3" /> retry
                </Button>
              </li>
            ))}
          </ul>
        </Surface>
      ) : null}

      <section className="mb-8">
        <div className="mb-3 flex items-center justify-between">
          <SectionTitle>Active</SectionTitle>
          {data && data.active.length > 0 ? (
            <Button
              variant="secondary"
              onClick={() => cancelAll.mutate()}
              disabled={cancelAll.isPending}
            >
              <X className="h-3 w-3" /> cancel all ({data.active.length})
            </Button>
          ) : null}
        </div>
        {data && data.active.length === 0 ? (
          <EmptyState>Nothing running.</EmptyState>
        ) : (
          <ul className="space-y-3">
            {data?.active.map((run) => (
              <ActiveCard key={run.id} run={run} />
            ))}
          </ul>
        )}
      </section>

      <section>
        <SectionTitle>Recent</SectionTitle>
        {data && data.recent.length === 0 ? (
          <EmptyState>No activity yet.</EmptyState>
        ) : (
          <RecentTable runs={data?.recent ?? []} />
        )}
      </section>
    </div>
  );
}

function ActiveCard({ run }: { run: ActivityRunDto }) {
  const elapsed = useLiveElapsed(run.startedAt);
  const cancel = useCancelActivity();
  return (
    <Surface>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Chip tone={OP_TONES[run.op as ActivityOp] ?? 'slate'}>
              {OP_LABELS[run.op as ActivityOp] ?? run.op}
            </Chip>
            {run.state === 'queued' ? <Chip tone="slate">queued</Chip> : null}
            {run.jobId ? (
              <Link to={`/jobs/${run.jobId}`} className="font-semibold text-primary hover:underline">
                {run.label}
              </Link>
            ) : (
              <span className="font-semibold text-fg">{run.label}</span>
            )}
          </div>
          {run.step ? <p className="mt-1 text-sm text-muted">{run.step}</p> : null}
        </div>
        <div className="flex shrink-0 items-center gap-3">
          {elapsed !== null ? <span className="text-xs tabular-nums text-faint">{elapsed}</span> : null}
          {run.state === 'running' ? <Spinner /> : null}
          <Button
            variant="ghost"
            onClick={() => cancel.mutate(run.id)}
            disabled={cancel.isPending}
          >
            <X className="h-3 w-3" /> cancel
          </Button>
        </div>
      </div>
    </Surface>
  );
}

function RecentTable({ runs }: { runs: ActivityRunDto[] }) {
  return (
    <Surface className="overflow-x-auto p-0">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border text-left text-xs font-semibold uppercase tracking-wide text-faint">
            <th className="px-4 py-2">Op</th>
            <th className="px-4 py-2">Label</th>
            <th className="px-4 py-2">State</th>
            <th className="px-4 py-2">Duration</th>
          </tr>
        </thead>
        <tbody>
          {runs.map((run) => (
            <tr key={run.id} className="border-b border-border last:border-0">
              <td className="px-4 py-2">
                <Chip tone={OP_TONES[run.op as ActivityOp] ?? 'slate'}>
                  {OP_LABELS[run.op as ActivityOp] ?? run.op}
                </Chip>
              </td>
              <td className="px-4 py-2">
                {run.jobId ? (
                  <Link to={`/jobs/${run.jobId}`} className="text-primary hover:underline">
                    {run.label}
                  </Link>
                ) : (
                  <span className="text-fg">{run.label}</span>
                )}
                {(run.state === 'failed' || run.state === 'cancelled') && run.error ? (
                  <p className="mt-0.5 text-xs text-danger">{run.error}</p>
                ) : null}
              </td>
              <td className="px-4 py-2">
                {run.state === 'succeeded' ? (
                  <span className="text-success">✓ succeeded</span>
                ) : run.state === 'failed' ? (
                  <span className="text-danger">✗ failed</span>
                ) : run.state === 'cancelled' ? (
                  <span className="text-amber-500">⊘ cancelled</span>
                ) : (
                  <span className="text-muted">{run.state}</span>
                )}
              </td>
              <td className="px-4 py-2 tabular-nums text-muted">{formatDuration(run.elapsedMs)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </Surface>
  );
}

function useLiveElapsed(startedAt: string | null) {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!startedAt) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [startedAt]);

  if (!startedAt) return null;
  return formatDuration(now - new Date(startedAt).getTime());
}

function formatDuration(ms: number | null | undefined) {
  if (ms === null || ms === undefined) return '—';
  const totalSeconds = Math.max(0, Math.round(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return minutes > 0 ? `${minutes}m ${seconds}s` : `${seconds}s`;
}
