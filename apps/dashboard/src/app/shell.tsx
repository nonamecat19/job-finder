import {
  Activity,
  BriefcaseBusiness,
  ClipboardList,
  FileEdit,
  LayoutPanelLeft,
  Plug,
  Rss,
  Settings2,
  UserRound,
  type LucideIcon,
} from 'lucide-react';
import { NavLink, useLocation } from 'react-router-dom';
import { IconTile, type IconTileTint } from '../components/layout';
import { ReactNode } from 'react';
import { cn } from '../lib/utils';
import { useMediaQuery } from '../lib/use-media-query';
import { getLayoutMode } from './routes';
import NotificationBell from '../features/notifications/NotificationBell';

const TRANSITION = 'transition-[color,background-color,border-color,box-shadow] duration-[160ms] ease-[cubic-bezier(0.2,0,0.2,1)]';

type NavItem = { to: string; label: string; icon: LucideIcon; tint: IconTileTint };

/** One tint per destination, assigned once and kept — the tints are how the nav
    reads at a glance, so they never rotate with state. */
const navItems: NavItem[] = [
  { to: '/', label: 'Feed', icon: Rss, tint: 'violet' },
  { to: '/tracker', label: 'Tracker', icon: ClipboardList, tint: 'blue' },
  { to: '/tailor', label: 'Tailor', icon: FileEdit, tint: 'mint' },
  { to: '/generate', label: 'Generate', icon: LayoutPanelLeft, tint: 'amber' },
  { to: '/status', label: 'Status', icon: Activity, tint: 'violet' },
  { to: '/sources', label: 'Sources', icon: Settings2, tint: 'blue' },
  { to: '/profile', label: 'Profile', icon: UserRound, tint: 'mint' },
  { to: '/settings', label: 'Settings', icon: Plug, tint: 'amber' },
];

export function AppShell({ children }: { children: ReactNode }) {
  const isMobile = useMediaQuery('(max-width: 767.98px)');
  const location = useLocation();
  const layoutMode = getLayoutMode(location.pathname);

  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="flex min-h-screen gap-3 p-3 md:p-3">
        {/* One sidebar: a floating white nav panel, per the design system's own
            shell. The canvas shows around it — that gap is the composition. */}
        {!isMobile && (
          <aside className="sticky top-3 hidden h-[calc(100vh-1.5rem)] w-[var(--width-sidebar)] shrink-0 flex-col rounded-2xl border border-border bg-surface px-[1.125rem] py-5 shadow-tile md:flex">
            <Brand />
            <nav className="mt-7 flex-1 space-y-0.5" aria-label="Primary">
              {navItems.map((item) => (
                <PanelLink key={item.to} {...item} />
              ))}
            </nav>
            <div className="flex items-center justify-end px-1">
              <NotificationBell placement="top" />
            </div>
          </aside>
        )}

        <div className="min-w-0 flex-1">
          {isMobile && (
            <header className="sticky top-0 z-20 border-b border-border bg-surface/95 px-4 py-3 backdrop-blur-xl md:hidden">
              <div className="flex items-center justify-between gap-3">
                <Brand compact />
                <NotificationBell />
              </div>
              <nav className="mt-3 flex gap-1 overflow-x-auto" aria-label="Primary">
                {navItems.map((item) => (
                  <MobileLink key={item.to} {...item} />
                ))}
              </nav>
            </header>
          )}
          <main
            className={cn(
              'mx-auto w-full max-w-[var(--container-dashboard)] px-1 py-2 lg:px-5 lg:py-4',
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
      <div className="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-accent text-accent-foreground shadow-brand">
        <BriefcaseBusiness className="h-5 w-5" aria-hidden="true" strokeWidth={2} />
      </div>
      {!compact && (
        <div>
          <div className="text-lg font-extrabold tracking-tight text-foreground">
            job<span className="text-accent">finder</span>
          </div>
          <p className="text-xs font-medium text-faint">Career operations desk</p>
        </div>
      )}
    </div>
  );
}

function PanelLink({ to, label, icon, tint }: NavItem) {
  return (
    <NavLink
      to={to}
      end={to === '/'}
      className={({ isActive }) =>
        cn(
          'group flex items-center gap-2.5 rounded-lg py-1.5 pr-3 pl-1.5 text-sm font-semibold',
          TRANSITION,
          isActive ? 'bg-surface-tertiary text-foreground' : 'text-muted hover:bg-surface-secondary hover:text-foreground',
        )
      }
    >
      {({ isActive }) => (
        <>
          <IconTile
            icon={icon}
            tint={tint}
            size="sm"
            className={cn(TRANSITION, isActive ? 'shadow-card' : 'opacity-80 group-hover:opacity-100')}
          />
          <span>{label}</span>
        </>
      )}
    </NavLink>
  );
}

function MobileLink({ to, label, icon: Icon }: NavItem) {
  return (
    <NavLink
      to={to}
      end={to === '/'}
      className={({ isActive }) =>
        cn(
          'inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius-control)] px-2.5 py-2 text-sm font-semibold',
          TRANSITION,
          isActive ? 'bg-accent-soft text-foreground' : 'text-muted hover:bg-surface-tertiary hover:text-foreground',
        )
      }
    >
      {({ isActive }) => (
        <>
          <Icon
            className={cn('h-[18px] w-[18px]', TRANSITION, isActive ? 'text-accent' : 'text-faint')}
            aria-hidden="true"
            strokeWidth={2}
          />
          <span className="sr-only sm:not-sr-only">{label}</span>
        </>
      )}
    </NavLink>
  );
}
