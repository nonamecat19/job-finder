import { BriefcaseBusiness } from 'lucide-react';
import { Link } from 'react-router-dom';
import { IconTile } from '../../../components/layout';
import { Chip, SkeletonBlock } from '../../../components/ui';
import { useJobDetail } from '../../job-detail/hooks';

// The mock's read-only top bar: the vacancy a run is (or will be) started
// against, shown here and edited only from the job itself. Every run is
// job-backed now, so this always renders from a jobId — before a run exists
// (picked from Feed/Job Detail) and unchanged once it does.
export default function VacancySummaryBar({ jobId }: { jobId: string }) {
  const { data: job, isLoading } = useJobDetail(jobId);

  if (isLoading || !job) {
    return <SkeletonBlock className="h-24 w-full" />;
  }

  return (
    <div className="flex items-start gap-4 rounded-2xl border border-border bg-surface p-4 shadow-tile" data-testid="vacancy-summary-bar">
      <IconTile icon={BriefcaseBusiness} tint="amber" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-2">
          <span className="text-base font-semibold text-foreground">{job.title ?? 'Untitled role'}</span>
          <span className="text-sm text-muted">
            {[job.company, job.location, job.remote ? 'remote' : undefined, job.salaryRaw]
              .filter(Boolean)
              .join(' · ')}
          </span>
        </div>

        <p className="mt-2.5 line-clamp-2 max-w-[78ch] text-sm leading-relaxed text-muted">{job.description}</p>

        <p className="mt-2 text-xs text-faint">
          Read-only — this vacancy came from the job you picked in Feed.{' '}
          <Link to={`/jobs/${job.id}`} className="text-accent hover:underline">
            Edit it there
          </Link>
          .
        </p>
      </div>

      <div className="flex shrink-0 flex-wrap justify-end gap-1">
        {job.matchResult ? <Chip tone="green">fit {job.matchResult.score}</Chip> : null}
        {job.status ? <Chip>{job.status}</Chip> : null}
      </div>
    </div>
  );
}
