import { type ReactNode } from 'react';
import { Skeleton } from '@heroui/react/skeleton';
import { Button } from '@heroui/react/button';
import { cn } from '../../lib/utils';

export function TileSkeleton({ lines = 3, className }: { lines?: number; className?: string }) {
  return (
    <div className={cn('flex flex-col gap-3 p-5', className)}>
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton key={i} className="h-4 w-full rounded" />
      ))}
    </div>
  );
}

export function TileEmpty({ children, icon }: { children: ReactNode; icon?: ReactNode }) {
  return (
    // No wrapper element around `children`: an empty state is often more than
    // a sentence (a paragraph plus an action), and a <p> around that is
    // invalid markup. A bare string still picks up the text styling here.
    <div className="flex flex-1 flex-col items-center justify-center gap-2 p-5 text-center text-sm text-muted">
      {icon}
      {children}
    </div>
  );
}

export function TileError({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const message = error instanceof Error ? error.message : String(error ?? 'An error occurred');
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 p-5">
      <p className="text-sm text-danger">{message}</p>
      {onRetry && (
        <Button variant="ghost" size="sm" onPress={onRetry}>
          Retry
        </Button>
      )}
    </div>
  );
}
