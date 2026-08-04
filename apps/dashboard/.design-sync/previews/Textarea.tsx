import { Field, Textarea } from '@job-finder/dashboard';

export const Filled = () => (
  <Textarea
    rows={4}
    className="max-w-md"
    defaultValue="Owned the payments ingestion pipeline; cut reconciliation lag from 40 minutes to under 2. Led the migration from a cron-driven batch job to a streaming consumer."
  />
);

export const Placeholder = () => (
  <Textarea rows={4} className="max-w-md" placeholder="Paste the job description here…" />
);

export const Labelled = () => (
  <div className="max-w-md">
    <Field label="Summary">
      <Textarea rows={3} defaultValue="Backend engineer, 8 years, Go and distributed systems." />
    </Field>
  </div>
);
