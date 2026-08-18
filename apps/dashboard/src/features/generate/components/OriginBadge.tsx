import { cn } from '../../../lib/utils';

export interface OriginBadgeProps {
  origin: string;
  kind: string;
  className?: string;
}

export default function OriginBadge({ origin, kind, className }: OriginBadgeProps) {
  if (origin === 'profile') {
    return (
      <span
        className={cn(
          'inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset',
          'bg-success-soft text-success ring-success/25',
          className,
        )}
        data-badge="origin-profile"
      >
        from your profile
      </span>
    );
  }

  if (kind === 'summary') {
    return (
      <span
        className={cn(
          'inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset',
          'bg-accent-soft text-accent ring-accent/25',
          className,
        )}
        data-badge="origin-summary"
        title="Written by the model and checked against your profile — not the same guarantee as an unverified suggestion."
      >
        AI-written summary
      </span>
    );
  }

  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset',
        'bg-warning-soft text-warning ring-warning/25',
        className,
      )}
      data-badge="origin-ai-unverified"
      title="A model suggestion, not from your profile. Off by default; review before including it."
    >
      AI · unverified
    </span>
  );
}
