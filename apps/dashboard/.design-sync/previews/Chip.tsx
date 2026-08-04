import { Chip } from '@job-finder/dashboard';

export const Tones = () => (
  <div className="flex flex-wrap items-center gap-2">
    <Chip tone="green">Matched</Chip>
    <Chip tone="red">Missing</Chip>
    <Chip tone="slate">Neutral</Chip>
  </div>
);

export const KeywordDiff = () => (
  <div className="flex flex-wrap items-center gap-1.5">
    <Chip tone="green">Go</Chip>
    <Chip tone="green">PostgreSQL</Chip>
    <Chip tone="green">Kafka</Chip>
    <Chip tone="red">Kubernetes</Chip>
    <Chip tone="red">Terraform</Chip>
    <Chip tone="slate">gRPC</Chip>
  </div>
);
