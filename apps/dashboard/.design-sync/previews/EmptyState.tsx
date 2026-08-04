import { Button, EmptyState } from '@job-finder/dashboard';

export const NoApplications = () => (
  <div className="max-w-md">
    <EmptyState>No applications yet. Shortlist jobs from the feed to start tracking.</EmptyState>
  </div>
);

export const WithAction = () => (
  <div className="max-w-md">
    <EmptyState>
      <p className="mb-3">
        No profile configuration found. Upload a RenderCV config or start from a blank profile to begin
        scoring jobs against your experience.
      </p>
      <Button variant="primary">Create profile</Button>
    </EmptyState>
  </div>
);
