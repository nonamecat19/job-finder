import { AlertTriangle, CheckCircle, Clock, ListFilter, Play, Plus, RefreshCw, ToggleLeft, ToggleRight, Trash2, XCircle } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import type { JobSourceDto, SavedSearchDto, SearchQuery, SubscriptionDto } from '@job-finder/shared';
import { summarizeDjinniBasicSearch } from './djinniSearchSummary';
import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import {
  Button,
  Chip,
  EmptyState,
  ErrorState,
  Field,
  HealthDot,
  Input,
  LoadingRegion,
  Spinner,
  SkeletonBlock,
  Surface,
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
      <SourcesPanel />
      <HostRetrievalPanel />
      <RosterPanel />
      <CandidatesPanel />
      <SubscriptionsPanel />
      <SearchesPanel />
      <RecentRunsPanel />
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
    <Surface className="mb-5">
      <SectionTitle>Job sources</SectionTitle>
      <ul className="space-y-2">
        {sources.map((s) => (
          <SourceRow
            key={s.key}
            source={s}
            onToggle={() => update.mutate({ key: s.key, body: { enabled: !s.enabled } })}
            onTest={() => test.mutate(s.key)}
            testing={test.isPending && test.variables === s.key}
          />
        ))}
      </ul>
    </Surface>
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
    <li className="flex flex-col gap-2 rounded-md border border-border bg-elevated/60 p-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex items-center gap-2">
        <HealthDot healthy={source.healthy} />
        <span className="font-medium text-fg">{source.key}</span>
        <Chip>{source.kind}</Chip>
        {!source.enabled ? <Chip tone="red">disabled</Chip> : null}
      </div>
      <div className="flex items-center gap-2">
        <Button variant="ghost" onClick={onTest} disabled={testing}>
          {testing ? <Spinner /> : <>test</>}
        </Button>
        <button onClick={onToggle} className="text-muted hover:text-fg" title="toggle">
          {source.enabled ? (
            <ToggleRight className="h-6 w-6 text-primary" />
          ) : (
            <ToggleLeft className="h-6 w-6 text-faint" />
          )}
        </button>
      </div>
    </li>
  );
}

function HostRetrievalPanel() {
  const { data: sources } = useSources();
  const [selectedHost, setSelectedHost] = useState<string | null>(null);
  const { data: status, isLoading } = useHostRetrievalStatus(selectedHost ?? '');
  const clearRung = useClearRungPreference();
  const clearCookies = useClearCookies();
  const overrideCooling = useOverrideCoolingOff();

  if (!sources?.length) return null;

  const hosts = [...new Set(sources.map((s) => s.key))];

  return (
    <Surface className="mb-5">
      <SectionTitle>Host retrieval status</SectionTitle>
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
        <div className="rounded-lg border border-border bg-elevated/60 p-3 text-sm">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted">
            <span>
              <span className="font-medium text-fg">Rung:</span> {status.currentRung}
            </span>
            <span>
              <span className="font-medium text-fg">Budget:</span> {status.budgetUsed}/{status.budgetLimit}
            </span>
            <span>
              <span className="font-medium text-fg">Budget resets:</span> {new Date(status.budgetResetsAt).toLocaleString()}
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
    </Surface>
  );
}

function SearchesPanel() {
  const { data: searches, isLoading, error } = useSearches();
  const create = useCreateSearch();
  const [showForm, setShowForm] = useState(false);

  return (
    <Surface className="mb-5">
      <div className="mb-3 flex items-center justify-between">
        <SectionTitle>Saved searches</SectionTitle>
        <Button onClick={() => setShowForm((v) => !v)}>{showForm ? 'cancel' : '+ new search'}</Button>
      </div>

      {showForm ? <NewSearchForm onSubmit={(q) => { create.mutate(q); setShowForm(false); }} /> : null}

      {isLoading ? <ListRowsSkeleton label="loading searches…" /> : null}
      {error ? <ErrorState error={error} /> : null}
      {searches && searches.length === 0 && !showForm ? (
        <EmptyState>No saved searches. Create one to start finding jobs.</EmptyState>
      ) : null}

      <ul className="space-y-2">
        {searches?.map((s) => (
          <SearchRow key={s.id} search={s} />
        ))}
      </ul>
    </Surface>
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
    <div className="mb-3 rounded-lg border border-primary/30 bg-primary-soft p-3">
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
      <div className="mt-2">
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
    <li className="flex flex-col gap-2 rounded-md border border-border bg-elevated/60 p-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <span className="font-medium text-fg">{search.name}</span>
        <span className="ml-2 text-xs text-muted">
          {search.query.keywords}
          {search.query.location ? ` in ${search.query.location}` : ''}
        </span>
        {search.cron ? <Chip>{search.cron}</Chip> : null}
        {!search.enabled ? <Chip tone="red">paused</Chip> : null}
        {search.lastRunAt ? (
          <span className="ml-2 text-xs text-faint">
            last run {new Date(search.lastRunAt).toLocaleString()}
          </span>
        ) : null}
      </div>
      <div className="flex items-center gap-2">
        <Button variant="secondary" onClick={() => run.mutate(search.id)} disabled={run.isPending}>
          <Play className="h-3 w-3" /> run now
        </Button>
        <Button variant="ghost" onClick={() => remove.mutate(search.id)}>
          delete
        </Button>
      </div>
    </li>
  );
}

