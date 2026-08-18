import { StrictMode, useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';

import { sendToBackground } from '@/shared/messages';

import '../popup/popup.css';

function Options() {
  const [apiBaseUrl, setApiBaseUrl] = useState('http://localhost:3000');
  const [debug, setDebug] = useState(false);
  const [status, setStatus] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    void (async () => {
      const res = await sendToBackground({ kind: 'bg/getSettings' });
      if (res.ok && res.value.kind === 'bg/settings') {
        setApiBaseUrl(res.value.settings.apiBaseUrl);
        setDebug(res.value.settings.debug);
      }
    })();
  }, []);

  async function save() {
    setStatus(null);
    setError(null);

    const origin = originOf(apiBaseUrl);
    if (origin && !(await chrome.permissions.contains({ origins: [origin] }))) {
      const granted = await chrome.permissions.request({ origins: [origin] });
      if (!granted) {
        setError(`Permission for ${origin} was declined, so job-finder can't be reached there.`);
        return;
      }
    }
    const res = await sendToBackground({ kind: 'bg/setSettings', settings: { apiBaseUrl, debug } });
    if (!res.ok) {
      setError(res.error.message);
      return;
    }
    setStatus('Saved.');
  }

  async function test() {
    setStatus(null);
    setError(null);
    const res = await sendToBackground({ kind: 'bg/ping', apiBaseUrl });
    if (res.ok) setStatus('Connected.');
    else setError(res.error.message);
  }

  return (
    <main className="app" style={{ width: 420 }}>
      <h1 style={{ fontSize: 16, margin: 0 }}>job-finder settings</h1>
      <label>
        <div className="section-label">API base URL</div>
        <input
          value={apiBaseUrl}
          onChange={(e) => setApiBaseUrl(e.target.value)}
          style={{ width: '100%', padding: 6 }}
          spellCheck={false}
        />
      </label>
      <label style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
        <input type="checkbox" checked={debug} onChange={(e) => setDebug(e.target.checked)} />
        Log debug detail to the console
      </label>
      <div className="actions">
        <button className="primary" onClick={() => void save()}>
          Save
        </button>
        <button onClick={() => void test()}>Test connection</button>
      </div>
      {status ? <p className="ok">{status}</p> : null}
      {error ? <p className="error">{error}</p> : null}
    </main>
  );
}

function originOf(raw: string): string | null {
  try {
    return `${new URL(raw).origin}/*`;
  } catch {
    return null;
  }
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Options />
  </StrictMode>,
);
