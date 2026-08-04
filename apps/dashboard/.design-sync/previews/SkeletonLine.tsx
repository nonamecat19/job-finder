import { SkeletonLine } from '@job-finder/dashboard';

export const Widths = () => (
  <div className="space-y-2">
    <SkeletonLine width="w-full" />
    <SkeletonLine width="w-3/4" />
    <SkeletonLine width="w-1/2" />
    <SkeletonLine width="w-24" />
  </div>
);

export const Paragraph = () => (
  <div className="max-w-sm space-y-2 rounded-xl border border-border bg-surface p-4">
    <SkeletonLine width="w-1/3" />
    <SkeletonLine width="w-full" />
    <SkeletonLine width="w-full" />
    <SkeletonLine width="w-2/3" />
  </div>
);
