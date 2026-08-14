import { Clock, ExternalLink, EyeOff, Star } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import type { JobDto } from '@job-finder/shared';
import { type JobFilters } from '../../lib/api';
import { DashboardGrid, ListRow, PageHeader, Tile } from '../../components/layout';
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
} from '../../components/ui';
import { AddVacancyForm } from './AddVacancyForm';
import { useFeedSources, useFeedSubscriptions, useHideJob, useInfiniteJobs, useShortlistJob } from './hooks';
import { postAgeLabel } from '../../lib/time';

type FeedFilters = Omit<JobFilters, 'page'>;

function filtersFromParams(params: URLSearchParams): FeedFilters {
  const floor = params.get('floor');
  const hidden = params.get('hidden');
  const applied = params.get('applied');
  return {
    sort: params.get('sort') === 'date' ? 'date' : 'score',
    source: params.get('source') ?? undefined,
    subscriptionId: params.get('subscriptionId') ?? undefined,
    minScore: params.get('minScore') ? Number(params.get('minScore')) : undefined,
    remote: params.get('remote') === 'true' ? true : undefined,
    q: params.get('q') ?? undefined,
    showBelowFloor: floor === 'all' ? true : undefined,
    onlyBelowFloor: floor === 'only' ? true : undefined,
    includeHidden: hidden === 'all' ? true : undefined,
    onlyHidden: hidden === 'only' ? true : undefined,
    includeApplied: applied === 'all' ? true : undefined,
    onlyApplied: applied === 'only' ? true : undefined,
    onlyManual: params.get('manual') === 'only' ? true : undefined,
  };
}

function paramsFromFilters(filters: FeedFilters): URLSearchParams {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(filters)) {
    if (v === undefined || v === '' || v === null) continue;
    if (k === 'sort' && v === 'score') continue;
    params.set(k, String(v));
  }
  const floor = filters.onlyBelowFloor ? 'only' : filters.showBelowFloor ? 'all' : 'above';
  const hidden = filters.onlyHidden ? 'only' : filters.includeHidden ? 'all' : 'fit';
  const applied = filters.onlyApplied ? 'only' : filters.includeApplied ? 'all' : 'unapplied';
  params.delete('showBelowFloor');
  params.delete('onlyBelowFloor');
  params.delete('includeHidden');
  params.delete('onlyHidden');
  params.delete('includeApplied');
  params.delete('onlyApplied');
  params.delete('onlyManual');
  if (filters.onlyManual) params.set('manual', 'only');
  if (floor !== 'above') params.set('floor', floor);
  if (hidden !== 'fit') params.set('hidden', hidden);
  if (applied !== 'unapplied') params.set('applied', applied);
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

  // The vacancy a manual add just created, so its row is findable in a feed
  // that may already be long.
  const [highlightedJobId, setHighlightedJobId] = useState<string | null>(null);

  const [searchInput, setSearchInput] = useState(filters.q ?? '');
  useEffect(() => {
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
      <PageHeader
        title="Job feed"
        description="Review fresh matches, filter by source or fit, and move promising roles into the tracker."
      />

      <div className="sticky top-0 z-10 bg-background pb-4">
        <DashboardGrid>
          <Tile title="Add a vacancy" span="full">
            <AddVacancyForm onCreated={setHighlightedJobId} />
          </Tile>

          <Tile title="Filters" span="full">
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
            <div className="mt-3 flex flex-wrap items-end gap-x-4 gap-y-2">
              <Field label="Below floor">
                <Select
                  value={filters.onlyBelowFloor ? 'only' : filters.showBelowFloor ? 'all' : 'above'}
                  onChange={(e) => {
                    const v = e.target.value;
                    set({
                      onlyBelowFloor: v === 'only' ? true : undefined,
                      showBelowFloor: v === 'all' ? true : undefined,
                    });
                  }}
                  className="w-full"
                >
                  <option value="above">only above-floor</option>
                  <option value="all">all</option>
                  <option value="only">only below-floor</option>
                </Select>
              </Field>
              <Field label="Applied">
                <Select
                  value={filters.onlyApplied ? 'only' : filters.includeApplied ? 'all' : 'unapplied'}
                  onChange={(e) => {
                    const v = e.target.value;
                    set({
                      onlyApplied: v === 'only' ? true : undefined,
                      includeApplied: v === 'all' ? true : undefined,
                    });
                  }}
                  className="w-full"
                >
                  <option value="unapplied">only unapplied</option>
                  <option value="all">all</option>
                  <option value="only">only applied</option>
                </Select>
              </Field>
              <Field label="Added by hand">
                <Select
                  value={filters.onlyManual ? 'only' : 'all'}
                  onChange={(e) => set({ onlyManual: e.target.value === 'only' ? true : undefined })}
                  className="w-full"
                >
                  <option value="all">all vacancies</option>
                  <option value="only">only manual</option>
                </Select>
              </Field>
              <Field label="Non-fit">
                <Select
                  value={filters.onlyHidden ? 'only' : filters.includeHidden ? 'all' : 'fit'}
                  onChange={(e) => {
                    const v = e.target.value;
                    set({
                      onlyHidden: v === 'only' ? true : undefined,
                      includeHidden: v === 'all' ? true : undefined,
                    });
                  }}
                  className="w-full"
                >
                  <option value="fit">only fit</option>
                  <option value="all">all</option>
                  <option value="only">only non-fit</option>
                </Select>
              </Field>
            </div>
          </Tile>
        </DashboardGrid>
      </div>

      <div className="pt-4">
        <DashboardGrid>
          <Tile
            span="full"
            title="Jobs"
            action={
              data ? (
                <span className="[font:var(--type-caption)] tabular-nums text-muted">
                  {total} job{total === 1 ? '' : 's'}
                </span>
              ) : undefined
            }
            footer={
              items.length > 0 && !hasNextPage
                ? `${total} job${total === 1 ? '' : 's'} · end of feed`
                : undefined
            }
          >
            {isLoading ? <JobListSkeleton /> : null}
            {error ? <ErrorState error={error} /> : null}
            {data && items.length === 0 ? (
              <EmptyState>No jobs yet. Add a saved search on the Sources page and hit "Run now".</EmptyState>
            ) : null}

            {items.length > 0 ? (
              <div className="flex flex-col">
                {items.map((job) => (
                  <JobRow
                    key={job.id}
                    job={job}
                    highlighted={job.id === highlightedJobId}
                    onShortlist={() => shortlist.mutate(job.id)}
                    onHide={() => hide.mutate(job.id)}
                  />
                ))}
              </div>
            ) : null}

            <div ref={sentinelRef} />

            {isFetchingNextPage ? (
              <div className="flex flex-col gap-1">
                {Array.from({ length: 4 }).map((_, i) => (
                  <JobRowSkeleton key={i} />
                ))}
              </div>
            ) : null}
          </Tile>
        </DashboardGrid>
      </div>
    </div>
  );
}

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
        <span className="ml-1.5 inline-flex items-center rounded-full bg-danger-soft px-1.5 py-0.5 text-[0.6875rem] font-semibold text-danger">
          below floor
        </span>
      ) : null}
    </>
  );
}

