import { ReactNode } from 'react';

export function ScoreBadge({ score }: { score: number | null | undefined }) {
  if (score === null || score === undefined) {
    return <span className="rounded-full bg-slate-200 px-2 py-0.5 text-xs text-slate-500">—</span>;
  }
  const color =
    score >= 80
      ? 'bg-emerald-100 text-emerald-800'
      : score >= 60
        ? 'bg-lime-100 text-lime-800'
        : score >= 40
          ? 'bg-amber-100 text-amber-800'
          : 'bg-rose-100 text-rose-800';
  return <span className={`rounded-full px-2 py-0.5 text-xs font-semibold ${color}`}>{score}</span>;
}

export function Chip({ children, tone = 'slate' }: { children: ReactNode; tone?: 'green' | 'red' | 'slate' }) {
  const cls =
    tone === 'green'
      ? 'bg-emerald-50 text-emerald-700 border-emerald-200'
      : tone === 'red'
        ? 'bg-rose-50 text-rose-700 border-rose-200'
        : 'bg-slate-100 text-slate-600 border-slate-200';
  return <span className={`inline-block rounded border px-1.5 py-0.5 text-xs ${cls}`}>{children}</span>;
}

export function Spinner({ label }: { label?: string }) {
  return (
    <span className="inline-flex items-center gap-2 text-sm text-slate-500">
      <span className="h-4 w-4 animate-spin rounded-full border-2 border-slate-300 border-t-sky-600" />
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
}: {
  children: ReactNode;
  onClick?: () => void;
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  disabled?: boolean;
  type?: 'button' | 'submit';
}) {
  const cls = {
    primary: 'bg-sky-600 text-white hover:bg-sky-700 disabled:bg-slate-300',
    secondary: 'bg-white text-slate-700 border border-slate-300 hover:bg-slate-50 disabled:text-slate-400',
    danger: 'bg-rose-600 text-white hover:bg-rose-700 disabled:bg-slate-300',
    ghost: 'text-slate-500 hover:bg-slate-100 disabled:text-slate-300',
  }[variant];
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={`rounded px-3 py-1.5 text-sm font-medium transition-colors ${cls}`}
    >
      {children}
    </button>
  );
}

export function healthDot(healthy: boolean) {
  return (
    <span
      className={`inline-block h-2.5 w-2.5 rounded-full ${healthy ? 'bg-emerald-500' : 'bg-rose-500'}`}
      title={healthy ? 'healthy' : 'unhealthy'}
    />
  );
}
