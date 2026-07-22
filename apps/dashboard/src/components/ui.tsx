import { ReactNode, type ComponentPropsWithoutRef, type ElementType } from 'react';
import { cn } from '../lib/utils';

export function ScoreBadge({ score }: { score?: number | null }) {
  if (score === null || score === undefined) {
    return (
      <span className="rounded-full border border-border bg-overlay px-2 py-0.5 text-xs font-semibold text-faint">
        —
      </span>
    );
  }
  const color =
    score >= 80
      ? 'bg-success-soft text-success ring-success/30'
      : score >= 60
        ? 'bg-primary-soft text-primary ring-primary/30'
        : score >= 40
          ? 'bg-warning-soft text-warning ring-warning/30'
          : 'bg-danger-soft text-danger ring-danger/30';
  return (
    <span className={cn('rounded-full px-2 py-0.5 text-xs font-bold ring-1 ring-inset tabular-nums', color)}>
      {score}
    </span>
  );
}

// GhostBadge renders the ghost-job detector's (005) score, informational
// only: yellow for 50-79, red for 80-100, and NOTHING below 50 or when no
// ghost signal exists — the feed must stay quiet for jobs the system isn't
// suspicious of (FR-012, FR-017). This component must never hide, dim, or
// otherwise act on the job itself; it only ever renders a badge next to
// ScoreBadge (Constitution Principle I / FR-015).
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

export function Chip({ children, tone = 'slate' }: { children: ReactNode; tone?: 'green' | 'red' | 'slate' }) {
  const cls =
    tone === 'green'
      ? 'bg-success-soft text-success ring-success/25'
      : tone === 'red'
        ? 'bg-danger-soft text-danger ring-danger/25'
        : 'bg-overlay text-muted ring-border';
  return (
    <span className={cn('inline-flex items-center rounded-md px-1.5 py-0.5 text-xs font-medium ring-1 ring-inset', cls)}>
      {children}
    </span>
  );
}

export function Spinner({ label }: { label?: string }) {
  return (
    <span className="inline-flex items-center gap-2 text-sm text-muted">
      <span className="h-4 w-4 animate-spin rounded-full border-2 border-border-strong border-t-primary" />
      {label}
    </span>
  );
}

export function Button({
  children,
  onClick,
  variant = 'primary',
  disabled,
  type = 'button',
  className,
  ...props
}: {
  children: ReactNode;
  onClick?: () => void;
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  disabled?: boolean;
  type?: 'button' | 'submit';
} & Omit<ComponentPropsWithoutRef<'button'>, 'type' | 'onClick' | 'disabled'>) {
  const cls = {
    primary:
      'bg-primary text-primary-fg shadow-sm shadow-primary/25 hover:brightness-110 disabled:bg-overlay disabled:text-faint disabled:shadow-none',
    secondary:
      'border border-border bg-elevated text-fg shadow-sm hover:border-border-strong hover:bg-overlay disabled:text-faint',
    danger: 'bg-danger text-white hover:brightness-110 disabled:bg-overlay disabled:text-faint',
    ghost: 'text-muted hover:bg-overlay hover:text-fg disabled:text-faint',
  }[variant];
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={cn(
        'inline-flex items-center justify-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-semibold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:cursor-not-allowed',
        cls,
        className,
      )}
      {...props}
    >
      {children}
    </button>
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

export function Field({
  label,
  children,
  className,
}: {
  label: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <label className={cn('block text-xs font-semibold uppercase tracking-wide text-faint', className)}>
      <span className="mb-1.5 block">{label}</span>
      {children}
    </label>
  );
}

const controlClass =
  'w-full rounded-lg border border-border bg-elevated px-3 py-2 text-sm text-fg shadow-sm outline-none transition placeholder:text-faint focus:border-primary focus:ring-2 focus:ring-primary-soft disabled:bg-overlay disabled:text-faint';

export function Input(props: ComponentPropsWithoutRef<'input'>) {
  return <input {...props} className={cn(controlClass, props.className)} />;
}

export function Textarea(props: ComponentPropsWithoutRef<'textarea'>) {
  return <textarea {...props} className={cn(controlClass, props.className)} />;
}

export function Select(props: ComponentPropsWithoutRef<'select'>) {
  return <select {...props} className={cn(controlClass, props.className)} />;
}

export function Checkbox(props: ComponentPropsWithoutRef<'input'>) {
  return (
    <input
      type="checkbox"
      {...props}
      className={cn(
        'h-4 w-4 rounded border-border-strong bg-elevated text-primary accent-primary focus:ring-primary',
        props.className,
      )}
    />
  );
}

export function Surface({
  as,
  children,
  className,
}: {
  as?: ElementType;
  children: ReactNode;
  className?: string;
}) {
  const Comp = as ?? 'section';
  return (
    <Comp className={cn('rounded-xl border border-border bg-surface p-4 shadow-sm shadow-black/20', className)}>
      {children}
    </Comp>
  );
}

export function EmptyState({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-xl border border-dashed border-border-strong bg-surface/60 p-6 text-sm text-muted">
      {children}
    </div>
  );
}

export function ErrorState({ error }: { error: unknown }) {
  return (
    <div className="rounded-xl border border-danger/30 bg-danger-soft p-3 text-sm text-danger">
      {error instanceof Error ? error.message : String(error)}
    </div>
  );
}
