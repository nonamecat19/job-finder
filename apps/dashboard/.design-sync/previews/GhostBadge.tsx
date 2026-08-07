import { GhostBadge, ScoreBadge } from '@job-finder/dashboard';

export const Bands = () => (
  <div className="flex flex-wrap items-center gap-2">
    <GhostBadge score={95} />
    <GhostBadge score={80} />
    <GhostBadge score={66} />
    <GhostBadge score={50} />
  </div>
);

export const NextToScore = () => (
  <div className="flex max-w-md items-center justify-between gap-3 rounded-xl border border-border bg-surface p-4">
    <div>
      <p className="text-sm font-semibold text-foreground">Senior Data Engineer</p>
      <p className="text-xs text-muted">Initech · Reposted 4 times in 90 days</p>
    </div>
    <div className="flex items-center gap-1.5">
      <GhostBadge score={88} />
      <ScoreBadge score={62} />
    </div>
  </div>
);
