import { type ReactNode } from 'react';
import { ScrollShadow } from '@heroui/react/scroll-shadow';
import { cn } from '../../lib/utils';
import { TileSkeleton, TileEmpty, TileError } from './TileStates';

export type SpanRule = 'compact' | 'standard' | 'wide' | 'tall' | 'feature' | 'full';
export type TileTone = 'default' | 'quiet' | 'inverse';

const SPAN_CLASSES: Record<SpanRule, string> = {
  compact: '',
  standard: '',
  wide: 'sm:col-span-2 lg:col-span-2 3xl:col-span-2',
  tall: 'sm:row-span-2 lg:row-span-2 3xl:row-span-2',
  feature: 'sm:col-span-2 lg:col-span-2 3xl:col-span-2 sm:row-span-2 lg:row-span-2 3xl:row-span-2',
  full: 'sm:col-span-2 lg:col-span-3 3xl:col-span-4',
};

const TONE_CLASSES: Record<TileTone, string> = {
  default: 'bg-surface text-foreground shadow-tile',
  quiet: 'bg-surface-secondary text-foreground',
  inverse: 'bg-foreground text-background',
};

export type TileProps = {
  title?: ReactNode;
  action?: ReactNode;
  footer?: ReactNode;
  span?: SpanRule;
  tone?: TileTone;

  interactive?: boolean;
  scroll?: boolean;
  scrollLabel?: string;
  state?: 'ready' | 'loading' | 'empty' | 'error';
  emptyMessage?: ReactNode;
  error?: unknown;
  onRetry?: () => void;
  children?: ReactNode;
  className?: string;

  contentClassName?: string;
};

export function Tile({
  title,
  action,
  footer,
  span = 'standard',
  tone = 'default',
  interactive = false,
  scroll = false,
  scrollLabel,
  state = 'ready',
  emptyMessage,
  error,
  onRetry,
  children,
  className,
  contentClassName,
}: TileProps) {
  const body = (() => {
    switch (state) {
      case 'loading':
        return <TileSkeleton />;
      case 'empty':
        return <TileEmpty>{emptyMessage ?? 'No data'}</TileEmpty>;
      case 'error':
        return <TileError error={error} onRetry={onRetry} />;
      case 'ready':
        return children !== undefined ? (scroll ? (
          <ScrollShadow
            className="flex-1 overflow-y-auto"
            tabIndex={0}
            role="region"
            aria-label={scrollLabel ?? 'Scrollable content'}
          >
            <div className="p-5">{children}</div>
          </ScrollShadow>
        ) : (
          <div className={cn('p-5', contentClassName)}>{children}</div>
        )) : null;
      default:
        return null;
    }
  })();

  return (
    <div
      className={cn(
        'flex min-h-0 flex-col rounded-2xl border border-border',
        'transition-[color,background-color,border-color,box-shadow] duration-[160ms] ease-[cubic-bezier(0.2,0,0.2,1)]',
        SPAN_CLASSES[span],
        TONE_CLASSES[tone],
        interactive && 'cursor-pointer hover:-translate-y-0.5 hover:shadow-raise motion-safe:transition-transform',
        className,
      )}
    >
      {(title !== undefined || action !== undefined) && (
        <div className="flex items-center justify-between gap-2 px-5 py-4">
          {title !== undefined && (
            <h2 className="truncate [font:var(--type-caption)] uppercase tracking-[var(--tracking-wide)] text-muted">
              {title}
            </h2>
          )}
          {action !== undefined && <div className="flex shrink-0 items-center gap-1">{action}</div>}
        </div>
      )}
      {body}
      {footer !== undefined && (
        <div className="px-5 py-3 text-xs text-muted">{footer}</div>
      )}
    </div>
  );
}
