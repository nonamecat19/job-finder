import { RefreshCw } from 'lucide-react';
import type { JobSignalDto } from '@job-finder/shared';
import { Button, Chip, Spinner } from '../../components/ui';
import { useRefreshGhostScore } from './hooks';

const LOW_CONFIDENCE_THRESHOLD = 0.5;

function scoreTone(score: number): string {
  if (score >= 80) return 'text-danger';
  if (score >= 50) return 'text-warning';
  return 'text-foreground';
}

interface SignalRowProps {
  label: string;
  value: number | null | undefined;
  note?: string;
}

function SignalRow({ label, value, note }: SignalRowProps) {
  return (
    <div className="flex items-center justify-between gap-2 rounded-lg px-2.5 py-1.5 text-sm">
      <span className="[font:var(--type-caption)] uppercase tracking-[var(--tracking-wide)] text-muted">{label}</span>
      <span className="[font:var(--type-figure-sm)] tabular-nums text-foreground">
        {value !== null && value !== undefined ? (
          value
        ) : (
          <span className="[font:var(--type-body)] italic text-faint" title={note}>
            unknown{note ? ` (${note.replace(/^unknown:\s*/, '')})` : ''}
          </span>
        )}
      </span>
    </div>
  );
}

export default function GhostSignalPanel({ jobId, ghostSignal }: { jobId: string; ghostSignal?: JobSignalDto | null }) {
  const refresh = useRefreshGhostScore(jobId);

  if (!ghostSignal) {
    return (
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <p className="text-sm text-muted">Not scored yet.</p>
        </div>
        <Button variant="secondary" onClick={() => refresh.mutate()} disabled={refresh.isPending}>
          {refresh.isPending ? <Spinner /> : <RefreshCw className="h-4 w-4" />}
          {refresh.isPending ? 'scoring…' : 'score now'}
        </Button>
      </div>
    );
  }

  const { score, model, createdAt, signals } = ghostSignal;
  const lowConfidence = signals.confidence < LOW_CONFIDENCE_THRESHOLD;

  return (
    <>
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <span className={`[font:var(--type-figure-sm)] tabular-nums ${scoreTone(score)}`}>{score}</span>
          <span className="text-xs font-normal text-faint">
            confidence {signals.confidence.toFixed(2)} · {model} · {new Date(createdAt).toLocaleString()}
          </span>
          {lowConfidence ? <Chip tone="slate">low confidence</Chip> : null}
        </div>
        <Button variant="secondary" onClick={() => refresh.mutate()} disabled={refresh.isPending}>
          {refresh.isPending ? <Spinner /> : <RefreshCw className="h-4 w-4" />}
          {refresh.isPending ? 'scoring…' : 'refresh'}
        </Button>
      </div>

      {signals.explanation ? <p className="mb-3 text-sm leading-6 text-muted">{signals.explanation}</p> : null}

      <div className="flex flex-col gap-0.5">
        <SignalRow label="Repost count" value={signals.repostCount} note={signals.notes.repost} />
        <SignalRow label="Days open" value={signals.daysOpen} note={signals.notes.daysOpen} />
        <SignalRow label="Cross-board duplicates" value={signals.crossBoardCount} note={signals.notes.crossBoard} />
        <SignalRow label="Always-hiring count" value={signals.alwaysHiringCount} note={signals.notes.alwaysHiring} />
      </div>

      <p className="mt-3 text-xs text-faint">
        This score is informational only — it never hides, filters, or reorders jobs. It estimates a likelihood, not a
        verdict.
      </p>
    </>
  );
}
