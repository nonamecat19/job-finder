import { Activity, BriefcaseBusiness, ClipboardList, FileEdit, LayoutPanelLeft, Plug, Rss, Settings2, UserRound, Users } from 'lucide-react';
import { NavLink, useLocation } from 'react-router-dom';
import { ReactNode } from 'react';
import { cn } from '../lib/utils';
import { useMediaQuery } from '../lib/use-media-query';
import { getLayoutMode } from './routes';
import NotificationBell from '../features/notifications/NotificationBell';

const navItems = [
  { to: '/', label: 'Feed', icon: Rss },
  { to: '/tracker', label: 'Tracker', icon: ClipboardList },
  { to: '/tailor', label: 'Tailor', icon: FileEdit },
  { to: '/generate', label: 'Generate', icon: LayoutPanelLeft },
  { to: '/contacts', label: 'Contacts', icon: Users },
  { to: '/status', label: 'Status', icon: Activity },
  { to: '/sources', label: 'Sources', icon: Settings2 },
  { to: '/profile', label: 'Profile', icon: UserRound },
  { to: '/settings', label: 'Settings', icon: Plug },
];

export function AppShell({ children }: { children: ReactNode }) {
  const isMobile = useMediaQuery('(max-width: 767.98px)');
  const location = useLocation();
  const layoutMode = getLayoutMode(location.pathname);

  return (
    <div className="min-h-screen text-foreground">
      <div className="grid min-h-screen gap-3 p-3 md:grid-cols-[16rem_1fr]">
        {!isMobile && (
          <aside className="sticky top-3 hidden h-[calc(100vh-1.5rem)] flex-col rounded-2xl border border-border/60 bg-surface/80 px-5 py-6 shadow-lg shadow-black/10 backdrop-blur-xl md:flex">
            <Brand />
            <nav className="mt-8 space-y-1" aria-label="Primary">
              {navItems.map((item) => (
                <ShellLink key={item.to} {...item} />
              ))}
            </nav>
            <div className="mt-auto flex items-center justify-end px-1">
              <NotificationBell placement="top" />
            </div>
          </aside>
        )}

        <div className="min-w-0">
          {isMobile && (
            <header className="sticky top-0 z-20 border-b border-border bg-background/70 px-4 py-3 backdrop-blur-xl md:hidden">
              <div className="flex items-center justify-between gap-3">
                <Brand compact />
                <div className="flex items-center gap-1">
                  <NotificationBell />
                  <nav className="flex gap-1" aria-label="Primary">
                    {navItems.map((item) => (
                      <ShellLink key={item.to} {...item} compact />
                    ))}
                  </nav>
                </div>
              </div>
            </header>
          )}
          <main
            className={cn(
              'p-4 sm:p-6 lg:p-8',
              layoutMode === 'fit' && 'lg:h-[calc(100dvh-1.5rem)] lg:overflow-hidden',
            )}
          >
            {children}
          </main>
        </div>
      </div>
    </div>
  );
}

function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div className="flex items-center gap-3">
      <div className="grid h-10 w-10 place-items-center rounded-xl bg-accent text-accent-foreground shadow-lg shadow-accent/30">
        <BriefcaseBusiness className="h-5 w-5" aria-hidden="true" />
      </div>
      {!compact && (
        <div>
          <div className="text-lg font-black tracking-tight text-foreground">
            job<span className="text-accent">finder</span>
          </div>
          <p className="text-xs font-medium text-faint">Career operations desk</p>
        </div>
      )}
    </div>
  );
}

function ShellLink({
  to,
  label,
  icon: Icon,
  compact = false,
}: {
  to: string;
  label: string;
  icon: typeof Rss;
  compact?: boolean;
}) {
  return (
    <NavLink
      to={to}
      end={to === '/'}
      className={({ isActive }) =>
        cn(
          'group inline-flex items-center gap-2.5 rounded-lg text-sm font-semibold transition',
          compact ? 'px-2.5 py-2' : 'w-full px-3 py-2.5',
          isActive
            ? 'bg-accent-soft text-foreground ring-1 ring-inset ring-accent/30'
            : 'text-muted hover:bg-surface-tertiary hover:text-foreground',
        )
      }
    >
      {({ isActive }) => (
        <>
          <Icon
            className={cn('h-4 w-4 transition-colors', isActive ? 'text-accent' : 'text-faint group-hover:text-foreground')}
            aria-hidden="true"
          />
          <span className={compact ? 'sr-only sm:not-sr-only' : undefined}>{label}</span>
        </>
      )}
    </NavLink>
  );
}
