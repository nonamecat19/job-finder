import { AlertTriangle, CheckCircle, Clock, Play, Plus, RefreshCw, ToggleLeft, ToggleRight, XCircle } from 'lucide-react';
import { useState } from 'react';
import type { JobSourceDto, SavedSearchDto, SearchQuery } from '@job-finder/shared';
import { SubscriptionRow } from './SubscriptionRow';
import { PageHeader } from '../../components/layout/PageHeader';
import { DashboardGrid, ListRow, Tile } from '../../components/layout';
import {
  Button,
  Chip,
  EmptyState,
  ErrorState,
  Field,
  HealthDot,
  Input,
  LoadingRegion,
  Select,
  Spinner,
  SkeletonBlock,
} from '../../components/ui';
import {
  useCreateSearch,
  useCreateSubscription,
  useDeleteSearch,
  useDeleteSubscription,
  useHostRetrievalStatus,
  useClearRungPreference,
  useClearCookies,
  useOverrideCoolingOff,
  useRecentRuns,
  useRunSearch,
  useRunSubscription,
  useRunAllSubscriptions,
  useSearches,
  useSources,
  useSubscriptions,
  useTestSource,
  useUpdateSource,
} from './hooks';
import RosterPanel from './roster/RosterPanel';
import CandidatesPanel from './roster/CandidatesPanel';

export default function SourcesPage() {
  return (
    <div>
      <PageHeader
        title="Sources & searches"
        description="Configure which job boards to scrape and manage saved searches that run on a schedule."
      />
      <DashboardGrid>
        <Tile span="full" title="Job sources"><SourcesPanel /></Tile>
        <Tile span="full" title="Host retrieval status"><HostRetrievalPanel /></Tile>
        <Tile span="full" title="Employer roster"><RosterPanel /></Tile>
        <Tile span="full" title="Board candidates"><CandidatesPanel /></Tile>
        <Tile span="full" title="Subscriptions"><SubscriptionsPanel /></Tile>
        <Tile span="full" title="Saved searches"><SearchesPanel /></Tile>
        <Tile span="full" title="Recent runs"><RecentRunsPanel /></Tile>
      </DashboardGrid>
    </div>
  );
}

function ListRowsSkeleton({ label }: { label: string }) {
  return (
    <LoadingRegion label={label} className="space-y-2">
      {Array.from({ length: 3 }).map((_, i) => (
        <SkeletonBlock key={i} className="h-12 w-full" />
      ))}
    </LoadingRegion>
  );
}

function SourcesPanel() {
  const { data: sources, isLoading, error } = useSources();
  const update = useUpdateSource();
  const test = useTestSource();

  if (isLoading) return <ListRowsSkeleton label="loading sources…" />;
  if (error) return <ErrorState error={error} />;
  if (!sources?.length) return <EmptyState>No sources configured.</EmptyState>;

  return (
    <div>
      {sources.map((s) => (
        <SourceRow
          key={s.key}
          source={s}
          onToggle={() => update.mutate({ key: s.key, body: { enabled: !s.enabled } })}
          onTest={() => test.mutate(s.key)}
          testing={test.isPending && test.variables === s.key}
        />
      ))}
    </div>
  );
}

function SourceRow({
  source,
  onToggle,
  onTest,
  testing,
}: {
  source: JobSourceDto;
  onToggle: () => void;
  onTest: () => void;
  testing: boolean;
}) {
  return (
    <ListRow
      leading={<HealthDot healthy={source.healthy} />}
      title={
        <span className="inline-flex items-center gap-2">
          {source.key}
          <Chip>{source.kind}</Chip>
          {!source.enabled ? <Chip tone="red">disabled</Chip> : null}
        </span>
      }
      aside={
        <div className="flex items-center gap-2">
          <Button variant="ghost" onClick={onTest} disabled={testing}>
            {testing ? <Spinner /> : <>test</>}
          </Button>
          <button onClick={onToggle} className="text-muted hover:text-foreground" title="toggle">
            {source.enabled ? (
              <ToggleRight className="h-6 w-6 text-accent" />
            ) : (
              <ToggleLeft className="h-6 w-6 text-faint" />
            )}
          </button>
        </div>
      }
    />
  );
}

