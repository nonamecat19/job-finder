import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  DollarSign,
  Ghost,
  GitCompare,
  Inbox,
  RotateCw,
  Sparkles,
  Wand2,
  X,
  type LucideIcon,
} from 'lucide-react';
import type { ActivityOp, ActivityRunDto, QueueBacklogDto } from '@job-finder/shared';
import { PageHeader } from '../../components/layout/PageHeader';
import { IconTile, Tile, type IconTileTint } from '../../components/layout';
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

const QUEUE_LABELS: Record<string, string> = {
  ingest: 'Ingest',
  match: 'Match',
  generate: 'Generate',
  enrich: 'Enrich',
  'salary:infer': 'Salary infer',
  'ghost:score': 'Ghost score',
};

const QUEUE_ICONS: Record<string, LucideIcon> = {
  ingest: Inbox,
  match: GitCompare,
  generate: Sparkles,
  enrich: Wand2,
  'salary:infer': DollarSign,
  'ghost:score': Ghost,
};

const QUEUE_TINTS: Record<string, IconTileTint> = {
  ingest: 'blue',
  match: 'violet',
  generate: 'mint',
  enrich: 'amber',
  'salary:infer': 'blue',
  'ghost:score': 'rose',
};

const OP_TONES: Record<ActivityOp, 'green' | 'red' | 'slate'> = {
  ingest: 'slate',
  match: 'slate',
  generate: 'green',
  enrich: 'slate',
  ghost_score: 'slate',
  salary_infer: 'slate',
};

const OP_ICONS: Record<ActivityOp, LucideIcon> = {
  ingest: Inbox,
  match: GitCompare,
  generate: Sparkles,
  enrich: Wand2,
  ghost_score: Ghost,
  salary_infer: DollarSign,
};

const OP_TINTS: Record<ActivityOp, IconTileTint> = {
  ingest: 'blue',
  match: 'violet',
  generate: 'mint',
  enrich: 'amber',
  ghost_score: 'rose',
  salary_infer: 'blue',
};

