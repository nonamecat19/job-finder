import { Clock, ExternalLink, EyeOff, Star, Trash2 } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import type { JobDto } from '@job-finder/shared';
import { type JobFilters } from '../../lib/api';
import { PageHeader } from '../../components/layout/PageHeader';
import { VirtualList } from '../../components/VirtualList';
import {
  Button,
  Checkbox,
  Chip,
  EmptyState,
  ErrorState,
  Field,
  GhostBadge,
  Input,
  LoadingRegion,
  ScoreBadge,
  Select,
  SkeletonBlock,
  SkeletonLine,
  Surface,
} from '../../components/ui';
import {
  useClearJobs,
  useFeedSources,
  useFeedSubscriptions,
  useHideJob,
  useJobs,
  useShortlistJob,
} from './hooks';
import { postAgeLabel } from '../../lib/time';

function filtersFromParams(params: URLSearchParams): JobFilters {
  return {
    sort: params.get('sort') === 'date' ? 'date' : 'score',
    source: params.get('source') ?? undefined,
    subscriptionId: params.get('subscriptionId') ?? undefined,
    minScore: params.get('minScore') ? Number(params.get('minScore')) : undefined,
    remote: params.get('remote') === 'true' ? true : undefined,
    q: params.get('q') ?? undefined,
    page: params.get('page') ? Number(params.get('page')) : 1,
    showBelowFloor: params.get('showBelowFloor') === 'true' ? true : undefined,
  };
}

function paramsFromFilters(filters: JobFilters): URLSearchParams {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(filters)) {
    if (v === undefined || v === '' || v === null) continue;
    if (k === 'sort' && v === 'score') continue;
    if (k === 'page' && v === 1) continue;
    params.set(k, String(v));
  }
  return params;
}

