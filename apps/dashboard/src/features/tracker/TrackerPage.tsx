import { BarChart3 } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import type { ApplicationDto, ApplicationStatus, StatsDto } from '@job-finder/shared';
import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import { DashboardGrid, GridCard } from '../../components/layout';
import { VirtualList } from '../../components/VirtualList';
import {
  Button,
  Chip,
  EmptyState,
  ErrorState,
  Field,
  LoadingRegion,
  Select,
  SkeletonBlock,
  SkeletonLine,
  Surface,
  Textarea,
} from '../../components/ui';
import { useApplications, useStats, useUpdateApplication } from './hooks';

const STATUS_LABELS: Record<ApplicationStatus, string> = {
  found: 'Found',
  shortlisted: 'Shortlisted',
  docs_generated: 'Docs generated',
  applied: 'Applied',
  interview: 'Interview',
  offer: 'Offer',
  rejected: 'Rejected',
};

const STATUS_COLORS: Record<ApplicationStatus, string> = {
  found: 'slate',
  shortlisted: 'blue',
  docs_generated: 'purple',
  applied: 'sky',
  interview: 'amber',
  offer: 'emerald',
  rejected: 'rose',
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
        actions={
          stats ? <StatsSummary stats={stats} /> : null
        }
      />

      <DashboardGrid>
        <GridCard span="full">

      <Surface className="mb-5">
        <Field label="Filter by status">
          <Select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="w-full sm:w-48"
          >
            <option value="">all statuses</option>
            {(Object.keys(STATUS_LABELS) as ApplicationStatus[]).map((s) => (
              <option key={s} value={s}>
                {STATUS_LABELS[s]}
              </option>
            ))}
          </Select>
        </Field>
      </Surface>

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
        </GridCard>
      </DashboardGrid>
    </div>
  );
}

function ApplicationListSkeleton() {
  return (
    <LoadingRegion label="loading applications…" className="flex flex-col gap-3">
      {Array.from({ length: 5 }).map((_, i) => (
        <div key={i} className="rounded-xl border border-border bg-surface p-4 shadow-sm shadow-black/20">
          <SkeletonLine width="w-2/5" />
          <SkeletonLine width="w-1/3" className="mt-2" />
          <SkeletonBlock className="mt-3 h-5 w-24" />
        </div>
      ))}
    </LoadingRegion>
  );
}

function StatsSummary({ stats }: { stats: StatsDto }) {
  return (
    <div className="flex items-center gap-3 text-sm text-muted">
      <BarChart3 className="h-4 w-4" />
      <span>{stats.jobsTotal} total</span>
      <span>{stats.highFit} high fit</span>
      <span>{stats.jobsLast24h} new today</span>
    </div>
  );
}

function ApplicationCard({ application }: { application: ApplicationDto }) {
  const update = useUpdateApplication();
  const [editingNotes, setEditingNotes] = useState(false);
  const [notes, setNotes] = useState(application.notes ?? '');

  const statusColor = (STATUS_COLORS[application.status as ApplicationStatus] ?? 'slate') as
    | 'green' | 'red' | 'slate';

  return (
    <Surface>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          {application.job ? (
            <Link
              to={`/jobs/${application.jobId}`}
              className="font-semibold text-primary hover:underline"
            >
              {application.job.title}
            </Link>
          ) : (
            <span className="font-semibold text-fg">Job {application.jobId.slice(0, 8)}</span>
          )}
          {application.job ? (
            <p className="text-sm text-muted">
              {application.job.company}
              {application.job.location ? ` · ${application.job.location}` : ''}
            </p>
          ) : null}
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <Chip tone={statusColor}>{STATUS_LABELS[application.status as ApplicationStatus] ?? application.status}</Chip>
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
    </Surface>
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
