import { cn } from '../lib/utils';

export function ScoreBadge({ score }: { score?: number | null }) {
  if (score === null || score === undefined) {
    return (
      <span className="inline-flex shrink-0 items-center rounded-full border border-border bg-surface-tertiary px-2 py-0.5 text-xs font-bold text-faint">
        —
      </span>
    );
  }
  const tone =
    score >= 80
      ? 'bg-success text-success-foreground'
      : score >= 60
        ? 'bg-accent text-accent-foreground'
        : score >= 40
          ? 'bg-warning text-warning-foreground'
          : 'bg-danger text-danger-foreground';
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-bold tabular-nums whitespace-nowrap',
        tone,
      )}
    >
      {score}
    </span>
  );
}

export function GhostBadge({ score }: { score?: number | null }) {
  if (score === null || score === undefined || score < 50) {
    return null;
  }
  const tone = score >= 80 ? 'bg-danger text-danger-foreground' : 'bg-warning text-warning-foreground';
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-xs font-bold tabular-nums whitespace-nowrap',
        tone,
      )}
      title="Ghost-job likelihood score — informational only"
    >
      <span aria-hidden="true">👻</span>
      {score}
    </span>
  );
}

export function HealthDot({ healthy }: { healthy: boolean }) {
  return (
    <span
      className={cn(
        'inline-block h-2.5 w-2.5 rounded-full ring-2',
        healthy ? 'bg-success ring-success-soft' : 'bg-danger ring-danger-soft',
      )}
      title={healthy ? 'healthy' : 'unhealthy'}
    />
  );
}
