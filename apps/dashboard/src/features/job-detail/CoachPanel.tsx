import { Sparkles } from 'lucide-react';
import type { FitGapEvidence, FitGapItem, FitGapProximity } from '@job-finder/shared';
import { SectionTitle } from '../../components/layout/PageHeader';
import { Button, Chip, ErrorState, ScoreBadge, Spinner, Surface } from '../../components/ui';
import { emitToast, toErrorMessage } from '../../lib/toastBus';
import { useAssessCoach, useCoachAssessment } from './hooks';

const PROXIMITY_TONE: Record<FitGapProximity, 'green' | 'red' | 'slate'> = {
  close: 'green',
  moderate: 'slate',
  distant: 'red',
};

/**
 * CoachPanel renders the fit-gap coach (009): "you fail N of M must-haves",
 * then per-gap adjacent evidence from the profile with a grounded rephrase
 * suggestion. Assessment is on-demand (a live LLM call per gap), so the
 * panel starts empty until the user clicks Assess.
 */
export default function CoachPanel({ jobId }: { jobId: string | undefined }) {
  const { data, isLoading, isError: isCachedError } = useCoachAssessment(jobId);
  const assess = useAssessCoach(jobId);

  const handleAssess = () => {
    assess.mutate(undefined, {
      onError: (err) => emitToast({ title: 'Fit-gap assessment failed', description: toErrorMessage(err), variant: 'error' }),
    });
  };

  const assessed = assess.data ?? (isCachedError ? undefined : data);

  return (
    <Surface>
      <div className="mb-3 flex items-center justify-between gap-2">
        <SectionTitle className="mb-0">Fit-gap coach</SectionTitle>
        <Button variant="secondary" onClick={handleAssess} disabled={assess.isPending || isLoading}>
          {assess.isPending ? <Spinner /> : <Sparkles className="h-4 w-4" aria-hidden="true" />}
          {assessed ? 'Re-assess' : 'Assess'}
        </Button>
      </div>

      {isLoading ? (
        <Spinner label="loading fit-gap assessment…" />
      ) : assess.isError ? (
        <ErrorState error={assess.error} />
      ) : !assessed ? (
        <p className="text-sm text-muted">
          Not assessed yet. Click Assess to see which must-haves you're missing and what in your
          profile might already count toward them.
        </p>
      ) : (
        <AssessmentBody assessment={assessed} />
      )}
    </Surface>
  );
}

function AssessmentBody({ assessment }: { assessment: NonNullable<ReturnType<typeof useAssessCoach>['data']> }) {
  const { totalMustHaves, failedMustHaves, coveragePct, gaps } = assessment;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        {totalMustHaves > 0 ? (
          <span className="text-muted">
            {`You fail ${failedMustHaves} of ${totalMustHaves} must-haves`}
          </span>
        ) : (
          <span className="text-muted">No must-haves extracted for this job yet.</span>
        )}
        <ScoreBadge score={Math.round(coveragePct)} />
      </div>

      {gaps.length === 0 ? (
        <p className="text-sm text-muted">No missing must-haves — strong fit.</p>
      ) : (
        <ul className="space-y-3">
          {gaps.map((gap, i) => (
            <GapCard key={`${gap.term}-${i}`} gap={gap} />
          ))}
        </ul>
      )}
    </div>
  );
}

function GapCard({ gap }: { gap: FitGapItem }) {
  return (
    <li className="rounded-lg border border-border bg-overlay/40 p-3">
      <div className="mb-2 flex items-center gap-2">
        <Chip tone="red">{gap.term}</Chip>
      </div>

      {gap.noAdjacentEvidence || gap.adjacentEvidence.length === 0 ? (
        <p className="text-xs italic text-faint">
          No adjacent experience in your profile — add this skill only if you truly have it.
        </p>
      ) : (
        <ul className="space-y-2">
          {gap.adjacentEvidence.map((evidence, i) => (
            <EvidenceRow key={`${evidence.sourceEntry}-${i}`} evidence={evidence} />
          ))}
        </ul>
      )}
    </li>
  );
}

function EvidenceRow({ evidence }: { evidence: FitGapEvidence }) {
  return (
    <li className="rounded-md border border-border bg-elevated/60 p-3">
      <div className="mb-1 flex flex-wrap items-center gap-1.5 text-xs">
        <Chip tone={PROXIMITY_TONE[evidence.proximity]}>{evidence.proximity}</Chip>
        <span className="text-faint">from: {evidence.sourceEntry}</span>
      </div>
      <div className="flex items-start justify-between gap-2">
        <p className="text-sm leading-6 text-fg">{evidence.rephrase}</p>
        <CopyButton text={evidence.rephrase} />
      </div>
      <p className="mt-1 text-xs text-faint">original: {evidence.sourceBullet}</p>
    </li>
  );
}

function CopyButton({ text }: { text: string }) {
  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      emitToast({ title: 'Suggestion copied', variant: 'success' });
    } catch (err) {
      emitToast({ title: 'Copy failed', description: toErrorMessage(err), variant: 'error' });
    }
  };

  return (
    <Button variant="secondary" onClick={onCopy} aria-label="Copy suggestion">
      copy
    </Button>
  );
}