export default function FeedPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = useMemo(() => filtersFromParams(searchParams), [searchParams]);
  const { data, isLoading, error } = useJobs(filters);
  const { data: sources } = useFeedSources();
  const { data: subscriptions } = useFeedSubscriptions();
  const shortlist = useShortlistJob();
  const hide = useHideJob();
  const clear = useClearJobs();

  const set = (patch: Partial<JobFilters>) =>
    setSearchParams(paramsFromFilters({ ...filters, ...patch, page: 1 }), { replace: true });

  const setPage = (page: number) => setSearchParams(paramsFromFilters({ ...filters, page }), { replace: true });

  const [searchInput, setSearchInput] = useState(filters.q ?? '');
  useEffect(() => {
    setSearchInput(filters.q ?? '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters.q]);
  useEffect(() => {
    if (searchInput === (filters.q ?? '')) return;
    const id = setTimeout(() => set({ q: searchInput || undefined }), 300);
    return () => clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchInput]);

  const onClearAll = () => {
    if (window.confirm('Delete ALL vacancies and their match results, documents, and applications? This cannot be undone.')) {
      clear.mutate();
    }
  };

  return (
    <div>
      <PageHeader
        title="Job feed"
        description="Review fresh matches, filter by source or fit, and move promising roles into the tracker."
        actions={
          <Button variant="danger" onClick={onClearAll} disabled={clear.isPending}>
            <Trash2 className="h-4 w-4" aria-hidden="true" />
            {clear.isPending ? 'clearing…' : 'clear all'}
          </Button>
        }
      />

      <Surface className="mb-4">
        <div className="grid gap-3 md:grid-cols-[minmax(14rem,1.4fr)_repeat(4,minmax(9rem,0.7fr))_auto] md:items-end">
          <Field label="Search">
            <Input
              placeholder="Search title/company…"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
            />
          </Field>
          <Field label="Source">
            <Select
              value={filters.source ?? ''}
              onChange={(e) => set({ source: e.target.value || undefined, subscriptionId: undefined })}
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
          <Field label="Subscription">
            <Select
              value={filters.subscriptionId ?? ''}
              onChange={(e) => set({ subscriptionId: e.target.value || undefined })}
              className="w-full"
              disabled={!filters.source}
            >
              <option value="">all subscriptions</option>
              {subscriptions
                ?.filter((s) => s.sourceKey === filters.source)
                .map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name ?? s.sourceKey}
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
        <label className="mt-3 flex items-center gap-2 text-sm font-medium text-muted">
          <Checkbox
            checked={!filters.showBelowFloor}
            onChange={(e) => set({ showBelowFloor: e.target.checked ? undefined : true })}
          />
          hide below-floor jobs
        </label>
      </Surface>

      {isLoading ? <JobListSkeleton /> : null}
      {error ? <ErrorState error={error} /> : null}
      {data && data.items.length === 0 ? (
        <EmptyState>No jobs yet. Add a saved search on the Sources page and hit "Run now".</EmptyState>
      ) : null}

      {data && data.items.length > 0 ? (
        <VirtualList
          items={data.items}
          getKey={(job) => job.id}
          estimateSize={128}
          gap={12}
          renderItem={(job) => (
            <JobCard
              job={job}
              onShortlist={() => shortlist.mutate(job.id)}
              onHide={() => hide.mutate(job.id)}
            />
          )}
        />
      ) : null}

      {data && data.total > data.pageSize ? (
        <Pagination
          page={data.page}
          total={data.total}
          pageSize={data.pageSize}
          currentPage={filters.page ?? 1}
          onPrev={() => setPage((filters.page ?? 1) - 1)}
          onNext={() => setPage((filters.page ?? 1) + 1)}
        />
      ) : null}
    </div>
  );
}

// LOW_CONFIDENCE_THRESHOLD mirrors the backend's blended-confidence cutoff
// (spec 006 FR-006): below this, a band is shown but visibly discredited.
const LOW_CONFIDENCE_THRESHOLD = 0.3;

function formatSalaryBand(job: JobDto): string | null {
  if (job.salaryMin == null || job.salaryMax == null || !job.salaryCurrency) return null;
  return `${job.salaryMin.toLocaleString()}–${job.salaryMax.toLocaleString()} ${job.salaryCurrency}`;
}

// SalaryInfo renders the inferred band where salaryRaw used to render alone,
// keeping salaryRaw displayed alongside it per FR-024. Band-less jobs fall
// back to the salaryRaw-only display unchanged.
function SalaryInfo({ job }: { job: JobDto }) {
  const band = formatSalaryBand(job);
  const lowConfidence = job.salaryConfidence != null && job.salaryConfidence < LOW_CONFIDENCE_THRESHOLD;

  return (
    <>
      {band ? (
        <span className={lowConfidence ? 'text-warning' : undefined}>
          {' · '}
          {band}
          {lowConfidence ? ' (low confidence)' : ''}
        </span>
      ) : null}
      {job.salaryRaw ? ` · ${job.salaryRaw}` : ''}
      {job.salaryBelowFloor ? (
        <span className="ml-2 rounded bg-danger px-1.5 py-0.5 text-xs font-semibold text-white">
          below floor
        </span>
      ) : null}
    </>
  );
}

function JobListSkeleton() {
  return (
    <LoadingRegion label="loading jobs…" className="flex flex-col gap-3">
      {Array.from({ length: 6 }).map((_, i) => (
        <JobCardSkeleton key={i} />
      ))}
    </LoadingRegion>
  );
}

function JobCardSkeleton() {
  return (
    <div className="rounded-xl border border-border bg-surface p-4 shadow-sm shadow-black/20">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <SkeletonLine width="w-12" className="h-5 rounded-full" />
            <SkeletonLine width="w-2/5" className="h-5" />
          </div>
          <SkeletonLine width="w-3/5" className="mt-2" />
          <div className="mt-3 flex gap-1">
            <SkeletonBlock className="h-5 w-16" />
            <SkeletonBlock className="h-5 w-16" />
            <SkeletonBlock className="h-5 w-16" />
          </div>
        </div>
        <SkeletonBlock className="h-8 w-32 shrink-0" />
      </div>
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
    <div className="group rounded-xl border border-border bg-surface p-4 shadow-sm shadow-black/20 transition hover:border-primary/40 hover:bg-elevated">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <ScoreBadge score={job.matchResult?.score} />
            {/* Informational only — never filters, hides, dims, or reorders
                this job (Constitution Principle I / FR-015). */}
            <GhostBadge score={job.ghostSignal?.score} />
            <Link to={`/jobs/${job.id}`} className="truncate font-semibold text-primary hover:underline">
              {job.title}
            </Link>
          </div>
          <div className="mt-1 text-sm text-muted">
            {job.company}
            {job.location ? ` · ${job.location}` : ''}
            {job.remote ? ' · remote' : ''}
            <SalaryInfo job={job} />
            {postAgeLabel(job.postedAt) ? (
              <span className="ml-2 inline-flex items-center gap-1 text-xs text-faint" title={job.postedAt ?? undefined}>
                <Clock className="h-3 w-3" aria-hidden="true" />
                {postAgeLabel(job.postedAt)}
              </span>
            ) : null}
          </div>
          <div className="mt-2 flex flex-wrap gap-1">
            <Chip>{job.sourceKey}</Chip>
            {(job.matchResult?.matchedSkills ?? []).slice(0, 3).map((s) => (
              <Chip key={`matched-${s}`} tone="green">
                {s}
              </Chip>
            ))}
            {(job.matchResult?.missingSkills ?? []).slice(0, 3).map((s) => (
              <Chip key={`missing-${s}`} tone="red">
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
    </div>
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