export default function StatusPage() {
  const { data, isLoading, error, dataUpdatedAt } = useActivity(100);
  const { data: backlog } = useQueueBacklog();
  const retry = useRetryActivity();
  const cancelAll = useCancelAllActivity();
  const updatedAgo = useLiveAgo(dataUpdatedAt);

  const failed =
    data?.recent.filter(
      (r) => r.state === 'failed' || r.state === 'cancelled' || r.state === 'timed_out' || r.state === 'interrupted',
    ) ?? [];
  const failedByOp = failed.reduce<Record<string, number>>((acc, r) => {
    acc[r.op] = (acc[r.op] ?? 0) + 1;
    return acc;
  }, {});
  const anyCancelled = failed.some((r) => r.state === 'cancelled');

  const totalPending = backlog?.queues.reduce((sum, q) => sum + q.pending, 0) ?? 0;
  const totalActive = backlog?.queues.reduce((sum, q) => sum + q.active, 0) ?? 0;
  const totalRate = backlog?.queues.reduce((sum, q) => sum + q.processedPerMinute, 0) ?? 0;
  const anyUnhealthy = backlog?.queues.some((q) => q.error) ?? false;

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <PageHeader
        title="Status"
        description="Live and recent activity across scraping, matching, generation and enrichment."
        actions={
          backlog ? (
            <div className="flex items-center gap-3">
              <div className="flex items-center gap-2 rounded-xl border border-border bg-surface px-3 py-1.5">
                <span
                  className={cn(
                    'inline-block h-2 w-2 shrink-0 rounded-full',
                    anyUnhealthy ? 'bg-danger shadow-[0_0_0_3px_var(--danger-soft)]' : 'bg-success shadow-[0_0_0_3px_var(--success-soft)]',
                  )}
                />
                <span className="text-sm font-semibold text-foreground">{totalPending} queued</span>
                <span className="font-mono text-xs tabular-nums text-faint">
                  · {totalActive} active · {totalRate.toFixed(1)}/min
                </span>
              </div>
              {updatedAgo ? (
                <span className="font-mono text-xs tabular-nums text-faint">updated {updatedAgo}</span>
              ) : null}
            </div>
          ) : undefined
        }
      />

      {isLoading ? <ActivitySkeleton /> : null}
      {error ? <ErrorState error={error} /> : null}

      {backlog && backlog.queues.length > 0 ? <QueueStrip queues={backlog.queues} /> : null}

      {failed.length > 0 ? (
        <div className="flex flex-wrap items-center gap-3 rounded-xl border border-danger/35 bg-danger-soft px-3 py-2">
          <span className="shrink-0 text-sm font-semibold text-danger">{failed.length} failed / cancelled</span>
          <div className="flex flex-wrap items-center gap-1.5">
            {Object.entries(failedByOp).map(([op, count]) => (
              <span key={op} className="flex items-center gap-1.5 rounded-full border border-border bg-surface py-0.5 pr-1 pl-1">
                <Chip tone={OP_TONES[op as ActivityOp] ?? 'slate'}>{OP_LABELS[op as ActivityOp] ?? op}</Chip>
                <span className="font-mono text-xs tabular-nums text-muted">{count}</span>
                <Button variant="ghost" onClick={() => retry.mutate(op)} disabled={retry.isPending}>
                  <RotateCw className="h-3 w-3" /> retry
                </Button>
              </span>
            ))}
          </div>
          {anyCancelled ? (
            <span className="min-w-0 flex-1 text-xs text-muted">
              Some of these were cancelled, not failed — an upstream provider hit its rate limit. Retry once it
              resets.
            </span>
          ) : (
            <span className="min-w-0 flex-1" />
          )}
          <Button variant="secondary" onClick={() => retry.mutate(undefined)} disabled={retry.isPending}>
            <RotateCw className="h-3 w-3" /> retry all
          </Button>
        </div>
      ) : null}

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 lg:grid-cols-[5fr_7fr]">
        <Tile
          title="Active"
          className="min-h-0"
          scroll
          scrollLabel="Active runs"
          action={
            data && data.active.length > 0 ? (
              <Button variant="secondary" onClick={() => cancelAll.mutate()} disabled={cancelAll.isPending}>
                <X className="h-3 w-3" /> cancel all ({data.active.length})
              </Button>
            ) : undefined
          }
        >
          {data && data.active.length === 0 ? (
            <EmptyState>Nothing running.</EmptyState>
          ) : (
            <div className="flex flex-col gap-2.5">
              {(data?.active ?? []).map((run) => (
                <ActiveCard key={run.id} run={run} />
              ))}
            </div>
          )}
        </Tile>

        <Tile title="Recent" className="min-h-0" scroll={false}>
          {data && data.recent.length === 0 ? <EmptyState>No activity yet.</EmptyState> : <RecentTable runs={data?.recent ?? []} />}
        </Tile>
      </div>
    </div>
  );
}

