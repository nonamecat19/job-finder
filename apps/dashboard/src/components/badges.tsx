import { type ReactNode } from 'react';
import { Chip } from '@heroui/react/chip';
import { cn } from '../lib/utils';

export function ScoreBadge({ score }: { score?: number | null }) {
  if (score === null || score === undefined) {
    return (
      <span className="rounded-full border border-border bg-surface-tertiary px-2 py-0.5 text-xs font-semibold text-faint">
        —
      </span>
    );
  }
  const color =
    score >= 80
      ? 'bg-success-soft text-success ring-success/30'
      : score >= 60
        ? 'bg-accent-soft text-accent ring-accent/30'
        : score >= 40
          ? 'bg-warning-soft text-warning ring-warning/30'
          : 'bg-danger-soft text-danger ring-danger/30';
  return (
    <span className={cn('rounded-full px-2 py-0.5 text-xs font-bold ring-1 ring-inset tabular-nums', color)}>
      {score}
    </span>
  );
}

export function GhostBadge({ score }: { score?: number | null }) {
  if (score === null || score === undefined || score < 50) {
    return null;
  }
  const tone = score >= 80 ? 'bg-danger-soft text-danger ring-danger/30' : 'bg-warning-soft text-warning ring-warning/30';
  return (
    <span
      className={cn('rounded-full px-2 py-0.5 text-xs font-bold ring-1 ring-inset tabular-nums', tone)}
      title="Ghost-job likelihood score — informational only"
    >
      👻 {score}
    </span>
  );
}

export function HealthDot({ healthy }: { healthy: boolean }) {
  return (
    <span
      className={cn(
        'inline-block h-2.5 w-2.5 rounded-full ring-2',
        healthy ? 'bg-success ring-success/25' : 'bg-danger ring-danger/25',
      )}
      title={healthy ? 'healthy' : 'unhealthy'}
    />
  );
}
