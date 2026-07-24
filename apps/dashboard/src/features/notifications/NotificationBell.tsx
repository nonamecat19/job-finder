import { Bell, Check, ExternalLink } from 'lucide-react';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Button, Chip, LoadingRegion, SkeletonLine, Surface } from '../../components/ui';
import { useMarkNotificationSeen, useNotifications, useUnseenNotificationCount } from './hooks';

export default function NotificationBell() {
  const [open, setOpen] = useState(false);

  const { data: countData } = useUnseenNotificationCount();
  const { data: notifications, isLoading } = useNotifications(open);
  const markSeen = useMarkNotificationSeen();

  const unseen = countData?.count ?? 0;

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="relative inline-flex items-center justify-center rounded-lg p-2 text-muted hover:bg-overlay hover:text-fg transition"
        aria-label={`Notifications${unseen > 0 ? ` (${unseen} unread)` : ''}`}
      >
        <Bell className="h-5 w-5" aria-hidden="true" />
        {unseen > 0 ? (
          <span className="absolute -right-0.5 -top-0.5 flex h-4 min-w-[1rem] items-center justify-center rounded-full bg-danger px-1 text-[10px] font-bold leading-none text-white">
            {unseen > 99 ? '99+' : unseen}
          </span>
        ) : null}
      </button>

      {open ? (
        <>
          <div className="fixed inset-0 z-30" onClick={() => setOpen(false)} />
          <div className="absolute right-0 z-40 mt-2 w-80 rounded-xl border border-border bg-surface shadow-xl shadow-black/30">
            <div className="flex items-center justify-between border-b border-border px-4 py-3">
              <h3 className="text-sm font-semibold text-fg">Notifications</h3>
              <button
                onClick={() => setOpen(false)}
                className="text-xs text-muted hover:text-fg"
              >
                close
              </button>
            </div>
            <div className="max-h-80 overflow-y-auto">
              {isLoading ? (
                <LoadingRegion label="loading notifications…" className="space-y-2 p-4">
                  <SkeletonLine width="w-3/4" />
                  <SkeletonLine width="w-1/2" />
                  <SkeletonLine width="w-2/3" />
                </LoadingRegion>
              ) : notifications && notifications.length > 0 ? (
                notifications.map((n) => (
                  <div
                    key={n.id}
                    className={`flex items-start gap-3 border-b border-border/50 px-4 py-3 text-sm transition ${
                      n.seen ? 'opacity-60' : 'bg-primary-soft/20'
                    }`}
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        {n.fresh ? (
                          <Chip tone="green">fresh</Chip>
                        ) : null}
                        <span className="font-medium text-fg truncate">
                          {n.jobTitle ?? 'Job match'}
                        </span>
                      </div>
                      {n.company ? (
                        <p className="mt-0.5 text-xs text-muted">{n.company}</p>
                      ) : null}
                      {n.matchScore != null ? (
                        <p className="mt-0.5 text-xs text-faint">
                          Match score: {n.matchScore}
                        </p>
                      ) : null}
                      <p className="mt-0.5 text-xs text-faint">
                        {new Date(n.createdAt).toLocaleDateString()}
                      </p>
                    </div>
                    <div className="flex shrink-0 flex-col gap-1">
                      {!n.seen ? (
                        <Button
                          variant="ghost"
                          onClick={() => markSeen.mutate(n.id)}
                          className="!p-1"
                          aria-label="Mark as seen"
                        >
                          <Check className="h-3.5 w-3.5" />
                        </Button>
                      ) : null}
                      <Link
                        to={`/jobs/${n.jobId}`}
                        className="inline-flex rounded-md p-1 text-muted hover:bg-overlay hover:text-fg"
                        aria-label="View job"
                        onClick={() => setOpen(false)}
                      >
                        <ExternalLink className="h-3.5 w-3.5" />
                      </Link>
                    </div>
                  </div>
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
