import { Checkbox } from '@job-finder/dashboard';

export const States = () => (
  <div className="flex items-center gap-6">
    <label className="flex items-center gap-2 text-sm text-foreground">
      <Checkbox defaultChecked />
      Checked
    </label>
    <label className="flex items-center gap-2 text-sm text-foreground">
      <Checkbox />
      Unchecked
    </label>
    <label className="flex items-center gap-2 text-sm text-faint">
      <Checkbox disabled />
      Disabled
    </label>
  </div>
);

export const FilterList = () => (
  <div className="max-w-xs space-y-2 rounded-xl border border-border bg-surface p-4">
    <label className="flex items-center gap-2 text-sm text-foreground">
      <Checkbox defaultChecked />
      Remote only
    </label>
    <label className="flex items-center gap-2 text-sm text-foreground">
      <Checkbox defaultChecked />
      Hide ghost jobs
    </label>
    <label className="flex items-center gap-2 text-sm text-foreground">
      <Checkbox />
      Include contract roles
    </label>
  </div>
);
