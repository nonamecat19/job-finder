import { ErrorState } from '@job-finder/dashboard';

export const FromError = () => (
  <div className="max-w-md">
    <ErrorState error={new Error('Failed to load applications: gateway timed out after 30s')} />
  </div>
);

export const FromString = () => (
  <div className="max-w-md">
    <ErrorState error="Resume generation failed — the LLM returned an unparseable shape." />
  </div>
);
