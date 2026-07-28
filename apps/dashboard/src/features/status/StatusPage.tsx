import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { RotateCw, X } from 'lucide-react';
import type { ActivityOp, ActivityRunDto } from '@job-finder/shared';
import { PageHeader } from '../../components/layout/PageHeader';
import { DashboardGrid, Tile } from '../../components/layout';
import { VirtualList } from '../../components/VirtualList';
import {
  Button,
  Chip,
  EmptyState,
  ErrorState,
  LoadingRegion,
  Spinner,
  SkeletonBlock,
  SkeletonLine,
  Surface,
} from '../../components/ui';
import { cn } from '../../lib/utils';
import {
  useActivity,
  useCancelActivity,
  useCancelAllActivity,
  useQueueBacklog,
  useRetryActivity,
} from './hooks';

const OP_LABELS: Record<ActivityOp, string> = {
  ingest: 'Ingest',
  match: 'Match',
  generate: 'Generate',
  enrich: 'Enrich',
  ghost_score: 'Ghost score',
  salary_infer: 'Salary infer',
};

// Queue names as reported by GET /api/activity/queues don't match ActivityOp
// (e.g. "salary:infer" vs "salary_infer"); map separately for display.
const QUEUE_LABELS: Record<string, string> = {
  ingest: 'Ingest',
  match: 'Match',
  generate: 'Generate',
  enrich: 'Enrich',
  'salary:infer': 'Salary infer',
  'ghost:score': 'Ghost score',
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
  const { data: backlog } = useQueueBacklog();
  const retry = useRetryActivity();
  const cancelAll = useCancelAllActivity();

  const failed =
    data?.recent.filter(
      (r) => r.state === 'failed' || r.state === 'cancelled' || r.state === 'timed_out' || r.state === 'interrupted',
    ) ?? [];
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

      {isLoading ? <ActivitySkeleton /> : null}
      {error ? <ErrorState error={error} /> : null}

      <DashboardGrid>

      {failed.length > 0 ? (
        <Tile
          span="full"
          title={`Failed / cancelled (${failed.length})`}
          action={
            <Button
              variant="secondary"
              onClick={() => retry.mutate(undefined)}
              disabled={retry.isPending}
            >
              <RotateCw className="h-3 w-3" /> retry all
            </Button>
          }
        >
          {anyCancelled ? (
            <p className="mb-3 text-xs text-muted">
              Some of these were cancelled, not failed — an upstream provider (Cerebras)
              hit its rate limit, so the rest of that batch was skipped instead of
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
        </Tile>
      ) : null}

      {backlog && backlog.queues.length > 0
        ? backlog.queues.map((q) => (
            <Tile
              key={q.queue}
              span="compact"
              title={QUEUE_LABELS[q.queue] ?? q.queue}
              footer={
                q.error ? (
                  <span className="text-danger">error</span>
                ) : (
                  <span>
                    {q.pending} pending &middot; {q.active} active &middot; {q.processedPerMinute.toFixed(1)}/min &middot; {formatEta(q.etaSeconds)}
                  </span>
                )
              }
            >
              <div className="flex items-center gap-2">
                {q.providerClass ? (
                  <Chip tone={q.providerClass === 'hosted' ? 'green' : 'slate'}>{q.providerClass}</Chip>
                ) : (
                  <span className="text-xs text-muted">—</span>
                )}
                <span className="text-xs tabular-nums text-muted">conc {q.concurrency}</span>
              </div>
            </Tile>
          ))
        : null}

      <Tile
        span="wide"
        title="Active"
        action={
          data && data.active.length > 0 ? (
            <Button
              variant="secondary"
              onClick={() => cancelAll.mutate()}
              disabled={cancelAll.isPending}
            >
              <X className="h-3 w-3" /> cancel all ({data.active.length})
            </Button>
          ) : undefined
        }
      >
        {data && data.active.length === 0 ? (
          <EmptyState>Nothing running.</EmptyState>
        ) : (
          <VirtualList
            items={data?.active ?? []}
            getKey={(run) => run.id}
            estimateSize={88}
            gap={12}
            maxHeight="32rem"
            collapseThreshold={Infinity}
            renderItem={(run) => <ActiveCard run={run} />}
          />
        )}
      </Tile>

      <Tile span="wide" title="Recent">
        {data && data.recent.length === 0 ? (
          <EmptyState>No activity yet.</EmptyState>
        ) : (
          <RecentTable runs={data?.recent ?? []} />
        )}
      </Tile>

      </DashboardGrid>
    </div>
  );
}

function ActivitySkeleton() {
  return (
    <LoadingRegion label="loading activity…" className="mb-8 flex flex-col gap-3">
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="rounded-xl border border-border bg-surface p-4 shadow-sm shadow-black/20">
          <SkeletonLine width="w-1/3" />
          <SkeletonBlock className="mt-2 h-4 w-1/2" />
        </div>
      ))}
    </LoadingRegion>
  );
}

