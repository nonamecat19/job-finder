import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import type { SavedSearchDto } from '@job-finder/shared';
import { api } from '../api';
import { Button, Spinner, healthDot } from '../components/ui';

const CRON_PRESETS = [
  { label: 'hourly', value: '0 * * * *' },
  { label: 'every 6h', value: '0 */6 * * *' },
  { label: 'daily', value: '0 8 * * *' },
];

const CONFIG_FIELDS: Record<string, string[]> = {
  adzuna: ['appId', 'appKey', 'country'],
  djinni: ['sessionCookie'],
  jobspy: ['site'],
};

export default function SourcesPage() {
  const qc = useQueryClient();
  const { data: sources } = useQuery({ queryKey: ['sources'], queryFn: api.sources.list });
  const { data: searches } = useQuery({ queryKey: ['searches'], queryFn: api.searches.list });
  const { data: runs } = useQuery({
    queryKey: ['runs'],
    queryFn: api.searches.recentRuns,
    refetchInterval: 10_000,
  });

  const [testResult, setTestResult] = useState<Record<string, string>>({});
  const [configDrafts, setConfigDrafts] = useState<Record<string, Record<string, string>>>({});

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['sources'] });
    qc.invalidateQueries({ queryKey: ['searches'] });
  };

  const toggleSource = useMutation({
    mutationFn: (p: { key: string; enabled: boolean }) => api.sources.update(p.key, { enabled: p.enabled }),
    onSuccess: invalidate,
  });
  const saveConfig = useMutation({
    mutationFn: (p: { key: string; config: Record<string, unknown> }) =>
      api.sources.update(p.key, { config: p.config }),
    onSuccess: invalidate,
  });
  const testSource = useMutation({
    mutationFn: (key: string) => api.sources.test(key),
    onSuccess: (res, key) => {
      setTestResult((t) => ({ ...t, [key]: res.ok ? 'ok ✓' : `failed: ${res.error ?? ''}` }));
      invalidate();
    },
  });

  // saved searches
  const [newSearch, setNewSearch] = useState({
    name: '',
    keywords: '',
    location: '',
    remote: false,
    cron: '0 */6 * * *',
    sources: [] as string[],
  });
  const createSearch = useMutation({
    mutationFn: () =>
      api.searches.create({
        name: newSearch.name || newSearch.keywords,
        cron: newSearch.cron,
        query: {
          keywords: newSearch.keywords,
          location: newSearch.location || undefined,
          remote: newSearch.remote || undefined,
          sources: newSearch.sources.length ? newSearch.sources : undefined,
        },
      }),
    onSuccess: () => {
      setNewSearch({ name: '', keywords: '', location: '', remote: false, cron: '0 */6 * * *', sources: [] });
      invalidate();
    },
  });
  const runSearch = useMutation({
    mutationFn: (id: string) => api.searches.run(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['runs'] }),
  });
  const deleteSearch = useMutation({
    mutationFn: (id: string) => api.searches.remove(id),
    onSuccess: invalidate,
  });
  const toggleSearch = useMutation({
    mutationFn: (s: SavedSearchDto) => api.searches.update(s.id, { enabled: !s.enabled }),
    onSuccess: invalidate,
  });

  if (!sources) return <Spinner label="loading sources…" />;

  return (
    <div className="space-y-8">
      <section>
        <h1 className="mb-3 text-xl font-bold">Job sources</h1>
        <ul className="space-y-2">
          {sources.map((s) => {
            const fields = CONFIG_FIELDS[s.key] ?? [];
            const draft = configDrafts[s.key] ?? {};
            return (
              <li key={s.key} className="rounded-lg border border-slate-200 bg-white p-3">
                <div className="flex items-center gap-3">
                  {healthDot(s.healthy)}
                  <b className="w-24">{s.key}</b>
                  <span className="text-xs text-slate-400">{s.kind}</span>
                  <label className="ml-auto flex items-center gap-1 text-sm">
                    <input
                      type="checkbox"
                      checked={s.enabled}
                      onChange={(e) => toggleSource.mutate({ key: s.key, enabled: e.target.checked })}
                    />
                    enabled
                  </label>
                  <Button variant="secondary" onClick={() => testSource.mutate(s.key)}>
                    Test
                  </Button>
                  {testResult[s.key] && (
                    <span
                      className={`text-xs ${testResult[s.key].startsWith('ok') ? 'text-emerald-600' : 'text-rose-600'}`}
                    >
                      {testResult[s.key]}
                    </span>
                  )}
                </div>
                {fields.length > 0 && (
                  <div className="mt-2 flex flex-wrap items-end gap-2">
                    {fields.map((f) => (
                      <label key={f} className="text-xs text-slate-500">
                        {f}
                        <input
                          className="block w-44 rounded border border-slate-300 px-2 py-1 text-sm"
                          placeholder={String(s.config[f] ?? '')}
                          value={draft[f] ?? ''}
                          onChange={(e) =>
                            setConfigDrafts((d) => ({
                              ...d,
                              [s.key]: { ...d[s.key], [f]: e.target.value },
                            }))
                          }
                        />
                      </label>
                    ))}
                    <Button
                      variant="secondary"
                      disabled={Object.values(draft).every((v) => !v)}
                      onClick={() => {
                        const config: Record<string, unknown> = {};
                        for (const [k, v] of Object.entries(draft)) if (v) config[k] = v;
                        saveConfig.mutate({ key: s.key, config });
                        setConfigDrafts((d) => ({ ...d, [s.key]: {} }));
                      }}
                    >
                      save config
                    </Button>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      </section>

      <section>
        <h2 className="mb-3 text-lg font-bold">Saved searches</h2>
        <div className="mb-3 flex flex-wrap items-end gap-2 rounded-lg border border-slate-200 bg-white p-3">
          <label className="text-xs text-slate-500">
            keywords*
            <input
              className="block w-48 rounded border border-slate-300 px-2 py-1 text-sm"
              value={newSearch.keywords}
              onChange={(e) => setNewSearch({ ...newSearch, keywords: e.target.value })}
            />
          </label>
          <label className="text-xs text-slate-500">
            location
            <input
              className="block w-36 rounded border border-slate-300 px-2 py-1 text-sm"
              value={newSearch.location}
              onChange={(e) => setNewSearch({ ...newSearch, location: e.target.value })}
            />
          </label>
          <label className="flex items-center gap-1 pb-1.5 text-sm">
            <input
              type="checkbox"
              checked={newSearch.remote}
              onChange={(e) => setNewSearch({ ...newSearch, remote: e.target.checked })}
            />
            remote
          </label>
          <label className="text-xs text-slate-500">
            schedule
            <select
              className="block rounded border border-slate-300 px-2 py-1 text-sm"
              value={newSearch.cron}
              onChange={(e) => setNewSearch({ ...newSearch, cron: e.target.value })}
            >
              {CRON_PRESETS.map((c) => (
                <option key={c.value} value={c.value}>
                  {c.label}
                </option>
              ))}
            </select>
          </label>
          <div className="flex items-center gap-2 pb-1 text-xs text-slate-500">
            {sources.map((s) => (
              <label key={s.key} className="flex items-center gap-0.5">
                <input
                  type="checkbox"
                  checked={newSearch.sources.includes(s.key)}
                  onChange={(e) =>
                    setNewSearch({
                      ...newSearch,
                      sources: e.target.checked
                        ? [...newSearch.sources, s.key]
                        : newSearch.sources.filter((k) => k !== s.key),
                    })
                  }
                />
                {s.key}
              </label>
            ))}
          </div>
          <Button disabled={!newSearch.keywords} onClick={() => createSearch.mutate()}>
            + add search
          </Button>
        </div>

        <ul className="space-y-1">
          {searches?.map((s) => (
            <li
              key={s.id}
              className="flex items-center gap-3 rounded border border-slate-200 bg-white px-3 py-2 text-sm"
            >
              <b>{s.name}</b>
              <span className="text-slate-500">{s.query.keywords}</span>
              <span className="text-xs text-slate-400">
                cron {s.cron}
                {s.lastRunAt ? ` · last run ${new Date(s.lastRunAt).toLocaleString()}` : ' · never ran'}
              </span>
              <span className="ml-auto flex items-center gap-2">
                <label className="flex items-center gap-1 text-xs">
                  <input type="checkbox" checked={s.enabled} onChange={() => toggleSearch.mutate(s)} />
                  on
                </label>
                <Button variant="secondary" onClick={() => runSearch.mutate(s.id)}>
                  Run now
                </Button>
                <Button variant="ghost" onClick={() => deleteSearch.mutate(s.id)}>
                  ✕
                </Button>
              </span>
            </li>
          ))}
        </ul>
      </section>

      <section>
        <h2 className="mb-3 text-lg font-bold">Recent runs</h2>
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-slate-300 text-xs uppercase text-slate-400">
              <th className="py-1">source</th>
              <th>started</th>
              <th>status</th>
              <th>found</th>
              <th>new</th>
              <th>error</th>
            </tr>
          </thead>
          <tbody>
            {runs?.map((r) => (
              <tr key={r.id} className="border-b border-slate-100">
                <td className="py-1">{r.sourceKey}</td>
                <td>{new Date(r.startedAt).toLocaleString()}</td>
                <td>{r.ok === null ? '…' : r.ok ? '✓' : '✗'}</td>
                <td>{r.found}</td>
                <td>{r.new}</td>
                <td className="max-w-xs truncate text-rose-600">{r.error}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </div>
  );
}
