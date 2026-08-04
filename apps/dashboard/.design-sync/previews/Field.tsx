import { Field, Input, Select, Textarea } from '@job-finder/dashboard';

export const WithInput = () => (
  <Field label="Name">
    <Input defaultValue="Senior Backend Engineer" />
  </Field>
);

export const WithSelect = () => (
  <Field label="Filter by status">
    <Select defaultValue="shortlisted">
      <option value="all">All applications</option>
      <option value="shortlisted">Shortlisted</option>
      <option value="applied">Applied</option>
      <option value="rejected">Rejected</option>
    </Select>
  </Field>
);

export const EntryForm = () => (
  <div className="grid gap-4 sm:grid-cols-2">
    <Field label="Name">
      <Input defaultValue="Acme Corp" />
    </Field>
    <Field label="Location">
      <Input defaultValue="Berlin, DE (hybrid)" />
    </Field>
    <Field label="Start date">
      <Input defaultValue="2023-04" />
    </Field>
    <Field label="End date">
      <Input placeholder="present" />
    </Field>
    <Field label="Summary" className="sm:col-span-2">
      <Textarea
        rows={3}
        defaultValue="Owned the payments ingestion pipeline; cut reconciliation lag from 40 minutes to under 2."
      />
    </Field>
  </div>
);
