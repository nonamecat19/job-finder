import { Field, Input } from '@job-finder/dashboard';

export const Filled = () => <Input defaultValue="Senior Backend Engineer" className="max-w-sm" />;

export const Placeholder = () => <Input placeholder="Search job titles…" className="max-w-sm" />;

export const Disabled = () => <Input defaultValue="Locked value" disabled className="max-w-sm" />;

export const Labelled = () => (
  <div className="max-w-sm">
    <Field label="Company">
      <Input defaultValue="Acme Corp" />
    </Field>
  </div>
);