function formatPacing(pacing: { intervalSeconds: number; source: string }): string {
  const rounded = Math.round(pacing.intervalSeconds * 10) / 10;
  const interval = Number.isInteger(rounded) ? rounded.toString() : rounded.toFixed(1);
  return `~1 request every ${interval}s (${pacing.source})`;
}

export function HostRetrievalPanel() {
  const { data: sources } = useSources();
  const [selectedHost, setSelectedHost] = useState<string | null>(null);
  const { data: status, isLoading } = useHostRetrievalStatus(selectedHost ?? '');
  const clearRung = useClearRungPreference();
  const clearCookies = useClearCookies();
  const overrideCooling = useOverrideCoolingOff();

  if (!sources?.length) return null;

  const hosts = [...new Set(sources.map((s) => s.key))];

  return (
    <div>
      <div className="mb-3 flex flex-wrap gap-2">
        {hosts.map((h) => (
          <Button
            key={h}
            variant={selectedHost === h ? 'primary' : 'secondary'}
            onClick={() => setSelectedHost(selectedHost === h ? null : h)}
          >
            {h}
          </Button>
        ))}
      </div>

      {selectedHost && isLoading ? (
        <LoadingRegion label="loading host status…" className="space-y-2">
          <SkeletonBlock className="h-20 w-full" />
        </LoadingRegion>
      ) : null}

      {selectedHost && status ? (
        <div className="rounded-xl border border-border bg-surface-secondary p-4 text-sm">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted">
            <span>
              <span className="font-medium text-foreground">Rung:</span> {status.currentRung}
            </span>
            <span>
              <span className="font-medium text-foreground">Pace:</span> {formatPacing(status.pacing)}
            </span>
          </div>

          {status.lastBlockAt ? (
            <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
              <span className="text-warning">
                <AlertTriangle className="mr-1 inline h-3 w-3" />
                Blocked at {new Date(status.lastBlockAt).toLocaleString()}
              </span>
              {status.lastBlockReason ? (
                <span className="text-faint">Reason: {status.lastBlockReason}</span>
              ) : null}
            </div>
          ) : null}

          {status.coolingOffUntil ? (
            <div className="mt-1 text-xs text-danger">
              <Clock className="mr-1 inline h-3 w-3" />
              Cooling off until {new Date(status.coolingOffUntil).toLocaleString()}
            </div>
          ) : null}

          <div className="mt-3 flex flex-wrap gap-2">
            <Button
              variant="secondary"
              onClick={() => clearRung.mutate(selectedHost)}
              disabled={clearRung.isPending}
            >
              clear rung
            </Button>
            <Button
              variant="secondary"
              onClick={() => clearCookies.mutate(selectedHost)}
              disabled={clearCookies.isPending}
            >
              clear cookies
            </Button>
            <Button
              variant="secondary"
              onClick={() => overrideCooling.mutate(selectedHost)}
              disabled={overrideCooling.isPending}
            >
              override cooling-off
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function SearchesPanel() {
  const { data: searches, isLoading, error } = useSearches();
  const create = useCreateSearch();
  const [showForm, setShowForm] = useState(false);

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <Button onClick={() => setShowForm((v) => !v)}>{showForm ? 'cancel' : '+ new search'}</Button>
      </div>

      {showForm ? <NewSearchForm onSubmit={(q) => { create.mutate(q); setShowForm(false); }} /> : null}

      {isLoading ? <ListRowsSkeleton label="loading searches…" /> : null}
      {error ? <ErrorState error={error} /> : null}
      {searches && searches.length === 0 && !showForm ? (
        <EmptyState>No saved searches. Create one to start finding jobs.</EmptyState>
      ) : null}

      <div>
        {searches?.map((s) => (
          <SearchRow key={s.id} search={s} />
        ))}
      </div>
    </div>
  );
}

function NewSearchForm({ onSubmit }: { onSubmit: (q: { name: string; query: SearchQuery }) => void }) {
  const [name, setName] = useState('');
  const [keywords, setKeywords] = useState('');
  const [location, setLocation] = useState('');

  const handleSubmit = () => {
    if (!name.trim() || !keywords.trim()) return;
    onSubmit({ name: name.trim(), query: { keywords: keywords.trim(), location: location.trim() || undefined } });
  };

  return (
    <div className="mb-3 rounded-xl border border-border bg-surface-secondary p-4">
      <div className="grid gap-2 sm:grid-cols-3">
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. React remote" />
        </Field>
        <Field label="Keywords">
          <Input value={keywords} onChange={(e) => setKeywords(e.target.value)} placeholder="e.g. React developer" />
        </Field>
        <Field label="Location">
          <Input value={location} onChange={(e) => setLocation(e.target.value)} placeholder="optional" />
        </Field>
      </div>
      <div className="mt-3">
        <Button onClick={handleSubmit} disabled={!name.trim() || !keywords.trim()}>
          create
        </Button>
      </div>
    </div>
  );
}

function SearchRow({ search }: { search: SavedSearchDto }) {
  const run = useRunSearch();
  const remove = useDeleteSearch();

  return (
    <ListRow
      title={
        <span className="inline-flex items-center gap-2">
          {search.name}
          {search.cron ? <Chip>{search.cron}</Chip> : null}
          {!search.enabled ? <Chip tone="red">paused</Chip> : null}
        </span>
      }
      meta={
        <>
          {search.query.keywords}
          {search.query.location ? ` in ${search.query.location}` : ''}
          {search.lastRunAt ? ` · last run ${new Date(search.lastRunAt).toLocaleString()}` : ''}
        </>
      }
      aside={
        <div className="flex items-center gap-2">
          <Button variant="secondary" onClick={() => run.mutate(search.id)} disabled={run.isPending}>
            <Play className="h-3 w-3" /> run now
          </Button>
          <Button variant="ghost" onClick={() => remove.mutate(search.id)}>
            delete
          </Button>
        </div>
      }
    />
  );
}

const SUBSCRIPTION_SOURCES = [
  { key: 'dou', label: 'DOU.ua', placeholder: 'https://jobs.dou.ua/vacancies/?category=Node.js' },
  { key: 'djinni', label: 'Djinni', placeholder: 'https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote' },
  { key: 'indeed', label: 'Indeed', placeholder: 'https://www.indeed.com/jobs?q=golang&l=remote' },
  { key: 'remoteok', label: 'RemoteOK', placeholder: 'https://remoteok.com/remote-golang-jobs' },
  { key: 'himalayas', label: 'Himalayas', placeholder: 'https://himalayas.app/jobs?categories=Backend-Engineering' },
  { key: 'glassdoor', label: 'Glassdoor', placeholder: 'https://www.glassdoor.com/Job/remote-golang-jobs-SRCH_KO0,14.htm' },
  { key: 'jobleads', label: 'JobLeads', placeholder: 'https://www.jobleads.com/job-search?q=golang' },
  { key: 'wellfound', label: 'Wellfound', placeholder: 'https://wellfound.com/role/r/golang-engineer' },
  { key: 'jobgether', label: 'Jobgether', placeholder: 'https://jobgether.com/jobs/search?technology=go&remote=true' },
];

function SubscriptionsPanel() {
  const { data: subs, isLoading, error } = useSubscriptions();
  const create = useCreateSubscription();
  const remove = useDeleteSubscription();
  const run = useRunSubscription();
  const runAll = useRunAllSubscriptions();
  const [showForm, setShowForm] = useState(false);

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            onClick={() => runAll.mutate()}
            disabled={runAll.isPending || !subs?.some((s) => s.enabled && s.kind !== 'manual')}
          >
            <Play className="h-3 w-3" /> run all
          </Button>
          <Button onClick={() => setShowForm((v) => !v)}>{showForm ? 'cancel' : '+ add subscription'}</Button>
        </div>
      </div>

      {showForm ? <NewSubscriptionForm onSubmit={(body) => { create.mutate(body); setShowForm(false); }} /> : null}

      {isLoading ? <ListRowsSkeleton label="loading subscriptions…" /> : null}
      {error ? <ErrorState error={error} /> : null}
      {subs && subs.length === 0 && !showForm ? (
        <EmptyState>No subscriptions. Add a DOU or Djinni subscription URL to scrape job listings.</EmptyState>
      ) : null}

      <div>
        {subs?.map((s) => (
          <SubscriptionRow
            key={s.id}
            sub={s}
            onRun={() => run.mutate(s.id)}
            onDelete={() => remove.mutate(s.id)}
            running={run.isPending && run.variables === s.id}
          />
        ))}
      </div>
    </div>
  );
}

function NewSubscriptionForm({ onSubmit }: { onSubmit: (body: { sourceKey: string; url: string; name?: string }) => void }) {
  const [sourceKey, setSourceKey] = useState('dou');
  const [url, setUrl] = useState('');
  const [name, setName] = useState('');
  const source = SUBSCRIPTION_SOURCES.find((s) => s.key === sourceKey);

  const handleSubmit = () => {
    if (!url.trim()) return;
    onSubmit({ sourceKey, url: url.trim(), name: name.trim() || undefined });
  };

  return (
    <div className="mb-3 rounded-xl border border-border bg-surface-secondary p-4">
      <div className="grid gap-2 sm:grid-cols-3">
        <Field label="Source">
          <Select value={sourceKey} onChange={(e) => setSourceKey(e.target.value)}>
            {SUBSCRIPTION_SOURCES.map((s) => (
              <option key={s.key} value={s.key}>{s.label}</option>
            ))}
          </Select>
        </Field>
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Node.js DOU" />
        </Field>
        <Field label="Subscription URL">
          <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder={source?.placeholder} />
        </Field>
      </div>
      <div className="mt-3">
        <Button onClick={handleSubmit} disabled={!url.trim()}>
          <Plus className="h-3 w-3" /> create
        </Button>
      </div>
    </div>
  );
}

function RecentRunsPanel() {
  const { data: runs, isLoading } = useRecentRuns();

  return (
    <div>
      {isLoading ? <ListRowsSkeleton label="loading recent runs…" /> : null}
      {runs && runs.length === 0 ? <EmptyState>No runs yet.</EmptyState> : null}
      <div>
        {runs?.map((r) => {
          const verdict = r.verdict;
          const isRunning = r.ok === null;
          return (
            <ListRow
              key={r.id}
              leading={
                isRunning ? (
                  <Spinner />
                ) : verdict === 'blocked' ? (
                  <XCircle className="h-4 w-4 text-danger" />
                ) : verdict === 'success' || r.ok === true ? (
                  <CheckCircle className="h-4 w-4 text-success" />
                ) : (
                  <RefreshCw className="h-4 w-4 text-danger" />
                )
              }
              title={
                <span className="inline-flex items-center gap-2">
                  {r.sourceKey}
                  {verdict ? (
                    <Chip tone={verdict === 'success' ? 'green' : verdict === 'blocked' ? 'red' : 'slate'}>{verdict}</Chip>
                  ) : null}
                </span>
              }
              meta={
                <>
                  found {r.found}, {r.new} new
                  {r.blockReason ? ` · ${r.blockReason.slice(0, 40)}` : ''}
                </>
              }
              aside={new Date(r.startedAt).toLocaleString()}
            />
          );
        })}
      </div>
    </div>
  );
}