function formatEta(etaSeconds: number | null): string {
  if (etaSeconds == null) return '—';
  const minutes = Math.round(etaSeconds / 60);
  if (minutes < 1) return '<1m';
  if (minutes < 60) return `${minutes}m`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
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
              <Link to={`/jobs/${run.jobId}`} className="font-semibold text-accent hover:underline">
                {run.label}
              </Link>
            ) : (
              <span className="font-semibold text-foreground">{run.label}</span>
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

const RECENT_ROW_GRID = 'grid grid-cols-[minmax(6rem,auto)_1fr_minmax(7rem,auto)_minmax(5rem,auto)] gap-2 px-4';

function RecentTable({ runs }: { runs: ActivityRunDto[] }) {
  return (
    <Surface className="overflow-x-auto p-0">
      <div className={cn(RECENT_ROW_GRID, 'border-b border-border py-2 text-left text-xs font-semibold uppercase tracking-wide text-faint')}>
        <span>Op</span>
        <span>Label</span>
        <span>State</span>
        <span>Duration</span>
      </div>
      <VirtualList
        items={runs}
        getKey={(run) => run.id}
        estimateSize={44}
        gap={0}
        renderItem={(run) => <RecentRow run={run} />}
      />
    </Surface>
  );
}

function RecentRow({ run }: { run: ActivityRunDto }) {
  return (
    <div className={cn(RECENT_ROW_GRID, 'items-center border-b border-border py-2 text-sm last:border-0')}>
      <span>
        <Chip tone={OP_TONES[run.op as ActivityOp] ?? 'slate'}>
          {OP_LABELS[run.op as ActivityOp] ?? run.op}
        </Chip>
      </span>
      <span className="min-w-0">
        {run.jobId ? (
          <Link to={`/jobs/${run.jobId}`} className="text-accent hover:underline">
            {run.label}
          </Link>
        ) : (
          <span className="text-foreground">{run.label}</span>
        )}
        {(run.state === 'failed' ||
          run.state === 'cancelled' ||
          run.state === 'timed_out' ||
          run.state === 'interrupted') &&
        run.error ? <p className="mt-0.5 text-xs text-danger">{run.error}</p> : null}
      </span>
      <span>
        {run.state === 'succeeded' ? (
          <span className="text-success">✓ succeeded</span>
        ) : run.state === 'failed' ? (
          <span className="text-danger">✗ failed</span>
        ) : run.state === 'cancelled' ? (
          <span className="text-amber-500">⊘ cancelled</span>
        ) : run.state === 'timed_out' ? (
          <span className="text-danger">⏱ timed out</span>
        ) : run.state === 'interrupted' ? (
          <span className="text-amber-500">⚠ interrupted</span>
        ) : (
          <span className="text-muted">{run.state}</span>
        )}
      </span>
      <span className="tabular-nums text-muted">{formatDuration(run.elapsedMs)}</span>
    </div>
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