function JobListSkeleton() {
  return (
    <LoadingRegion label="loading jobs…" className="flex flex-col gap-1">
      {Array.from({ length: 10 }).map((_, i) => (
        <JobRowSkeleton key={i} />
      ))}
    </LoadingRegion>
  );
}

function JobRowSkeleton() {
  return (
    <div className="flex items-center gap-3 rounded-lg px-3 py-2.5">
      <SkeletonBlock className="h-6 w-9 shrink-0 rounded-full" />
      <div className="min-w-0 flex-1">
        <SkeletonLine width="w-2/5" />
        <SkeletonLine width="w-3/5" className="mt-1.5" />
      </div>
      <SkeletonBlock className="h-8 w-28 shrink-0" />
    </div>
  );
}

function JobRow({
  job,
  highlighted,
  onShortlist,
  onHide,
}: {
  job: JobDto;
  highlighted?: boolean;
  onShortlist: () => void;
  onHide: () => void;
}) {
  const matched = job.matchResult?.matchedSkills ?? [];
  const missing = job.matchResult?.missingSkills ?? [];

  return (
    <ListRow
      className={highlighted ? 'ring-2 ring-accent/30' : undefined}
      leading={<ScoreBadge score={job.matchResult?.score} />}
      title={
        <span className="inline-flex min-w-0 items-center gap-2">
          <Link to={`/jobs/${job.id}`} className="truncate text-accent hover:underline">
            {job.title}
          </Link>
          <GhostBadge score={job.ghostSignal?.score} />
        </span>
      }
      meta={
        <span className="inline-flex flex-wrap items-center gap-x-1">
          {job.company}
          {job.location ? ` · ${job.location}` : ''}
          {job.remote ? ' · remote' : ''}
          <SalaryInfo job={job} />
          {postAgeLabel(job.postedAt) ? (
            <span className="ml-1.5 inline-flex items-center gap-1" title={job.postedAt ?? undefined}>
              <Clock className="h-3 w-3" aria-hidden="true" />
              {postAgeLabel(job.postedAt)}
            </span>
          ) : null}
          {matched.length > 0 ? ` · matches ${matched.slice(0, 3).join(', ')}` : ''}
          {missing.length > 0 ? ` · missing ${missing.slice(0, 3).join(', ')}` : ''}
        </span>
      }
      aside={
        <div className="flex shrink-0 items-center gap-1">
          <Chip>{job.sourceKey}</Chip>
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
      }
    />
  );
}
