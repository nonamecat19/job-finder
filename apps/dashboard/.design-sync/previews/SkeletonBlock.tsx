import { SkeletonBlock } from '@job-finder/dashboard';

export const Sizes = () => (
  <div className="flex flex-wrap items-end gap-3">
    <SkeletonBlock className="h-16 w-24" />
    <SkeletonBlock className="h-24 w-40" />
    <SkeletonBlock className="h-32 w-56" />
  </div>
);

export const CardPlaceholder = () => (
  <div className="max-w-sm space-y-3 rounded-xl border border-border bg-surface p-4">
    <SkeletonBlock className="h-32 w-full" />
    <SkeletonBlock className="h-4 w-2/3" />
    <SkeletonBlock className="h-4 w-1/3" />
  </div>
);
