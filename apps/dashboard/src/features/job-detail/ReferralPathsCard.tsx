import { ArrowRight, Users } from 'lucide-react';
import type { ReferralContactDto, ReferralPathDto } from '@job-finder/shared';
import { Chip, LoadingRegion, SkeletonLine } from '../../components/ui';
import { useReferralPaths } from './hooks';

function strengthTone(score: number): 'green' | 'red' | 'slate' {
  if (score >= 0.6) return 'green';
  if (score >= 0.3) return 'slate';
  return 'red';
}

function strengthLabel(score: number): string {
  if (score >= 0.6) return 'strong';
  if (score >= 0.3) return 'moderate';
  return 'weak';
}

export default function ReferralPathsCard({ jobId }: { jobId: string | undefined }) {
  const { data: paths, isLoading, isError } = useReferralPaths(jobId);

  if (isLoading) {
    return (
      <LoadingRegion label="looking for warm paths…" className="space-y-2">
        <SkeletonLine width="w-2/3" />
        <SkeletonLine width="w-1/2" />
      </LoadingRegion>
    );
  }

  // A 404/no-company job (or the referral graph being empty) both surface
  // as "nothing to show" rather than an error banner — this card is
  // advisory, not a required part of the job-detail flow.
  if (isError || !paths || paths.length === 0) return null;

  return (
    <>
      <div className="mb-3 flex items-center gap-2">
        <Users className="h-4 w-4 text-faint" aria-hidden="true" />
        <span className="text-xs text-muted">{paths.length} warm {paths.length === 1 ? 'path' : 'paths'} found</span>
      </div>
      <ul className="space-y-2">
        {paths.map((path, i) => (
          <ReferralPathRow key={i} path={path} />
        ))}
      </ul>
    </>
  );
}

function ReferralPathRow({ path }: { path: ReferralPathDto }) {
  return (
    <li className="rounded-md border border-border bg-surface-secondary/60 p-3 text-sm">
      <div className="flex flex-wrap items-center gap-1.5">
        {path.path.map((contact, i) => (
          <ContactChip key={contact.id} contact={contact} isLast={i === path.path.length - 1} />
        ))}
      </div>
      <div className="mt-2 flex items-center gap-2 text-xs text-faint">
        <Chip tone={strengthTone(path.score)}>{strengthLabel(path.score)} · {path.score.toFixed(2)}</Chip>
        <span>{path.length - 1} {path.length - 1 === 1 ? 'hop' : 'hops'}</span>
      </div>
    </li>
  );
}

function ContactChip({ contact, isLast }: { contact: ReferralContactDto; isLast: boolean }) {
  return (
    <span className="flex items-center gap-1.5">
      <span className="rounded-md bg-surface-tertiary/60 px-2 py-1 font-medium text-foreground">
        {contact.name}
        {contact.role ? <span className="ml-1 font-normal text-faint">· {contact.role}</span> : null}
      </span>
      {!isLast ? <ArrowRight className="h-3.5 w-3.5 text-faint" aria-hidden="true" /> : null}
    </span>
  );
}
