import { type ReactNode } from 'react';
import { cn } from '../../lib/utils';

export type LayoutMode = 'flow' | 'fit';

export function DashboardGrid({
  variant = 'flow',
  children,
  className,
}: {
  variant?: LayoutMode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        'mx-auto grid w-full max-w-[var(--container-dashboard)] gap-4 sm:gap-5 lg:gap-6',
        'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 3xl:grid-cols-4 4xl:grid-cols-5',
        variant === 'fit' ? 'auto-rows-fr items-stretch lg:h-full' : 'auto-rows-min items-start',
        className,
      )}
    >
      {children}
    </div>
  );
}
