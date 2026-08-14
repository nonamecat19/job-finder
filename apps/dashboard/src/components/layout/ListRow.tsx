import { type KeyboardEvent, type ReactNode } from 'react';
import { cn } from '../../lib/utils';

export type ListRowProps = {
  leading?: ReactNode;
  title: ReactNode;
  meta?: ReactNode;
  aside?: ReactNode;
  selected?: boolean;
  onClick?: () => void;
  className?: string;
};

/**
 * A row inside a scrolling tile. Rows sit flush with no dividers between
 * them; selection is a --surface-tertiary fill, never an accent border.
 */
export function ListRow({ leading, title, meta, aside, selected = false, onClick, className }: ListRowProps) {
  const interactive = typeof onClick === 'function';

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!interactive) return;
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onClick?.();
    }
  };

  return (
    <div
      role={interactive ? 'button' : undefined}
      tabIndex={interactive ? 0 : undefined}
      aria-current={selected || undefined}
      onClick={onClick}
      onKeyDown={handleKeyDown}
      className={cn(
        'flex w-full items-start gap-3 rounded-xl p-3 text-left',
        'transition-[color,background-color] duration-[160ms] ease-[cubic-bezier(0.2,0,0.2,1)]',
        interactive && 'cursor-pointer outline-none focus-visible:ring-[3px] focus-visible:ring-accent-soft',
        selected ? 'bg-surface-tertiary' : interactive && 'hover:bg-surface-tertiary',
        className,
      )}
    >
      {leading ? <div className="flex shrink-0 items-center">{leading}</div> : null}
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="truncate text-sm font-semibold text-foreground">{title}</div>
        {meta ? <div className="truncate text-xs text-muted">{meta}</div> : null}
      </div>
      {aside ? (
        <div className="flex shrink-0 items-center gap-2 font-mono text-xs text-faint">{aside}</div>
      ) : null}
    </div>
  );
}