const SUBSCRIPTION_SOURCES = [
  { key: 'dou', label: 'DOU.ua', placeholder: 'https://jobs.dou.ua/vacancies/?category=Node.js' },
  { key: 'djinni', label: 'Djinni', placeholder: 'https://djinni.co/my/dashboard/subs/{id}/' },
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
    <Surface className="mb-5">
      <div className="mb-3 flex items-center justify-between">
        <SectionTitle>Subscriptions</SectionTitle>
        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            onClick={() => runAll.mutate()}
            disabled={runAll.isPending || !subs?.some((s) => s.enabled)}
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

      <ul className="space-y-2">
        {subs?.map((s) => (
          <SubscriptionRow
            key={s.id}
            sub={s}
            onRun={() => run.mutate(s.id)}
            onDelete={() => remove.mutate(s.id)}
            running={run.isPending && run.variables === s.id}
          />
        ))}
      </ul>
    </Surface>
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
    <div className="mb-3 rounded-lg border border-primary/30 bg-primary-soft p-3">
      <div className="grid gap-2 sm:grid-cols-3">
        <Field label="Source">
          <select
            value={sourceKey}
            onChange={(e) => setSourceKey(e.target.value)}
            className="w-full rounded-lg border border-border bg-elevated px-3 py-2 text-sm text-fg shadow-sm outline-none transition placeholder:text-faint focus:border-primary focus:ring-2 focus:ring-primary-soft"
          >
            {SUBSCRIPTION_SOURCES.map((s) => (
              <option key={s.key} value={s.key}>{s.label}</option>
            ))}
          </select>
        </Field>
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Node.js DOU" />
        </Field>
        <Field label="Subscription URL">
          <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder={source?.placeholder} />
        </Field>
      </div>
      <div className="mt-2">
        <Button onClick={handleSubmit} disabled={!url.trim()}>
          <Plus className="h-3 w-3" /> create
        </Button>
      </div>
    </div>
  );
}

function SubscriptionRow({ sub, onRun, onDelete, running }: { sub: SubscriptionDto; onRun: () => void; onDelete: () => void; running: boolean }) {
  const basicSearchLabel = sub.sourceKey === 'djinni' ? summarizeDjinniBasicSearch(sub.url) : null
  const djinniModeMarker =
    sub.sourceKey === 'djinni' ? (
      <span className="ml-1 text-xs text-faint">
        {basicSearchLabel !== null ? '· basic-search' : '· dashboard'}
      </span>
    ) : null
  return (
    <li className="flex flex-col gap-2 rounded-md border border-border bg-elevated/60 p-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <span className="font-medium text-fg">{basicSearchLabel ?? sub.name ?? sub.sourceKey}</span>
        <span className="ml-2 text-xs text-muted">{sub.sourceKey}</span>
        {djinniModeMarker}
        <div className="truncate text-xs text-faint" title={sub.url}>{sub.url}</div>
        {sub.lastRunAt ? (
          <span className="mr-2 text-xs text-faint">
            last run {new Date(sub.lastRunAt).toLocaleString()}
          </span>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Link
          to={`/?source=${sub.sourceKey}&subscriptionId=${sub.id}`}
          className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-semibold text-muted transition hover:bg-overlay"
        >
          <ListFilter className="h-3 w-3" /> view jobs
        </Link>
        <Button variant="secondary" onClick={onRun} disabled={running}>
          <Play className="h-3 w-3" /> run now
        </Button>
        <Button variant="ghost" onClick={onDelete}>
          <Trash2 className="h-3 w-3" />
        </Button>
      </div>
    </li>
  );
}

function RecentRunsPanel() {
  const { data: runs, isLoading } = useRecentRuns();

  return (
    <Surface>
      <SectionTitle>Recent runs</SectionTitle>
      {isLoading ? <ListRowsSkeleton label="loading recent runs…" /> : null}
      {runs && runs.length === 0 ? <EmptyState>No runs yet.</EmptyState> : null}
      <ul className="space-y-1">
        {runs?.map((r) => {
          const verdict = r.verdict;
          const isRunning = r.ok === null;
          return (
            <li key={r.id} className="flex items-center gap-2 text-sm text-muted">
              {isRunning ? (
                <Spinner />
              ) : verdict === 'blocked' ? (
                <XCircle className="h-4 w-4 text-danger" />
              ) : verdict === 'success' || r.ok === true ? (
                <CheckCircle className="h-4 w-4 text-success" />
              ) : (
                <RefreshCw className="h-4 w-4 text-danger" />
              )}
              <span className="font-medium">{r.sourceKey}</span>
              <span className="text-xs text-muted">
                found {r.found}, {r.new} new
              </span>
              {verdict ? <Chip tone={verdict === 'success' ? 'green' : verdict === 'blocked' ? 'red' : 'slate'}>{verdict}</Chip> : null}
              {r.blockReason ? (
                <span className="text-xs text-faint" title={r.blockReason}>
                  {r.blockReason.slice(0, 40)}
                </span>
              ) : null}
              <span className="text-xs text-faint">{new Date(r.startedAt).toLocaleString()}</span>
            </li>
          );
        })}
      </ul>
    </Surface>
  );
}
