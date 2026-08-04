import { Field, Stepper } from '@job-finder/dashboard';

const noop = () => {};

export const Default = () => <Stepper value={5} onChange={noop} min={1} max={20} aria-label="results per page" />;

export const AtMin = () => <Stepper value={1} onChange={noop} min={1} max={20} aria-label="results per page" />;

export const AtMax = () => <Stepper value={20} onChange={noop} min={1} max={20} aria-label="results per page" />;

export const Labelled = () => (
  <Field label="Jobs per batch">
    <Stepper value={12} onChange={noop} min={1} max={50} step={1} aria-label="jobs per batch" />
  </Field>
);
