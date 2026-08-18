import { Surface, Switch } from '@job-finder/dashboard';

const noop = () => {};

export const States = () => (
  <div className="flex items-center gap-6">
    <div className="flex items-center gap-2 text-sm text-foreground">
      <Switch checked onChange={noop} label="On" />
      On
    </div>
    <div className="flex items-center gap-2 text-sm text-foreground">
      <Switch checked={false} onChange={noop} label="Off" />
      Off
    </div>
  </div>
);

export const SettingsRow = () => (
  <Surface>
    <div className="flex items-center justify-between gap-4">
      <div>
        <p className="mb-1 text-sm font-semibold text-foreground">Dark theme</p>
        <p className="text-sm text-muted">Switch the dashboard between light and dark appearance.</p>
      </div>
      <Switch checked onChange={noop} label="Dark theme" />
    </div>
    <div className="mt-4 flex items-center justify-between gap-4 border-t border-border pt-4">
      <div>
        <p className="mb-1 text-sm font-semibold text-foreground">Dark resume preview</p>
        <p className="text-sm text-muted">Show resume PDFs on a dark sheet with light text.</p>
      </div>
      <Switch checked={false} onChange={noop} label="Dark resume preview" />
    </div>
  </Surface>
);
