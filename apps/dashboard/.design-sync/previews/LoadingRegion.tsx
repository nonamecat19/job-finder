import { LoadingRegion, SkeletonCircle, SkeletonLine } from '@job-finder/dashboard';

export const ApplicationList = () => (
  <LoadingRegion label="loading applications…" className="flex max-w-md flex-col gap-3">
    <div className="space-y-2 rounded-xl border border-border bg-surface p-4">
      <SkeletonLine width="w-1/3" />
      <SkeletonLine width="w-3/4" />
    </div>
    <div className="space-y-2 rounded-xl border border-border bg-surface p-4">
      <SkeletonLine width="w-1/2" />
      <SkeletonLine width="w-2/3" />
    </div>
  </LoadingRegion>
);

export const ContactLine = () => (
  <LoadingRegion label="loading contact…" className="flex items-center gap-2">
    <SkeletonCircle size="sm" />
    <SkeletonLine width="w-40" />
  </LoadingRegion>
);
