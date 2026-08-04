import { SkeletonCircle, SkeletonLine } from '@job-finder/dashboard';

export const Sizes = () => (
  <div className="flex items-center gap-4">
    <SkeletonCircle size="sm" />
    <SkeletonCircle size="md" />
    <SkeletonCircle size="lg" />
  </div>
);

export const ContactRow = () => (
  <div className="flex max-w-sm items-center gap-3 rounded-xl border border-border bg-surface p-4">
    <SkeletonCircle size="lg" />
    <div className="flex-1 space-y-2">
      <SkeletonLine width="w-1/2" />
      <SkeletonLine width="w-3/4" />
    </div>
  </div>
);
