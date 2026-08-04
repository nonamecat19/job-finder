import { Field, Select } from '@job-finder/dashboard';

export const Statuses = () => (
  <Select defaultValue="shortlisted" className="max-w-sm">
    <option value="all">All applications</option>
    <option value="shortlisted">Shortlisted</option>
    <option value="applied">Applied</option>
    <option value="interviewing">Interviewing</option>
    <option value="rejected">Rejected</option>
  </Select>
);

export const Disabled = () => (
  <Select defaultValue="applied" disabled className="max-w-sm">
    <option value="applied">Applied</option>
  </Select>
);

export const Labelled = () => (
  <div className="max-w-sm">
    <Field label="Filter by status">
      <Select defaultValue="applied">
        <option value="all">All applications</option>
        <option value="applied">Applied</option>
        <option value="rejected">Rejected</option>
      </Select>
    </Field>
  </div>
);
