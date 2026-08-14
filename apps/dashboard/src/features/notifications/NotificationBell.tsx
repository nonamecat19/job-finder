import { Bell, Check, ExternalLink } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { IconTile, ListRow } from '../../components/layout';
import { Button, Chip, LoadingRegion, SkeletonLine } from '../../components/ui';
import { cn } from '../../lib/utils';
import { useMarkNotificationSeen, useNotifications, useUnseenNotificationCount } from './hooks';

export default function NotificationBell({ placement = 'bottom' }: { placement?: 'top' | 'bottom' }) {
  const [open, setOpen] = useState(false);

  const { data: countData } = useUnseenNotificationCount();
  const { data: notifications, isLoading } = useNotifications(open);
  const markSeen = useMarkNotificationSeen();

  const unseen = countData?.count ?? 0;

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="relative inline-flex items-center justify-center rounded-lg p-2 text-muted transition-[color,background-color] duration-[160ms] hover:bg-surface-tertiary hover:text-foreground"
        aria-label={`Notifications${unseen > 0 ? ` (${unseen} unread)` : ''}`}
      >
        <Bell className="h-5 w-5" aria-hidden="true" />
        {unseen > 0 ? (
          <span className="absolute -right-0.5 -top-0.5 flex h-4 min-w-[1rem] items-center justify-center rounded-full bg-danger px-1 text-[10px] font-bold leading-none text-danger-foreground">
            {unseen > 99 ? '99+' : unseen}
          </span>
        ) : null}
      </button>

      {open ? (
        <>
          <div className="fixed inset-0 z-30" onClick={() => setOpen(false)} />
          <div
            className={cn(
              'absolute right-0 z-40 w-80 overflow-hidden rounded-2xl border border-border bg-surface shadow-raise',
              placement === 'top' ? 'bottom-full mb-2' : 'mt-2',
            )}
          >
            <div className="flex items-center justify-between px-4 py-3">
              <h3 className="[font:var(--type-tile-title)] text-foreground">Notifications</h3>
              <button
                onClick={() => setOpen(false)}
                className="text-xs text-muted hover:text-foreground"
              >
                close
              </button>
            </div>
            <div className="max-h-80 overflow-y-auto px-2 pb-2">
              {isLoading ? (
                <LoadingRegion label="loading notifications…" className="space-y-2 px-2 py-2">
                  <SkeletonLine width="w-3/4" />
                  <SkeletonLine width="w-1/2" />
                  <SkeletonLine width="w-2/3" />
                </LoadingRegion>
              ) : notifications && notifications.length > 0 ? (
                notifications.map((n) => (
                  <ListRow
                    key={n.id}
                    className={cn(!n.seen && 'bg-accent-quiet hover:bg-accent-quiet')}
                    leading={<IconTile icon={Bell} tint={n.fresh ? 'amber' : 'blue'} size="sm" />}
                    title={
                      <span className="flex items-center gap-2">
                        {n.fresh ? <Chip tone="green">fresh</Chip> : null}
                        <span className="truncate">{n.jobTitle ?? 'Job match'}</span>
                      </span>
                    }
                    meta={
                      <span className="flex flex-col gap-0.5">
                        {n.company ? <span>{n.company}</span> : null}
                        {n.matchScore != null ? <span>Match score: {n.matchScore}</span> : null}
                        <span className="[font:var(--type-caption)] text-faint">
                          {new Date(n.createdAt).toLocaleDateString()}
                        </span>
                      </span>
                    }
                    aside={
                      <div className="flex shrink-0 flex-col gap-1">
                        {!n.seen ? (
                          <Button
                            variant="ghost"
                            onClick={() => markSeen.mutate(n.id)}
                            className="!h-7 !w-7 !p-0"
                            aria-label="Mark as seen"
                          >
                            <Check className="h-3.5 w-3.5" />
                          </Button>
                        ) : null}
                        <Link
                          to={`/jobs/${n.jobId}`}
                          className="inline-flex items-center justify-center rounded-md p-1 text-muted hover:bg-surface-tertiary hover:text-foreground"
                          aria-label="View job"
                          onClick={() => setOpen(false)}
                        >
                          <ExternalLink className="h-3.5 w-3.5" />
                        </Link>
                      </div>
                    }
                  />
                ))
              ) : (
                <p className="p-4 text-sm text-muted">No notifications yet.</p>
              )}
            </div>
          </div>
        </>
      ) : null}
    </div>
  );
}
