import { ListFilter, Play, Trash2 } from 'lucide-react';
import { Link } from 'react-router-dom';
import type { SubscriptionDto } from '@job-finder/shared';
import { summarizeDjinniBasicSearch } from './djinniSearchSummary';
import { Button, Chip } from '../../components/ui';

export function SubscriptionRow({ sub, onRun, onDelete, running }: { sub: SubscriptionDto; onRun: () => void; onDelete: () => void; running: boolean }) {
  // A manual subscription has nothing to crawl and nothing to schedule, so it
  // renders read-only: no cron, no run control, and delete disabled while its
  // hand-added vacancies still reference it (FR-014, FR-016).
  const isManual = sub.kind === 'manual';
  const basicSearchLabel = !isManual && sub.sourceKey === 'djinni' ? summarizeDjinniBasicSearch(sub.url) : null
  const djinniModeMarker =
    !isManual && sub.sourceKey === 'djinni' && basicSearchLabel !== null ? (
      <span className="ml-1 text-xs text-faint">· basic-search</span>
    ) : null
  const hasVacancies = (sub.manualCount ?? 0) > 0
  return (
    <li className="flex flex-col gap-2 rounded-md border border-border bg-surface-secondary/60 p-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <span className="font-medium text-foreground">{basicSearchLabel ?? sub.name ?? sub.sourceKey}</span>
        <span className="ml-2 text-xs text-muted">{sub.sourceKey}</span>
        {isManual ? <Chip>manual</Chip> : null}
        {djinniModeMarker}
        {isManual ? (
          <div className="text-xs text-faint">
            {sub.manualCount ?? 0} added by hand
            {sub.lastAddedAt ? ` · last ${new Date(sub.lastAddedAt).toLocaleString()}` : ''}
          </div>
        ) : (
          <>
            <div className="truncate text-xs text-faint" title={sub.url}>{sub.url}</div>
            {sub.lastRunAt ? (
              <span className="mr-2 text-xs text-faint">
                last run {new Date(sub.lastRunAt).toLocaleString()}
              </span>
            ) : null}
          </>
        )}
      </div>
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
    </li>
  );
}
