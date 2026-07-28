import { type ReactNode } from 'react';
import { cn } from '../../lib/utils';

const spanClasses: Record<string, string> = {
  narrow: '',
  wide: 'md:col-span-2 lg:col-span-2',
  full: 'md:col-span-2 lg:col-span-3',
};

export function GridCard({
  span = 'narrow',
  children,
  className,
}: {
  span?: 'narrow' | 'wide' | 'full';
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn('min-w-0', spanClasses[span], className)}>
      {children}
    </div>
  );
}
