import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import type { JsonResume } from '@job-finder/shared';
import { api } from '../api';
import { Button, Spinner } from '../components/ui';

export default function ProfilePage() {
  const qc = useQueryClient();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [docText, setDocText] = useState('{}');
  const [extraNotes, setExtraNotes] = useState('');
  const [draft, setDraft] = useState<JsonResume | null>(null);
  const [jsonError, setJsonError] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const { data: profiles, isLoading } = useQuery({ queryKey: ['profiles'], queryFn: api.profiles.list });

  const selected = profiles?.find((p) => p.id === selectedId) ?? profiles?.[0];

  useEffect(() => {
    if (selected) {
      setSelectedId(selected.id);
      setName(selected.name);
      setDocText(JSON.stringify(selected.document, null, 2));
      setExtraNotes(selected.extraNotes ?? '');
      setJsonError(null);
    }
  }, [selected?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const save = useMutation({
    mutationFn: async () => {
      let document: JsonResume;
      try {
        document = JSON.parse(docText);
        setJsonError(null);
      } catch (e) {
        setJsonError((e as Error).message);
        throw e;
      }
      if (selected && selectedId) {
        return api.profiles.update(selectedId, { name, document, extraNotes: extraNotes || null });
      }
      return api.profiles.create({ name: name || 'My profile', document, extraNotes: extraNotes || undefined });
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['profiles'] }),
  });

  const importPdf = useMutation({
    mutationFn: (file: File) => api.profiles.import(file),
    onSuccess: (res) => setDraft(res.draft),
  });

  const newProfile = () => {
    setSelectedId('new');
    setName('');
    setDocText(JSON.stringify({ basics: { name: '' }, work: [], skills: [], education: [] }, null, 2));
    setExtraNotes('');
    setDraft(null);
  };

  if (isLoading) return <Spinner label="loading profiles…" />;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h1 className="text-xl font-bold">Master profile</h1>
        <select
          className="rounded border border-slate-300 px-2 py-1.5 text-sm"
          value={selectedId ?? ''}
          onChange={(e) => setSelectedId(e.target.value)}
        >
          {profiles?.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
          {selectedId === 'new' && <option value="new">(new profile)</option>}
        </select>
        <Button variant="secondary" onClick={newProfile}>
          + new
        </Button>
        <div className="ml-auto flex items-center gap-2">
          <input
            ref={fileRef}
            type="file"
            accept="application/pdf"
            className="hidden"
            onChange={(e) => e.target.files?.[0] && importPdf.mutate(e.target.files[0])}
          />
          <Button variant="secondary" onClick={() => fileRef.current?.click()}>
            Import resume PDF
          </Button>
          {importPdf.isPending && <Spinner label="parsing with LLM…" />}
        </div>
      </div>

      {draft && (
        <div className="rounded-lg border border-amber-300 bg-amber-50 p-3">
          <p className="mb-2 text-sm font-medium text-amber-800">
            Parsed draft from PDF — review, then merge into the editor:
          </p>
          <pre className="max-h-64 overflow-auto rounded bg-white p-2 text-xs">
            {JSON.stringify(draft, null, 2)}
          </pre>
          <div className="mt-2 flex gap-2">
            <Button
              onClick={() => {
                setDocText(JSON.stringify(draft, null, 2));
                setDraft(null);
              }}
            >
              Use as document
            </Button>
            <Button variant="ghost" onClick={() => setDraft(null)}>
              discard
            </Button>
          </div>
        </div>
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600">Profile name</label>
          <input
            className="mb-3 w-full rounded border border-slate-300 px-2 py-1.5 text-sm"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Backend profile"
          />
          <label className="mb-1 block text-sm font-medium text-slate-600">
            Document (JSON Resume format)
          </label>
          <textarea
            className="h-96 w-full rounded border border-slate-300 p-2 font-mono text-xs"
            value={docText}
            onChange={(e) => setDocText(e.target.value)}
            spellCheck={false}
          />
          {jsonError && <p className="text-sm text-rose-600">invalid JSON: {jsonError}</p>}
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600">
            Extra notes (free-form knowledge the LLM may use: preferences, war stories, context)
          </label>
          <textarea
            className="h-[28rem] w-full rounded border border-slate-300 p-2 text-sm"
            value={extraNotes}
            onChange={(e) => setExtraNotes(e.target.value)}
          />
        </div>
      </div>

      <div className="flex items-center gap-3">
        <Button onClick={() => save.mutate()} disabled={save.isPending}>
          {save.isPending ? 'saving…' : 'Save profile'}
        </Button>
        {save.isSuccess && <span className="text-sm text-emerald-600">saved ✓ (embedding refreshed)</span>}
        {save.isError && !jsonError && (
          <span className="text-sm text-rose-600">{(save.error as Error).message}</span>
        )}
      </div>
    </div>
  );
}
