import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { api, JobFilters } from '../api';
import { Button, Chip, ScoreBadge, Spinner } from '../components/ui';

export default function FeedPage() {
  const [filters, setFilters] = useState<JobFilters>({ sort: 'score', page: 1 });
  const qc = useQueryClient();

  const { data, isLoading, error } = useQuery({
    queryKey: ['jobs', filters],
    queryFn: () => api.jobs.list(filters),
  });
  const { data: sources } = useQuery({ queryKey: ['sources'], queryFn: api.sources.list });

  const shortlist = useMutation({
    mutationFn: (id: string) => api.jobs.shortlist(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['jobs'] }),
  });
  const hide = useMutation({
    mutationFn: (id: string) => api.jobs.hide(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['jobs'] }),
  });

  const set = (patch: Partial<JobFilters>) => setFilters((f) => ({ ...f, ...patch, page: 1 }));

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <input
          className="w-56 rounded border border-slate-300 px-2 py-1.5 text-sm"
          placeholder="Search title/company…"
          value={filters.q ?? ''}
          onChange={(e) => set({ q: e.target.value || undefined })}
        />
        <select
          className="rounded border border-slate-300 px-2 py-1.5 text-sm"
          value={filters.source ?? ''}
          onChange={(e) => set({ source: e.target.value || undefined })}
        >
          <option value="">all sources</option>
          {sources?.map((s) => (
            <option key={s.key} value={s.key}>
              {s.key}
            </option>
          ))}
        </select>
        <select
          className="rounded border border-slate-300 px-2 py-1.5 text-sm"
          value={filters.minScore ?? ''}
          onChange={(e) => set({ minScore: e.target.value ? Number(e.target.value) : undefined })}
        >
          <option value="">any score</option>
          <option value="80">80+</option>
          <option value="60">60+</option>
          <option value="40">40+</option>
        </select>
        <label className="flex items-center gap-1 text-sm text-slate-600">
          <input
            type="checkbox"
            checked={filters.remote ?? false}
            onChange={(e) => set({ remote: e.target.checked ? true : undefined })}
          />
          remote
        </label>
        <select
          className="rounded border border-slate-300 px-2 py-1.5 text-sm"
          value={filters.sort ?? 'score'}
          onChange={(e) => set({ sort: e.target.value as 'score' | 'date' })}
        >
          <option value="score">by fit score</option>
          <option value="date">by date</option>
        </select>
      </div>

      {isLoading && <Spinner label="loading jobs…" />}
      {error && <p className="text-sm text-rose-600">{(error as Error).message}</p>}
      {data && data.items.length === 0 && (
        <p className="text-sm text-slate-500">
          No jobs yet. Add a saved search on the Sources page and hit “Run now”.
        </p>
      )}

      <ul className="space-y-2">
        {data?.items.map((job) => (
          <li key={job.id} className="rounded-lg border border-slate-200 bg-white p-3 shadow-sm">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <ScoreBadge score={job.matchResult?.score} />
                  <Link to={`/jobs/${job.id}`} className="truncate font-medium text-sky-700 hover:underline">
                    {job.title}
                  </Link>
                </div>
                <div className="mt-0.5 text-sm text-slate-600">
                  {job.company}
                  {job.location ? ` · ${job.location}` : ''}
                  {job.remote ? ' · remote' : ''}
                  {job.salaryRaw ? ` · ${job.salaryRaw}` : ''}
                </div>
                <div className="mt-1.5 flex flex-wrap gap-1">
                  <Chip>{job.sourceKey}</Chip>
                  {(job.matchResult?.matchedSkills ?? []).slice(0, 3).map((s) => (
                    <Chip key={s} tone="green">
                      {s}
                    </Chip>
                  ))}
                  {(job.matchResult?.missingSkills ?? []).slice(0, 3).map((s) => (
                    <Chip key={s} tone="red">
                      {s}
                    </Chip>
                  ))}
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button variant="secondary" onClick={() => shortlist.mutate(job.id)}>
                  ★ shortlist
                </Button>
                <Button variant="ghost" onClick={() => hide.mutate(job.id)}>
                  hide
                </Button>
                <a
                  href={job.url}
                  target="_blank"
                  rel="noreferrer"
                  className="rounded px-2 py-1.5 text-sm text-slate-500 hover:bg-slate-100"
                >
                  ↗
                </a>
              </div>
            </div>
          </li>
        ))}
      </ul>

      {data && data.total > data.pageSize && (
        <div className="mt-4 flex items-center gap-3 text-sm">
          <Button
            variant="secondary"
            disabled={(filters.page ?? 1) <= 1}
            onClick={() => setFilters((f) => ({ ...f, page: (f.page ?? 1) - 1 }))}
          >
            ← prev
          </Button>
          <span>
            page {data.page} / {Math.ceil(data.total / data.pageSize)} ({data.total} jobs)
          </span>
          <Button
            variant="secondary"
            disabled={data.page * data.pageSize >= data.total}
            onClick={() => setFilters((f) => ({ ...f, page: (f.page ?? 1) + 1 }))}
          >
            next →
          </Button>
        </div>
      )}
    </div>
  );
}
