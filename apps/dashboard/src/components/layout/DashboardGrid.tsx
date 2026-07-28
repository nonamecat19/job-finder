import { type ReactNode } from 'react';
import { cn } from '../../lib/utils';

export function DashboardGrid({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        'grid gap-3 md:grid-cols-2 lg:grid-cols-3 lg:gap-5 items-start',
        className,
      )}
    >
      {children}
    </div>
  );
}
