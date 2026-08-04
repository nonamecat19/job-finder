import { Button } from '@job-finder/dashboard';

export const Variants = () => (
  <div className="flex flex-wrap items-center gap-2">
    <Button variant="primary">Shortlist job</Button>
    <Button variant="secondary">Open posting</Button>
    <Button variant="danger">Discard</Button>
    <Button variant="ghost">Cancel</Button>
  </div>
);

export const Disabled = () => (
  <div className="flex flex-wrap items-center gap-2">
    <Button variant="primary" disabled>
      Shortlist job
    </Button>
    <Button variant="secondary" disabled>
      Open posting
    </Button>
    <Button variant="danger" disabled>
      Discard
    </Button>
    <Button variant="ghost" disabled>
      Cancel
    </Button>
  </div>
);

export const InAForm = () => (
  <div className="flex items-center justify-end gap-2">
    <Button variant="ghost">Cancel</Button>
    <Button variant="primary" type="submit">
      Save profile
    </Button>
  </div>
);
