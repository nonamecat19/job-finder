import { Check, Search, X } from 'lucide-react';
import type { BoardCandidateDto } from '@job-finder/shared';
import {
  Button,
  Chip,
  EmptyState,
  ErrorState,
  LoadingRegion,
  SkeletonBlock,
} from '../../../components/ui';
import { useAcceptCandidate, useCandidates, useDiscoverCandidates, useRejectCandidate } from '../hooks';

export default function CandidatesPanel() {
  const { data, isLoading, error } = useCandidates();
  const discover = useDiscoverCandidates();
  const accept = useAcceptCandidate();
  const reject = useRejectCandidate();

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <Button onClick={() => discover.mutate()} disabled={discover.isPending}>
          <Search className="h-3 w-3" /> discover
        </Button>
      </div>

      {discover.data ? (
        <div className="mb-3 rounded-md border border-success/30 bg-success-soft p-2 text-xs text-success">
          Discovered {discover.data.newCandidates} new candidate{discover.data.newCandidates !== 1 ? 's' : ''}.
        </div>
      ) : null}

      {isLoading ? (
        <LoadingRegion label="loading candidates…" className="space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <SkeletonBlock key={i} className="h-12 w-full" />
          ))}
        </LoadingRegion>
      ) : null}
      {error ? <ErrorState error={error} /> : null}
      {data && data.candidates.length === 0 && !discover.data ? (
        <EmptyState>No board candidates. Click "discover" to scan existing jobs for potential boards.</EmptyState>
      ) : null}

      <ul className="space-y-2">
        {data?.candidates.map((c) => (
          <CandidateRow
            key={c.id}
            candidate={c}
            onAccept={() => accept.mutate(c.id)}
            onReject={() => reject.mutate(c.id)}
          />
        ))}
      </ul>
    </div>
  );
}

function CandidateRow({
  candidate: c,
  onAccept,
  onReject,
}: {
  candidate: BoardCandidateDto;
  onAccept: () => void;
  onReject: () => void;
}) {
  return (
    <li className="flex flex-col gap-2 rounded-md border border-border bg-surface-secondary/60 p-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="font-medium text-foreground">{c.displayName}</span>
          <Chip>{c.vendor}</Chip>
          <Chip tone={c.state === 'proposed' ? 'slate' : c.state === 'accepted' ? 'green' : 'red'}>
            {c.state}
          </Chip>
        </div>
        <div className="mt-1 text-xs text-muted">
          <span>{c.employerIdentifier}</span>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button variant="secondary" onClick={onAccept}>
          <Check className="h-3 w-3" /> accept
        </Button>
        <Button variant="ghost" onClick={onReject}>
          <X className="h-3 w-3" /> reject
        </Button>
      </div>
    </li>
  );
}
