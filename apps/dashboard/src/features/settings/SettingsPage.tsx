import { useEffect, useState } from 'react';
import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import { Button, ErrorState, Surface } from '../../components/ui';
import LlmSettingsCard from './LlmSettingsCard';
import { useBootstrapExtension } from './hooks';

// SettingsPage hosts the browser-extension pairing flow (spec
// 014-autofill-extension section 2.1 step 2): generate a one-time,
// 5-minute bootstrap code here, then paste it into the extension popup.
// The dashboard never sees the extension's access/refresh tokens — this
// page's only job is minting and displaying the short-lived code.
export default function SettingsPage() {
  return (
    <div>
      <PageHeader
        title="Settings"
        description="Connect the browser extension and manage account-level preferences."
      />
      <section>
        <SectionTitle>AI models</SectionTitle>
        <LlmSettingsCard />
      </section>
      <section>
        <SectionTitle>Browser extension</SectionTitle>
        <ExtensionPairingCard />
      </section>
    </div>
  );
}

function ExtensionPairingCard() {
  const bootstrap = useBootstrapExtension();
  const [secondsLeft, setSecondsLeft] = useState<number | null>(null);

  useEffect(() => {
    if (!bootstrap.data) {
      setSecondsLeft(null);
      return;
    }
    const expiresAt = new Date(bootstrap.data.expiresAt).getTime();
    const tick = () => setSecondsLeft(Math.max(0, Math.round((expiresAt - Date.now()) / 1000)));
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [bootstrap.data]);

  const expired = secondsLeft === 0;

  return (
    <Surface>
      <p className="text-sm text-muted">
        Generate a one-time code, then open the job-finder extension popup and paste it in. The
        code is valid for 5 minutes and can only be used once.
      </p>

      <div className="mt-4">
        <Button onClick={() => bootstrap.mutate()} disabled={bootstrap.isPending}>
          {bootstrap.isPending ? 'Generating…' : 'Generate pairing code'}
        </Button>
      </div>

      {bootstrap.error ? (
        <div className="mt-4">
          <ErrorState error={bootstrap.error} />
        </div>
      ) : null}

      {bootstrap.data ? (
        <div className="mt-5 rounded-lg border border-border bg-overlay p-4">
          <p className="text-xs font-semibold uppercase tracking-wide text-faint">Pairing code</p>
          <p
            data-testid="bootstrap-code"
            className="mt-1 select-all break-all font-mono text-2xl font-black tracking-widest text-fg"
          >
            {bootstrap.data.code}
          </p>
          <p className="mt-2 text-xs text-muted">
            {expired ? (
              <span className="font-semibold text-danger">Expired — generate a new code.</span>
            ) : (
              <>Expires in {secondsLeft ?? '…'}s</>
            )}
          </p>
        </div>
      ) : null}
    </Surface>
  );
}