function QueueStrip({ queues }: { queues: QueueBacklogDto[] }) {
  return (
    <div className="grid grid-cols-2 divide-x divide-separator overflow-hidden rounded-2xl border border-border bg-surface shadow-tile sm:grid-cols-3 lg:grid-cols-6 lg:divide-x">
      {queues.map((q) => {
        const Icon = QUEUE_ICONS[q.queue] ?? Inbox;
        const loadPct = q.concurrency > 0 ? Math.min(100, Math.round((q.active / q.concurrency) * 100)) : 0;
        return (
          <div key={q.queue} className="flex flex-col gap-2 px-3.5 py-2.5">
            <div className="flex items-center gap-2">
              <IconTile icon={Icon} tint={QUEUE_TINTS[q.queue] ?? 'blue'} size="sm" />
              <span className="min-w-0 flex-1 truncate text-sm font-semibold text-foreground">
                {QUEUE_LABELS[q.queue] ?? q.queue}
              </span>
              <span
                className={cn(
                  'inline-block h-2 w-2 shrink-0 rounded-full',
                  q.error ? 'bg-danger shadow-[0_0_0_3px_var(--danger-soft)]' : 'bg-success shadow-[0_0_0_3px_var(--success-soft)]',
                )}
              />
            </div>
            <div className="flex items-baseline gap-1.5">
              <span className="[font:var(--type-figure-sm)] tabular-nums text-foreground">{q.pending}</span>
              <span className="text-xs text-faint">pending</span>
            </div>
            <div className="h-[3px] overflow-hidden rounded-full bg-surface-tertiary">
              <div className="h-full bg-accent" style={{ width: `${loadPct}%` }} />
            </div>
            <div className="font-mono text-xs tabular-nums text-muted">
              {q.active} active · {q.processedPerMinute.toFixed(1)}/min · {formatEta(q.etaSeconds ?? null)}
            </div>
            <div className="flex items-center gap-1.5">
              {q.providerClass ? (
                <Chip tone={q.providerClass === 'hosted' ? 'green' : 'slate'}>{q.providerClass}</Chip>
              ) : (
                <span className="[font:var(--type-caption)] text-faint">—</span>
              )}
              <span className="font-mono text-xs tabular-nums text-faint">conc {q.concurrency}</span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function ActivitySkeleton() {
  return (
    <LoadingRegion label="loading activity…" className="mb-8 flex flex-col gap-3">
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="rounded-2xl border border-border bg-surface p-4 shadow-tile">
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
  const Icon = OP_ICONS[run.op as ActivityOp];
  return (
    <div className="rounded-2xl border border-border bg-surface p-3.5 shadow-card">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-2.5">
          <IconTile icon={Icon} tint={OP_TINTS[run.op as ActivityOp]} size="sm" />
          <div className="flex min-w-0 flex-col gap-0.5">
            <div className="flex flex-wrap items-center gap-2">
              <Chip tone={OP_TONES[run.op as ActivityOp] ?? 'slate'}>{OP_LABELS[run.op as ActivityOp] ?? run.op}</Chip>
              {run.jobId ? (
                <Link to={`/jobs/${run.jobId}`} className="truncate font-semibold text-accent hover:underline">
                  {run.label}
                </Link>
              ) : (
                <span className="truncate font-semibold text-foreground">{run.label}</span>
              )}
              {run.state === 'queued' ? <Chip tone="slate">queued</Chip> : null}
            </div>
            {run.step ? <p className="truncate text-sm text-muted">{run.step}</p> : null}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2.5">
          {elapsed !== null ? <span className="font-mono text-xs tabular-nums text-faint">{elapsed}</span> : null}
          {run.state === 'running' ? <Spinner /> : null}
          <Button variant="ghost" onClick={() => cancel.mutate(run.id)} disabled={cancel.isPending}>
            <X className="h-3 w-3" /> cancel
          </Button>
        </div>
      </div>
    </div>
  );
}

const RECENT_ROW_GRID = 'grid grid-cols-[minmax(6rem,auto)_1fr_minmax(7rem,auto)_minmax(5rem,auto)] gap-2';

function RecentTable({ runs }: { runs: ActivityRunDto[] }) {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className={cn(RECENT_ROW_GRID, 'shrink-0 border-b border-border px-5 py-2 [font:var(--type-caption)] uppercase tracking-[var(--tracking-wide)] text-muted')}>
        <span>Op</span>
        <span>Label</span>
        <span>State</span>
        <span>Duration</span>
      </div>
      <VirtualList
        items={runs}
        getKey={(run) => run.id}
        estimateSize={40}
        gap={0}
        maxHeight="100%"
        collapseThreshold={Infinity}
        className="flex-1 px-2"
        renderItem={(run) => <RecentRow run={run} />}
      />
    </div>
  );
}

function RecentRow({ run }: { run: ActivityRunDto }) {
  return (
    <div className={cn(RECENT_ROW_GRID, 'items-center rounded-lg px-3 py-2 text-sm hover:bg-surface-tertiary/60')}>
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
          <span className="text-warning">⊘ cancelled</span>
        ) : run.state === 'timed_out' ? (
          <span className="text-danger">⏱ timed out</span>
        ) : run.state === 'interrupted' ? (
          <span className="text-warning">⚠ interrupted</span>
        ) : (
          <span className="text-muted">{run.state}</span>
        )}
      </span>
      <span className="font-mono tabular-nums text-muted">{formatDuration(run.elapsedMs)}</span>
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

function useLiveAgo(since: number | undefined) {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!since) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [since]);

  if (!since) return null;
  const seconds = Math.max(0, Math.round((now - since) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  return `${Math.floor(seconds / 60)}m ago`;
}

function formatDuration(ms: number | null | undefined) {
  if (ms === null || ms === undefined) return '—';
  const totalSeconds = Math.max(0, Math.round(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return minutes > 0 ? `${minutes}m ${seconds}s` : `${seconds}s`;
}
