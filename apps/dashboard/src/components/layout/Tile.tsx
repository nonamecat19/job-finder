import { type ReactNode } from 'react';
import { ScrollShadow } from '@heroui/react/scroll-shadow';
import { cn } from '../../lib/utils';
import { TileSkeleton, TileEmpty, TileError } from './TileStates';

export type SpanRule = 'compact' | 'standard' | 'wide' | 'tall' | 'feature' | 'full';

const SPAN_CLASSES: Record<SpanRule, string> = {
  compact: '',
  standard: '',
  wide: 'sm:col-span-2 lg:col-span-2 3xl:col-span-2 4xl:col-span-2',
  tall: 'sm:row-span-2 lg:row-span-2 3xl:row-span-2 4xl:row-span-2',
  feature: 'sm:col-span-2 lg:col-span-2 3xl:col-span-2 4xl:col-span-3 sm:row-span-2 lg:row-span-2 3xl:row-span-2 4xl:row-span-2',
  full: 'sm:col-span-2 lg:col-span-3 3xl:col-span-4 4xl:col-span-5',
};

const TONE_CLASSES: Record<string, string> = {
  default: 'bg-surface border-border/60',
  inverse: 'bg-foreground text-background border-border-strong',
  quiet: 'bg-background-secondary border-border/40',
};

export type TileProps = {
  title?: ReactNode;
  action?: ReactNode;
  footer?: ReactNode;
  span?: SpanRule;
  tone?: 'default' | 'inverse' | 'quiet';
  scroll?: boolean;
  scrollLabel?: string;
  state?: 'ready' | 'loading' | 'empty' | 'error';
  emptyMessage?: ReactNode;
  error?: unknown;
  onRetry?: () => void;
  children?: ReactNode;
  className?: string;
};

export function Tile({
  title,
  action,
  footer,
  span = 'standard',
  tone = 'default',
  scroll = false,
  scrollLabel,
  state = 'ready',
  emptyMessage,
  error,
  onRetry,
  children,
  className,
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
            <div className="p-4">{children}</div>
          </ScrollShadow>
        ) : (
          <div className="p-4">{children}</div>
        )) : null;
      default:
        return null;
    }
  })();

  return (
    <div
      className={cn(
        'flex min-h-0 flex-col rounded-xl border shadow-lg shadow-black/10',
        'bg-gradient-to-b from-surface to-surface-secondary/80',
        SPAN_CLASSES[span],
        TONE_CLASSES[tone],
        className,
      )}
    >
      {(title !== undefined || action !== undefined) && (
        <div className="flex items-center justify-between gap-2 border-b border-border/60 px-5 py-3.5">
          {title !== undefined && (
            <h2 className="text-sm font-semibold tracking-wide text-foreground/90">{title}</h2>
          )}
          {action !== undefined && <div className="flex items-center gap-1">{action}</div>}
        </div>
      )}
      {body}
      {footer !== undefined && (
        <div className="border-t border-border/60 px-5 py-2.5 text-xs text-muted">{footer}</div>
      )}
    </div>
  );
}
