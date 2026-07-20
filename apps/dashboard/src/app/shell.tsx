import { Activity, BriefcaseBusiness, ClipboardList, FileEdit, Rss, Settings2, UserRound } from 'lucide-react';
import { NavLink } from 'react-router-dom';
import { ReactNode } from 'react';
import { cn } from '../lib/utils';
import { MD_BREAKPOINT_QUERY, useMediaQuery } from '../lib/use-media-query';

const navItems = [
  { to: '/', label: 'Feed', icon: Rss },
  { to: '/tracker', label: 'Tracker', icon: ClipboardList },
  { to: '/tailor', label: 'Tailor', icon: FileEdit },
  { to: '/status', label: 'Status', icon: Activity },
  { to: '/sources', label: 'Sources', icon: Settings2 },
  { to: '/profile', label: 'Profile', icon: UserRound },
];

export function AppShell({ children }: { children: ReactNode }) {
  // Only ONE nav is mounted at a time. Rendering both (CSS-hidden) duplicated every
  // nav link in the accessibility tree and broke `getByRole('link', { name })` queries.
  const isDesktop = useMediaQuery(MD_BREAKPOINT_QUERY);

  return (
    <div className="min-h-screen text-fg">
      <div className="mx-auto grid min-h-screen max-w-[90rem] md:grid-cols-[16rem_1fr]">
        {isDesktop && (
          <aside className="sticky top-0 flex h-screen flex-col border-r border-border bg-surface/60 px-4 py-6 backdrop-blur-xl">
            <Brand />
            <nav className="mt-8 space-y-1" aria-label="Primary">
              {navItems.map((item) => (
                <ShellLink key={item.to} {...item} />
              ))}
            </nav>
          </aside>
        )}

        <div className="min-w-0">
          {!isDesktop && (
            <header className="sticky top-0 z-20 border-b border-border bg-bg/70 px-4 py-3 backdrop-blur-xl">
              <div className="flex items-center justify-between gap-3">
                <Brand compact />
                <nav className="flex gap-1" aria-label="Primary">
                  {navItems.map((item) => (
                    <ShellLink key={item.to} {...item} compact />
                  ))}
                </nav>
              </div>
            </header>
          )}
          <main className="px-4 py-6 sm:px-6 lg:px-10">{children}</main>
        </div>
      </div>
    </div>
  );
}

function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div className="flex items-center gap-3">
      <div className="grid h-10 w-10 place-items-center rounded-xl bg-gradient-to-br from-primary to-accent text-white shadow-lg shadow-primary/30">
        <BriefcaseBusiness className="h-5 w-5" aria-hidden="true" />
      </div>
      {!compact && (
        <div>
          <div className="text-lg font-black tracking-tight text-fg">
            job<span className="bg-gradient-to-r from-primary to-accent bg-clip-text text-transparent">finder</span>
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
            ? 'bg-primary-soft text-fg ring-1 ring-inset ring-primary/30'
            : 'text-muted hover:bg-overlay hover:text-fg',
        )
      }
    >
      {({ isActive }) => (
        <>
          <Icon
            className={cn('h-4 w-4 transition-colors', isActive ? 'text-primary' : 'text-faint group-hover:text-fg')}
            aria-hidden="true"
          />
          <span className={compact ? 'sr-only sm:not-sr-only' : undefined}>{label}</span>
        </>
      )}
    </NavLink>
  );
}
