import { Briefcase, Clock3, Star } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import type { ApplicationDto, ApplicationStatus } from '@job-finder/shared';
import { DashboardGrid, IconTile, PageHeader, StatTile, Tile } from '../../components/layout';
import { VirtualList } from '../../components/VirtualList';
import {
  Button,
  Chip,
  EmptyState,
  ErrorState,
  LoadingRegion,
  Select,
  SkeletonBlock,
  SkeletonLine,
  Textarea,
} from '../../components/ui';
import { useApplications, useStats, useUpdateApplication } from './hooks';

const STATUS_LABELS: Record<ApplicationStatus, string> = {
  found: 'found',
  shortlisted: 'shortlisted',
  docs_generated: 'docs generated',
  applied: 'applied',
  interview: 'interview',
  offer: 'offer',
  rejected: 'rejected',
};

// Only the terminal outcomes get a coloured chip; everything mid-pipeline
// stays neutral so the one accent blue keeps carrying the page.
const STATUS_TONE: Partial<Record<ApplicationStatus, 'green' | 'red'>> = {
  offer: 'green',
  rejected: 'red',
};

export default function TrackerPage() {
  const [statusFilter, setStatusFilter] = useState<string>('');
  const { data: applications, isLoading, error } = useApplications(statusFilter || undefined);
  const { data: stats } = useStats();

  return (
    <div>
      <PageHeader
        title="Application tracker"
        description="Track your applications from first match to offer."
      />

      <DashboardGrid>
        {stats ? (
          <>
            <StatTile span="compact" caption="total" value={stats.jobsTotal} icon={Briefcase} tint="blue" />
            <StatTile span="compact" caption="high fit" value={stats.highFit} icon={Star} tint="mint" />
            <StatTile span="compact" caption="new today" value={stats.jobsLast24h} icon={Clock3} tint="amber" />
          </>
        ) : null}

        <Tile
          span="full"
          title="Applications"
          action={
            <Select
              aria-label="Filter by status"
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="w-44"
            >
              <option value="">all statuses</option>
              {(Object.keys(STATUS_LABELS) as ApplicationStatus[]).map((s) => (
                <option key={s} value={s}>
                  {STATUS_LABELS[s]}
                </option>
              ))}
            </Select>
          }
        >
          {isLoading ? <ApplicationListSkeleton /> : null}
          {error ? <ErrorState error={error} /> : null}
          {applications && applications.length === 0 ? (
            <EmptyState>No applications yet. Shortlist jobs from the feed to start tracking.</EmptyState>
          ) : null}

          {applications && applications.length > 0 ? (
            <VirtualList
              items={applications}
              getKey={(app) => app.id}
              estimateSize={140}
              gap={12}
              renderItem={(app) => <ApplicationCard application={app} />}
            />
          ) : null}
        </Tile>
      </DashboardGrid>
    </div>
  );
}

function ApplicationListSkeleton() {
  return (
    <LoadingRegion label="loading applications…" className="flex flex-col gap-3">
      {Array.from({ length: 5 }).map((_, i) => (
        <div key={i} className="rounded-2xl border border-border bg-surface p-4 shadow-tile">
          <SkeletonLine width="w-2/5" />
          <SkeletonLine width="w-1/3" className="mt-2" />
          <SkeletonBlock className="mt-3 h-5 w-24" />
        </div>
      ))}
    </LoadingRegion>
  );
}

function ApplicationCard({ application }: { application: ApplicationDto }) {
  const update = useUpdateApplication();
  const [editingNotes, setEditingNotes] = useState(false);
  const [notes, setNotes] = useState(application.notes ?? '');

  const status = application.status as ApplicationStatus;
  const tone = STATUS_TONE[status];

  return (
    <div className="rounded-2xl border border-border bg-surface p-4 shadow-tile">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 gap-3">
          <IconTile icon={Briefcase} tint="blue" />
          <div className="min-w-0">
            {application.job ? (
              <Link to={`/jobs/${application.jobId}`} className="font-medium text-accent hover:underline">
                {application.job.title}
              </Link>
            ) : (
              <span className="font-medium text-foreground">Job {application.jobId.slice(0, 8)}</span>
            )}
            {application.job ? (
              <p className="text-sm text-muted">
                {application.job.company}
                {application.job.location ? ` · ${application.job.location}` : ''}
              </p>
            ) : null}
            <div className="mt-1 flex flex-wrap items-center gap-2">
              {tone ? (
                <Chip tone={tone}>{STATUS_LABELS[status] ?? application.status}</Chip>
              ) : (
                <span className="[font:var(--type-caption)] uppercase tracking-[var(--tracking-wide)] text-muted">
                  {STATUS_LABELS[status] ?? application.status}
                </span>
              )}
              {application.appliedAt ? (
                <span className="text-xs text-faint">
                  applied {new Date(application.appliedAt).toLocaleDateString()}
                </span>
              ) : null}
              <span className="text-xs text-faint">
                updated {new Date(application.updatedAt).toLocaleDateString()}
              </span>
            </div>
            {application.events.length > 0 ? (
              <div className="mt-2 flex flex-wrap gap-1">
                {application.events.map((ev, i) => (
                  <span key={i} className="text-xs text-faint">
                    {ev.status} @ {new Date(ev.at).toLocaleDateString()}
                    {i < application.events.length - 1 ? ' →' : ''}
                  </span>
                ))}
              </div>
            ) : null}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <StatusSelect
            status={application.status}
            onChange={(s) => update.mutate({ id: application.id, body: { status: s } })}
            disabled={update.isPending}
          />
          <Button variant="ghost" onClick={() => setEditingNotes((v) => !v)}>
            notes
          </Button>
        </div>
      </div>

      {editingNotes ? (
        <div className="mt-3">
          <Textarea
            className="h-20"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Add notes…"
          />
          <div className="mt-2 flex gap-2">
            <Button
              onClick={() => {
                update.mutate({ id: application.id, body: { notes: notes || null } });
                setEditingNotes(false);
              }}
            >
              save
            </Button>
            <Button variant="ghost" onClick={() => setEditingNotes(false)}>
              cancel
            </Button>
          </div>
        </div>
      ) : null}

      {application.notes && !editingNotes ? (
        <p className="mt-2 text-xs text-muted">{application.notes}</p>
      ) : null}
    </div>
  );
}

function StatusSelect({
  status,
  onChange,
  disabled,
}: {
  status: string;
  onChange: (s: string) => void;
  disabled: boolean;
}) {
  return (
    <Select
      value={status}
      onChange={(e) => onChange(e.target.value)}
      disabled={disabled}
      aria-label="Update status"
      className="text-xs"
    >
      {(Object.keys(STATUS_LABELS) as ApplicationStatus[]).map((s) => (
        <option key={s} value={s}>
          {STATUS_LABELS[s]}
        </option>
      ))}
    </Select>
  );
}
