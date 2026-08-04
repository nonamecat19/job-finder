import { Button, Chip, ScoreBadge, Surface, ThemeRoot } from '@job-finder/dashboard';

const Panel = () => (
  <Surface>
    <div className="flex items-center justify-between gap-3">
      <h3 className="text-base font-semibold text-foreground">Senior Backend Engineer</h3>
      <ScoreBadge score={87} />
    </div>
    <p className="mt-2 text-sm text-muted">Acme Corp · Berlin, DE · Hybrid</p>
    <div className="mt-3 flex flex-wrap gap-1.5">
      <Chip tone="green">Go</Chip>
      <Chip tone="slate">Kafka</Chip>
    </div>
    <div className="mt-4 flex justify-end">
      <Button variant="primary">Shortlist</Button>
    </div>
  </Surface>
);

export const Dark = () => (
  <ThemeRoot theme="dark" style={{ padding: 16, borderRadius: 12 }}>
    <Panel />
  </ThemeRoot>
);

export const Light = () => (
  <ThemeRoot theme="light" style={{ padding: 16, borderRadius: 12 }}>
    <Panel />
  </ThemeRoot>
);
