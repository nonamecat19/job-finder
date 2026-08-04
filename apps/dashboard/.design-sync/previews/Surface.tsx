import { Button, Chip, ScoreBadge, Surface } from '@job-finder/dashboard';

export const JobCard = () => (
  <Surface>
    <div className="flex items-start justify-between gap-3">
      <div>
        <h3 className="text-base font-semibold text-foreground">Senior Backend Engineer</h3>
        <p className="text-sm text-muted">Acme Corp · Berlin, DE · Hybrid</p>
      </div>
      <ScoreBadge score={87} />
    </div>
    <p className="mt-3 text-sm text-muted">
      Go and PostgreSQL services behind a high-volume payments API. Strong match on your Go, Kafka and
      infrastructure-as-code experience; the posting also asks for Kubernetes on-call rotation.
    </p>
    <div className="mt-3 flex flex-wrap gap-1.5">
      <Chip tone="green">Go</Chip>
      <Chip tone="green">PostgreSQL</Chip>
      <Chip tone="slate">Kafka</Chip>
      <Chip tone="red">Kubernetes</Chip>
    </div>
    <div className="mt-4 flex justify-end gap-2">
      <Button variant="ghost">Discard</Button>
      <Button variant="primary">Shortlist</Button>
    </div>
  </Surface>
);

export const AsArticle = () => (
  <Surface as="article">
    <h3 className="text-base font-semibold text-foreground">Weekly summary</h3>
    <p className="mt-2 text-sm text-muted">
      42 jobs scored this week, 9 shortlisted, 3 applications sent. Median match score rose from 61 to 68
      after the resume shape update.
    </p>
  </Surface>
);
