import { Clock, ExternalLink, EyeOff, Star } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import type { JobDto } from '@job-finder/shared';
import { type JobFilters } from '../../lib/api';
import { PageHeader } from '../../components/layout/PageHeader';
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
import { useFeedSources, useFeedSubscriptions, useHideJob, useInfiniteJobs, useShortlistJob } from './hooks';
import { postAgeLabel } from '../../lib/time';

type FeedFilters = Omit<JobFilters, 'page'>;

function filtersFromParams(params: URLSearchParams): FeedFilters {
  return {
    sort: params.get('sort') === 'date' ? 'date' : 'score',
    source: params.get('source') ?? undefined,
    subscriptionId: params.get('subscriptionId') ?? undefined,
    minScore: params.get('minScore') ? Number(params.get('minScore')) : undefined,
    remote: params.get('remote') === 'true' ? true : undefined,
    q: params.get('q') ?? undefined,
    showBelowFloor: params.get('showBelowFloor') === 'true' ? true : undefined,
    includeHidden: params.get('includeHidden') === 'true' ? true : undefined,
    includeApplied: params.get('includeApplied') === 'true' ? true : undefined,
  };
}

function paramsFromFilters(filters: FeedFilters): URLSearchParams {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(filters)) {
    if (v === undefined || v === '' || v === null) continue;
    if (k === 'sort' && v === 'score') continue;
    params.set(k, String(v));
  }
  return params;
}

export default function FeedPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = useMemo(() => filtersFromParams(searchParams), [searchParams]);
  const { data, isLoading, error, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteJobs(filters);
  const { data: sources } = useFeedSources();
  const { data: subscriptions } = useFeedSubscriptions();
  const shortlist = useShortlistJob();
  const hide = useHideJob();

  const set = (patch: Partial<FeedFilters>) =>
    setSearchParams(paramsFromFilters({ ...filters, ...patch }), { replace: true });

  const items = useMemo(() => data?.pages.flatMap((p) => p.items) ?? [], [data]);
  const total = data?.pages[0]?.total ?? 0;

  const sentinelRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const node = sentinelRef.current;
    if (!node) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && hasNextPage && !isFetchingNextPage) {
          fetchNextPage();
        }
      },
      { rootMargin: '400px' },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [fetchNextPage, hasNextPage, isFetchingNextPage]);

  const [searchInput, setSearchInput] = useState(filters.q ?? '');
  useEffect(() => {
    // Deliberate prop-to-state sync: searchInput is a locally-editable,
    // debounced mirror of the URL-driven filters.q (see the effect below),
    // not state derivable from props alone during render — the two effects
    // together implement a controlled+debounced search box. Reviewed as
    // safe (spec 023-workflow-quality-gates FR-012 lint adoption).
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setSearchInput(filters.q ?? '');
  }, [filters.q]);
  useEffect(() => {
    if (searchInput === (filters.q ?? '')) return;
    const id = setTimeout(() => set({ q: searchInput || undefined }), 300);
    return () => clearTimeout(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchInput]);

  return (
    <div>
      <div className="sticky top-0 z-10 -mx-4 -mt-7 bg-background px-4 pt-7 sm:-mx-6 sm:-mt-9 sm:px-6 sm:pt-9 lg:-mx-8 lg:-mt-11 lg:px-8 lg:pt-11">
        <PageHeader
          title="Job feed"
          description="Review fresh matches, filter by source or fit, and move promising roles into the tracker."
        />

        <Surface className="mb-4 max-w-none w-full">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-[minmax(14rem,1.4fr)_repeat(4,minmax(11rem,0.8fr))_auto] lg:items-end">
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
        <div className="mt-3 flex flex-wrap items-center gap-x-6 gap-y-2">
          <label className="flex items-center gap-2 text-sm font-medium text-muted">
            <Checkbox
              checked={!filters.showBelowFloor}
              onChange={(e) => set({ showBelowFloor: e.target.checked ? undefined : true })}
            />
            hide below-floor jobs
          </label>
          <label className="flex items-center gap-2 text-sm font-medium text-muted">
            <Checkbox
              checked={!filters.includeApplied}
              onChange={(e) => set({ includeApplied: e.target.checked ? undefined : true })}
            />
            show unapplied
          </label>
          <label className="flex items-center gap-2 text-sm font-medium text-muted">
            <Checkbox
              checked={!filters.includeHidden}
              onChange={(e) => set({ includeHidden: e.target.checked ? undefined : true })}
            />
            hide non-fit
          </label>
        </div>
        </Surface>
      </div>

      <div className="pt-4">
      {isLoading ? <JobListSkeleton /> : null}
      {error ? <ErrorState error={error} /> : null}
      {data && items.length === 0 ? (
        <EmptyState>No jobs yet. Add a saved search on the Sources page and hit "Run now".</EmptyState>
      ) : null}

      {items.length > 0 ? (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3 3xl:grid-cols-4 4xl:grid-cols-5">
          {items.map((job) => (
            <JobCard
              key={job.id}
              job={job}
              onShortlist={() => shortlist.mutate(job.id)}
              onHide={() => hide.mutate(job.id)}
            />
          ))}
        </div>
      ) : null}

      <div ref={sentinelRef} />

      {isFetchingNextPage ? (
        <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3 3xl:grid-cols-4 4xl:grid-cols-5">
          {Array.from({ length: 4 }).map((_, i) => (
            <JobCardSkeleton key={i} />
          ))}
        </div>
      ) : null}

      {items.length > 0 && !hasNextPage ? (
        <p className="mt-4 text-center text-xs text-faint">
          {total} job{total === 1 ? '' : 's'} · end of feed
        </p>
      ) : null}
      </div>
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
    <LoadingRegion
      label="loading jobs…"
      className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3 3xl:grid-cols-4 4xl:grid-cols-5"
    >
      {Array.from({ length: 10 }).map((_, i) => (
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
    <div className="group flex h-full flex-col rounded-xl border border-border bg-surface p-4 shadow-sm shadow-black/20 transition hover:border-accent/40 hover:bg-surface-secondary">
      <div className="flex flex-1 flex-col gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <ScoreBadge score={job.matchResult?.score} />
            {/* Informational only — never filters, hides, dims, or reorders
                this job (Constitution Principle I / FR-015). */}
            <GhostBadge score={job.ghostSignal?.score} />
            <Link to={`/jobs/${job.id}`} className="min-w-0 flex-1 truncate font-semibold text-accent hover:underline">
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
            className="inline-flex rounded-md px-2 py-1.5 text-sm text-muted hover:bg-surface-tertiary"
            aria-label={`Open ${job.title}`}
          >
            <ExternalLink className="h-4 w-4" aria-hidden="true" />
          </a>
        </div>
      </div>
    </div>
  );
}

