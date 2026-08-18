import { ListFilter, Play, Trash2 } from 'lucide-react';
import { Link } from 'react-router-dom';
import type { SubscriptionDto } from '@job-finder/shared';
import { summarizeDjinniBasicSearch } from './djinniSearchSummary';
import { ListRow } from '../../components/layout';
import { Button, Chip } from '../../components/ui';

export function SubscriptionRow({ sub, onRun, onDelete, running }: { sub: SubscriptionDto; onRun: () => void; onDelete: () => void; running: boolean }) {

  const isManual = sub.kind === 'manual';
  const basicSearchLabel = !isManual && sub.sourceKey === 'djinni' ? summarizeDjinniBasicSearch(sub.url) : null
  const djinniModeMarker =
    !isManual && sub.sourceKey === 'djinni' && basicSearchLabel !== null ? (
      <span className="text-xs text-faint">· basic-search</span>
    ) : null
  const hasVacancies = (sub.manualCount ?? 0) > 0
  return (
    <ListRow
      title={
        <span className="inline-flex items-center gap-2">
          {basicSearchLabel ?? sub.name ?? sub.sourceKey}
          <span className="text-xs font-normal text-muted">{sub.sourceKey}</span>
          {isManual ? <Chip>manual</Chip> : null}
          {djinniModeMarker}
        </span>
      }
      meta={
        isManual ? (
          <>
            {sub.manualCount ?? 0} added by hand
            {sub.lastAddedAt ? ` · last ${new Date(sub.lastAddedAt).toLocaleString()}` : ''}
          </>
        ) : (
          <>
            {sub.url}
            {sub.lastRunAt ? ` · last run ${new Date(sub.lastRunAt).toLocaleString()}` : ''}
          </>
        )
      }
      aside={
        <div className="flex shrink-0 items-center gap-2">
          <Link
            to={`/?source=${sub.sourceKey}&subscriptionId=${sub.id}`}
            className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-semibold text-muted transition hover:bg-surface-tertiary"
          >
            <ListFilter className="h-3 w-3" /> view jobs
          </Link>
          {isManual ? null : (
            <Button variant="secondary" onClick={onRun} disabled={running}>
              <Play className="h-3 w-3" /> run now
            </Button>
          )}
          <Button
            variant="ghost"
            onClick={onDelete}
            disabled={isManual && hasVacancies}
            title={isManual && hasVacancies ? 'Delete the vacancies it holds first' : undefined}
            aria-label={`Delete ${sub.name ?? sub.sourceKey} subscription`}
          >
            <Trash2 className="h-3 w-3" />
          </Button>
        </div>
      }
    />
  );
}
