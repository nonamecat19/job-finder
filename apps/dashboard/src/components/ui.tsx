import { ReactNode, type ComponentPropsWithoutRef, type ElementType } from 'react';
import { cn } from '../lib/utils';

export { ScoreBadge, GhostBadge, HealthDot } from './badges';

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
      'bg-accent text-accent-foreground shadow-sm shadow-accent/25 hover:brightness-110 disabled:bg-surface-tertiary disabled:text-faint disabled:shadow-none',
    secondary:
      'border border-border bg-surface-secondary text-foreground shadow-sm hover:border-border-strong hover:bg-surface-tertiary disabled:text-faint',
    danger: 'bg-danger text-white hover:brightness-110 disabled:bg-surface-tertiary disabled:text-faint',
    ghost: 'text-muted hover:bg-surface-tertiary hover:text-foreground disabled:text-faint',
  }[variant];
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={cn(
        'inline-flex items-center justify-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-semibold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed',
        cls,
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

export function Chip({ children, tone = 'slate' }: { children: ReactNode; tone?: 'green' | 'red' | 'slate' }) {
  const cls =
    tone === 'green'
      ? 'bg-success-soft text-success ring-success/25'
      : tone === 'red'
        ? 'bg-danger-soft text-danger ring-danger/25'
        : 'bg-surface-tertiary text-muted ring-border';
  return (
    <span className={cn('inline-flex items-center rounded-md px-1.5 py-0.5 text-xs font-medium ring-1 ring-inset', cls)}>
      {children}
    </span>
  );
}

export function Spinner({ label }: { label?: string }) {
  return (
    <span className="inline-flex items-center gap-2 text-sm text-muted">
      <span className="h-4 w-4 animate-spin rounded-full border-2 border-border-strong border-t-accent" />
      {label}
    </span>
  );
}

export function SkeletonLine({ width, className }: { width?: string; className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={cn('h-3 animate-pulse rounded bg-surface-tertiary', width ?? 'w-full', className)}
    />
  );
}

export function SkeletonBlock({ className }: { className?: string }) {
  return <div aria-hidden="true" className={cn('animate-pulse rounded-xl bg-surface-tertiary', className)} />;
}

export function SkeletonCircle({ size = 'md', className }: { size?: 'sm' | 'md' | 'lg'; className?: string }) {
  const dim = { sm: 'h-6 w-6', md: 'h-8 w-8', lg: 'h-12 w-12' }[size];
  return <div aria-hidden="true" className={cn('animate-pulse rounded-full bg-surface-tertiary', dim, className)} />;
}

export function LoadingRegion({
  label,
  children,
  className,
}: {
  label: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div role="status" aria-busy="true" className={className}>
      <span className="sr-only">{label}</span>
      {children}
    </div>
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
  'w-full rounded-lg border border-border bg-surface-secondary px-3 py-2 text-sm text-foreground shadow-sm outline-none transition placeholder:text-faint focus:border-accent focus:ring-2 focus:ring-accent-soft disabled:bg-surface-tertiary disabled:text-faint';

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
        'h-4 w-4 rounded border-border-strong bg-surface-secondary text-accent accent-accent focus:ring-accent',
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
    <Comp className={cn('rounded-xl border border-border bg-surface p-4 shadow-sm shadow-black/20 max-w-3xl', className)}>
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
