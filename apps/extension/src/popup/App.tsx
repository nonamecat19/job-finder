import { useCallback, useEffect, useState } from 'react';

import {
  sendToBackground,
  sendToTab,
  type Capabilities,
  type DocumentSummary,
  type FillTarget,
} from '@/shared/messages';
import type { Result } from '@/shared/result';

import {
  blockedReason,
  canAttachFile,
  canPasteLetter,
  describeFill,
  formatDocLabel,
  shouldOfferOpenForm,
  splitDocuments,
  type PopupState,
} from './state';

export function App() {
  const [tabId, setTabId] = useState<number | null>(null);
  const [state, setState] = useState<PopupState>({ phase: 'loading' });
  const [busy, setBusy] = useState(false);
  const [pickedResume, setPickedResume] = useState<string | null>(null);
  const [pickedLetter, setPickedLetter] = useState<string | null>(null);

  const load = useCallback(async () => {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (!tab?.id || !tab.url) {
      setState({ phase: 'unsupported', host: '' });
      return;
    }
    setTabId(tab.id);

    const probe = await sendToTab(tab.id, { kind: 'cs/probe' }).catch(() => undefined);
    const caps = probe && probe.ok && probe.value.kind === 'cs/capabilities' ? probe.value.caps : null;
    if (!caps || caps.adapter === null) {
      setState({ phase: 'unsupported', host: new URL(tab.url).hostname });
      return;
    }

    const job = await sendToBackground({ kind: 'bg/resolveJob', url: tab.url, hints: caps.hints });
    if (!job.ok) {
      setState(job.error.code === 'not_found' ? { phase: 'not_found', url: tab.url } : { phase: 'error', error: job.error });
      return;
    }
    if (job.value.kind !== 'bg/job') return;

    const { resumes, letters } = splitDocuments(job.value.documents);
    setPickedResume(resumes[0]?.id ?? null);
    setPickedLetter(letters.find((d) => d.hasText)?.id ?? null);
    setState({ phase: 'ready', job: job.value.job, resumes, letters, caps });
  }, []);

  useEffect(() => {

    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load();
  }, [load]);

  const reprobe = useCallback(async (id: number, caps: Capabilities): Promise<Capabilities> => {
    const probe = await sendToTab(id, { kind: 'cs/probe' }).catch(() => undefined);
    return probe && probe.ok && probe.value.kind === 'cs/capabilities' ? probe.value.caps : caps;
  }, []);

  const openForm = useCallback(async () => {
    if (state.phase !== 'ready' || tabId === null) return;
    setBusy(true);
    const res = (await sendToTab(tabId, { kind: 'cs/openApplyForm' }).catch(() => undefined)) as
      | Result<{ kind: string; caps?: Capabilities }>
      | undefined;
    setBusy(false);
    if (!res) return;
    if (!res.ok) {
      setState({ ...state, notice: res.error.message });
      return;
    }
    setState({ ...state, caps: await reprobe(tabId, state.caps), notice: undefined });
  }, [state, tabId, reprobe]);

  const fill = useCallback(
    async (target: FillTarget) => {
      if (state.phase !== 'ready' || tabId === null) return;
      const documentId = target === 'letter' ? pickedLetter : pickedResume;
      if (!documentId) return;
      setBusy(true);
      const res = await sendToBackground({ kind: 'bg/fill', tabId, documentId, target });
      setBusy(false);
      if (!res.ok) {
        setState({ ...state, notice: res.error.message });
        return;
      }
      if (res.value.kind !== 'bg/filled') return;
      setState({ ...state, report: res.value.report, notice: undefined });
    },
    [state, tabId, pickedResume, pickedLetter],
  );

  const addVacancy = useCallback(async () => {
    if (state.phase !== 'not_found') return;
    setBusy(true);
    const res = await sendToBackground({ kind: 'bg/addVacancy', url: state.url });
    setBusy(false);
    if (!res.ok) {
      setState({ phase: 'error', error: res.error });
      return;
    }
    if (res.value.kind !== 'bg/addResult') return;
    if (res.value.outcome === 'duplicate') {
      void load();
      return;
    }
    const message =
      res.value.outcome === 'created'
        ? 'Added to job-finder. Generate the documents in the dashboard, then come back.'
        : res.value.outcome === 'needs_fill_in'
          ? "job-finder couldn't read this vacancy automatically — finish adding it in the dashboard."
          : (res.value.reason ?? 'Adding the vacancy failed.');
    setState({ phase: 'error', error: { code: 'bad_request', message } });
  }, [state, load]);

  if (state.phase === 'loading') return <main className="app">Loading…</main>;

  if (state.phase === 'unsupported') {
    return (
      <main className="app">
        <p className="muted">Open a vacancy on djinni.co, jobs.dou.ua or work.ua to use this.</p>
        <OptionsLink />
      </main>
    );
  }

  if (state.phase === 'not_found') {
    return (
      <main className="app">
        <p>This vacancy isn&apos;t in job-finder yet.</p>
        <button className="primary" onClick={addVacancy} disabled={busy}>
          Add to job-finder
        </button>
        <p className="muted hint">Adding only creates the vacancy — documents are still generated in the dashboard.</p>
        <OptionsLink />
      </main>
    );
  }

  if (state.phase === 'error') {
    return (
      <main className="app">
        <p className="error">{state.error.message}</p>
        {state.error.detail ? <details><summary>Details</summary><pre>{state.error.detail}</pre></details> : null}
        <button
          onClick={() => {
            setState({ phase: 'loading' });
            void load();
          }}
        >
          Retry
        </button>
        <OptionsLink />
      </main>
    );
  }

  const { job, resumes, letters, caps, report, notice } = state;
  const blocked = blockedReason(caps, resumes, letters);

  return (
    <main className="app">
      <div>
        <div className="job-title">{job.title}</div>
        <div className="muted">{job.company}</div>
      </div>

      {resumes.length === 0 && letters.length === 0 ? (
        <p className="muted">No documents generated for this vacancy yet.</p>
      ) : null}

      <DocPicker label="CV" docs={resumes} picked={pickedResume} onPick={setPickedResume} />
      <DocPicker label="Cover letter" docs={letters} picked={pickedLetter} onPick={setPickedLetter} />

      {blocked ? <p className="muted hint">{blocked}</p> : null}
      {notice ? <p className="error">{notice}</p> : null}
      {report ? <p className="ok">{describeFill(report)}</p> : null}
      {report?.warnings.map((w) => (
        <p key={w} className="muted hint">
          {w}
        </p>
      ))}

      <div className="actions">
        {shouldOfferOpenForm(caps) ? (
          <button className="primary" onClick={openForm} disabled={busy}>
            Open apply form
          </button>
        ) : null}
        <button onClick={() => void fill('file')} disabled={busy || !canAttachFile(caps, resumes) || !pickedResume}>
          Attach CV
        </button>
        <button onClick={() => void fill('letter')} disabled={busy || !canPasteLetter(caps, letters) || !pickedLetter}>
          Paste letter
        </button>
      </div>

      <p className="muted hint">Nothing is submitted — press Send on the site yourself.</p>
      <OptionsLink />
    </main>
  );
}

