import { HealthDot } from '@job-finder/dashboard';

export const States = () => (
  <div className="flex items-center gap-6 text-sm text-foreground">
    <span className="flex items-center gap-2">
      <HealthDot healthy />
      Healthy
    </span>
    <span className="flex items-center gap-2">
      <HealthDot healthy={false} />
      Unhealthy
    </span>
  </div>
);

export const ServiceList = () => (
  <div className="max-w-xs space-y-2 rounded-xl border border-border bg-surface p-4">
    <div className="flex items-center justify-between text-sm text-foreground">
      <span>Gateway</span>
      <HealthDot healthy />
    </div>
    <div className="flex items-center justify-between text-sm text-foreground">
      <span>Scoring worker</span>
      <HealthDot healthy />
    </div>
    <div className="flex items-center justify-between text-sm text-foreground">
      <span>Resume renderer</span>
      <HealthDot healthy={false} />
    </div>
  </div>
);
