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
import type { ActivityOp, ActivityRunDto } from '@job-finder/shared';
import { PageHeader } from '../../components/layout/PageHeader';
import { DashboardGrid, IconTile, ListRow, Tile, type IconTileTint } from '../../components/layout';
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

const OP_TONES: Record<ActivityOp, 'green' | 'red' | 'slate'> = {
  ingest: 'slate',
  match: 'slate',
  generate: 'green',
  enrich: 'slate',
  ghost_score: 'slate',
  salary_infer: 'slate',
};

// One glyph and one tint per op, kept consistent across the screen.
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
            <p className="mb-3 text-sm text-muted">
              Some of these were cancelled, not failed — an upstream AI provider
              hit its rate limit, so the rest of that batch was skipped instead of
              also erroring out. Retry once the limit resets.
            </p>
          ) : null}
          <ul className="flex flex-wrap gap-2">
            {Object.entries(failedByOp).map(([op, count]) => (
              <li key={op} className="flex items-center gap-2 rounded-lg border border-border bg-surface-secondary px-3 py-1.5">
                <Chip tone={OP_TONES[op as ActivityOp] ?? 'slate'}>
                  {OP_LABELS[op as ActivityOp] ?? op}
                </Chip>
                <span className="font-mono text-sm tabular-nums text-muted">{count}</span>
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
              action={
                q.providerClass ? (
                  <Chip tone={q.providerClass === 'hosted' ? 'green' : 'slate'}>{q.providerClass}</Chip>
                ) : (
                  <span className="[font:var(--type-caption)] text-faint">—</span>
                )
              }
              footer={
                q.error ? (
                  <span className="text-danger">error</span>
                ) : (
                  <span>
                    {q.active} active · {q.processedPerMinute.toFixed(1)}/min · {formatEta(q.etaSeconds)} · conc {q.concurrency}
                  </span>
                )
              }
            >
              <div className="flex flex-col gap-1.5">
                <span className="[font:var(--type-caption)] uppercase tracking-[var(--tracking-wide)] text-muted">
                  pending
                </span>
                <span className="[font:var(--type-figure)] tabular-nums">{q.pending}</span>
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
            estimateSize={64}
            gap={4}
            maxHeight="32rem"
            collapseThreshold={Infinity}
            renderItem={(run) => <ActiveCard run={run} />}
          />
        )}
      </Tile>

      <Tile span="full" title="Recent">
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
    <ListRow
      leading={<IconTile icon={Icon} tint={OP_TINTS[run.op as ActivityOp]} size="md" />}
      title={
        <span className="flex flex-wrap items-center gap-2">
          {run.jobId ? (
            <Link to={`/jobs/${run.jobId}`} className="font-semibold text-accent hover:underline">
              {run.label}
            </Link>
          ) : (
            <span className="font-semibold text-foreground">{run.label}</span>
          )}
          {run.state === 'queued' ? <Chip tone="slate">queued</Chip> : null}
        </span>
      }
      meta={run.step ?? undefined}
      aside={
        <div className="flex shrink-0 items-center gap-3">
          {elapsed !== null ? (
            <span className="font-mono text-xs tabular-nums text-faint">{elapsed}</span>
          ) : null}
          {run.state === 'running' ? <Spinner /> : null}
          <Button
            variant="ghost"
            onClick={() => cancel.mutate(run.id)}
            disabled={cancel.isPending}
          >
            <X className="h-3 w-3" /> cancel
          </Button>
        </div>
      }
    />
  );
}

const RECENT_ROW_GRID = 'grid grid-cols-[minmax(6rem,auto)_1fr_minmax(7rem,auto)_minmax(5rem,auto)] gap-2';

function RecentTable({ runs }: { runs: ActivityRunDto[] }) {
  return (
    <div>
      <div className={cn(RECENT_ROW_GRID, 'px-3 py-2 [font:var(--type-caption)] uppercase tracking-[var(--tracking-wide)] text-muted')}>
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

function formatDuration(ms: number | null | undefined) {
  if (ms === null || ms === undefined) return '—';
  const totalSeconds = Math.max(0, Math.round(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return minutes > 0 ? `${minutes}m ${seconds}s` : `${seconds}s`;
}
