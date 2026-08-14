import { Chip, LoadingRegion, SkeletonBlock } from '../../components/ui';
import { usePostAgeSignal } from './hooks';

export default function PostAgeSignal() {
  const { data, isLoading, error } = usePostAgeSignal();

  if (isLoading) {
    return (
      <LoadingRegion label="loading response rate…" className="space-y-2">
        <SkeletonBlock className="h-8 w-full" />
        <SkeletonBlock className="h-8 w-full" />
        <SkeletonBlock className="h-8 w-full" />
      </LoadingRegion>
    );
  }
  if (error) return null;
  if (!data) return null;

  const bucketLabel: Record<string, string> = {
    fresh: 'Applied within 2 days',
    recent: 'Applied within 3–7 days',
    aging: 'Applied within 8–21 days',
    stale: 'Applied 22+ days after posting',
    unknown: 'Posting date unknown',
  };

  return (
    <>
      {data.globalState === 'prior' && data.thresholdMsg ? (
        <p className="mb-3 text-sm text-muted">{data.thresholdMsg}</p>
      ) : null}
      <div className="flex flex-col gap-1">
        {data.buckets.map((b) => (
          <div key={b.bucket} className="flex items-center justify-between gap-3 rounded-lg bg-surface-tertiary/60 px-3 py-2 text-sm">
            <div className="min-w-0">
              <span className="font-medium capitalize text-foreground">{b.bucket}</span>
              <span className="ml-2 text-xs text-faint">{bucketLabel[b.bucket] ?? b.bucket}</span>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              {b.state === 'observed' && b.rate !== null ? (
                <span className="[font:var(--type-figure-sm)] tabular-nums text-foreground">{(b.rate * 100).toFixed(0)}%</span>
              ) : b.state === 'insufficient' ? (
                <Chip>not enough data</Chip>
              ) : (
                <span className="text-xs text-faint">—</span>
              )}
              <span className="text-xs text-faint">({b.n})</span>
            </div>
          </div>
        ))}
      </div>
      {data.globalState === 'prior' ? (
        <p className="mt-2 text-xs text-faint">
          {data.priorLabel} — {data.priorRate * 100}% baseline
        </p>
      ) : null}
    </>
  );
}
