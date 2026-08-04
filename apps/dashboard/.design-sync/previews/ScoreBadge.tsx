import { ScoreBadge } from '@job-finder/dashboard';

export const Bands = () => (
  <div className="flex flex-wrap items-center gap-2">
    <ScoreBadge score={92} />
    <ScoreBadge score={71} />
    <ScoreBadge score={48} />
    <ScoreBadge score={17} />
    <ScoreBadge score={null} />
  </div>
);

export const InAJobRow = () => (
  <div className="flex max-w-md items-center justify-between gap-3 rounded-xl border border-border bg-surface p-4">
    <div>
      <p className="text-sm font-semibold text-foreground">Staff Platform Engineer</p>
      <p className="text-xs text-muted">Globex · Remote (EU)</p>
    </div>
    <ScoreBadge score={84} />
  </div>
);