function DocPicker({
  label,
  docs,
  picked,
  onPick,
}: {
  label: string;
  docs: DocumentSummary[];
  picked: string | null;
  onPick: (id: string) => void;
}) {
  if (docs.length === 0) return null;
  const [newest, ...older] = docs;
  return (
    <section>
      <div className="section-label">{label}</div>
      <DocRow doc={newest} picked={picked === newest.id} onPick={onPick} />
      {older.length > 0 ? (
        <details>
          <summary>{older.length} older version{older.length > 1 ? 's' : ''}</summary>
          {older.map((doc) => (
            <DocRow key={doc.id} doc={doc} picked={picked === doc.id} onPick={onPick} />
          ))}
        </details>
      ) : null}
    </section>
  );
}

function DocRow({ doc, picked, onPick }: { doc: DocumentSummary; picked: boolean; onPick: (id: string) => void }) {
  return (
    <div className={picked ? 'row selected' : 'row'}>
      <span className="doc-meta">
        <span>{formatDocLabel(doc)}</span>
        <span className="muted">{[doc.company, doc.title].filter(Boolean).join(' · ')}</span>
      </span>
      <button onClick={() => onPick(doc.id)} disabled={picked}>
        {picked ? 'Selected' : 'Select'}
      </button>
    </div>
  );
}

function OptionsLink() {
  return (
    <button onClick={() => chrome.runtime.openOptionsPage()} className="muted">
      Settings
    </button>
  );
}
