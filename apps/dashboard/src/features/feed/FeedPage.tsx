import { ExternalLink, EyeOff, Star } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import type { JobDto } from '@job-finder/shared';
import { type JobFilters } from '../../api';
import { PageHeader } from '../../components/layout/PageHeader';
import {
  Button,
  Checkbox,
  Chip,
  EmptyState,
  ErrorState,
  Field,
  Input,
  ScoreBadge,
  Select,
  Spinner,
  Surface,
} from '../../components/ui';
import { useFeedSources, useHideJob, useJobs, useShortlistJob } from './hooks';

export default function FeedPage() {
  const [filters, setFilters] = useState<JobFilters>({ sort: 'score', page: 1 });
  const { data, isLoading, error } = useJobs(filters);
  const { data: sources } = useFeedSources();
  const shortlist = useShortlistJob();
  const hide = useHideJob();

  const set = (patch: Partial<JobFilters>) => setFilters((f) => ({ ...f, ...patch, page: 1 }));

  return (
    <div>
      <PageHeader
        title="Job feed"
        description="Review fresh matches, filter by source or fit, and move promising roles into the tracker."
      />

      <Surface className="mb-4">
        <div className="grid gap-3 md:grid-cols-[minmax(14rem,1.4fr)_repeat(3,minmax(9rem,0.7fr))_auto] md:items-end">
          <Field label="Search">
            <Input
              placeholder="Search title/company…"
              value={filters.q ?? ''}
              onChange={(e) => set({ q: e.target.value || undefined })}
            />
          </Field>
          <Field label="Source">
            <Select
              value={filters.source ?? ''}
              onChange={(e) => set({ source: e.target.value || undefined })}
              className="w-full"
            >
              <option value="">all sources</option>
              {sources?.map((s) => (
                <option key={s.key} value={s.key}>
                  {s.key}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Fit score">
            <Select
              value={filters.minScore ?? ''}
              onChange={(e) => set({ minScore: e.target.value ? Number(e.target.value) : undefined })}
              className="w-full"
            >
              <option value="">any score</option>
              <option value="80">80+</option>
              <option value="60">60+</option>
              <option value="40">40+</option>
            </Select>
          </Field>
          <Field label="Sort">
            <Select
              value={filters.sort ?? 'score'}
              onChange={(e) => set({ sort: e.target.value as 'score' | 'date' })}
              className="w-full"
            >
              <option value="score">by fit score</option>
              <option value="date">by date</option>
            </Select>
          </Field>
          <label className="flex items-center gap-2 pb-2 text-sm font-medium text-muted">
            <Checkbox
              checked={filters.remote ?? false}
              onChange={(e) => set({ remote: e.target.checked ? true : undefined })}
            />
            remote
          </label>
        </div>
      </Surface>

      {isLoading ? <Spinner label="loading jobs…" /> : null}
      {error ? <ErrorState error={error} /> : null}
      {data && data.items.length === 0 ? (
        <EmptyState>No jobs yet. Add a saved search on the Sources page and hit "Run now".</EmptyState>
      ) : null}

      <ul className="space-y-3">
        {data?.items.map((job) => (
          <JobCard
            key={job.id}
            job={job}
            onShortlist={() => shortlist.mutate(job.id)}
            onHide={() => hide.mutate(job.id)}
          />
        ))}
      </ul>

      {data && data.total > data.pageSize ? (
        <Pagination
          page={data.page}
          total={data.total}
          pageSize={data.pageSize}
          currentPage={filters.page ?? 1}
          onPrev={() => setFilters((f) => ({ ...f, page: (f.page ?? 1) - 1 }))}
          onNext={() => setFilters((f) => ({ ...f, page: (f.page ?? 1) + 1 }))}
        />
      ) : null}
    </div>
  );
}

function JobCard({
  job,
  onShortlist,
  onHide,
}: {
  job: JobDto;
  onShortlist: () => void;
  onHide: () => void;
}) {
  return (
    <li className="group rounded-xl border border-border bg-surface p-4 shadow-sm shadow-black/20 transition hover:border-primary/40 hover:bg-elevated">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <ScoreBadge score={job.matchResult?.score} />
            <Link to={`/jobs/${job.id}`} className="truncate font-semibold text-primary hover:underline">
              {job.title}
            </Link>
          </div>
          <div className="mt-1 text-sm text-muted">
            {job.company}
            {job.location ? ` · ${job.location}` : ''}
            {job.remote ? ' · remote' : ''}
            {job.salaryRaw ? ` · ${job.salaryRaw}` : ''}
          </div>
          <div className="mt-2 flex flex-wrap gap-1">
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
        <div className="flex shrink-0 flex-wrap items-center gap-1">
          <Button variant="secondary" onClick={onShortlist}>
            <Star className="h-4 w-4" aria-hidden="true" />
            shortlist
          </Button>
          <Button variant="ghost" onClick={onHide}>
            <EyeOff className="h-4 w-4" aria-hidden="true" />
            hide
          </Button>
          <a
            href={job.url}
            target="_blank"
            rel="noreferrer"
            className="inline-flex rounded-md px-2 py-1.5 text-sm text-muted hover:bg-overlay"
            aria-label={`Open ${job.title}`}
          >
            <ExternalLink className="h-4 w-4" aria-hidden="true" />
          </a>
        </div>
      </div>
    </li>
  );
}

function Pagination({
  page,
  total,
  pageSize,
  currentPage,
  onPrev,
  onNext,
}: {
  page: number;
  total: number;
  pageSize: number;
  currentPage: number;
  onPrev: () => void;
  onNext: () => void;
}) {
  return (
    <div className="mt-4 flex flex-wrap items-center gap-3 text-sm text-muted">
      <Button variant="secondary" disabled={currentPage <= 1} onClick={onPrev}>
        prev
      </Button>
      <span>
        page {page} / {Math.ceil(total / pageSize)} ({total} jobs)
      </span>
      <Button variant="secondary" disabled={page * pageSize >= total} onClick={onNext}>
        next
      </Button>
    </div>
  );
}
