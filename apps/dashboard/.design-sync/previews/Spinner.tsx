import { Spinner } from '@job-finder/dashboard';

export const WithLabel = () => <Spinner label="Scoring jobs…" />;

export const Bare = () => <Spinner />;

export const InlineInPanel = () => (
  <div className="flex items-center justify-between rounded-xl border border-border bg-surface p-4">
    <span className="text-sm text-foreground">Company intel</span>
    <Spinner label="loading…" />
  </div>
);
