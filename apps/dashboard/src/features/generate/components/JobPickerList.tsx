import { useQuery } from '@tanstack/react-query';
import { ListRow } from '../../../components/layout';
import { ScoreBadge, SkeletonLine } from '../../../components/ui';
import { api } from '../../../lib/api';
import { queryKeys } from '../../../lib/queryKeys';

const FILTERS = { sort: 'score' as const };

// A compact, click-to-pick substitute for the ad-hoc vacancy field this page
// used to have: since every run is job-backed now, picking a job is the
// first thing to do here, not a detour through Feed. Same source Feed uses
// (api.jobs.list), rendered as tight ListRows rather than Feed's full row
// (no shortlist/hide/open actions — this list only picks).
export default function JobPickerList({ onPick }: { onPick: (jobId: string) => void }) {
  const { data, isLoading } = useQuery({
    queryKey: queryKeys.jobs.picker(FILTERS),
    queryFn: () => api.jobs.list(FILTERS),
  });

  if (isLoading) {
    return (
      <div className="space-y-1.5">
        {Array.from({ length: 6 }).map((_, i) => (
          <SkeletonLine key={i} className="h-10 w-full" />
        ))}
      </div>
    );
  }

  const jobs = data?.items ?? [];

  if (jobs.length === 0) {
    return <p className="text-sm text-muted">No jobs in Feed yet.</p>;
  }

  return (
    <div className="w-full max-w-2xl space-y-1" data-testid="job-picker-list">
      {jobs.map((job) => (
        <ListRow
          key={job.id}
          leading={<ScoreBadge score={job.matchResult?.score} />}
          title={job.title}
          meta={[job.company, job.location, job.remote ? 'remote' : undefined].filter(Boolean).join(' · ')}
          aside={job.status}
          onClick={() => onPick(job.id)}
        />
      ))}
    </div>
  );
}
