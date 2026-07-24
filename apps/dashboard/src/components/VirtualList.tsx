import { useVirtualizer } from '@tanstack/react-virtual';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { useRef, useState, type ReactNode } from 'react';
import { cn } from '../lib/utils';

interface VirtualListProps<T> {
  items: T[];
  getKey: (item: T, index: number) => string | number;
  renderItem: (item: T, index: number) => ReactNode;
  estimateSize: number;
  gap?: number;
  className?: string;
  maxHeight?: string;
  /** Above this item count, the list starts collapsed to `collapsedHeight` behind a toggle button. */
  collapseThreshold?: number;
  collapsedHeight?: string;
}

export function VirtualList<T>({
  items,
  getKey,
  renderItem,
  estimateSize,
  gap = 12,
  className,
  maxHeight = '70vh',
  collapseThreshold = 20,
  collapsedHeight = '20rem',
}: VirtualListProps<T>) {
  const parentRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => estimateSize,
    overscan: 8,
    gap,
  });

  const collapsible = items.length > collapseThreshold;
  const [expanded, setExpanded] = useState(false);

  const virtualItems = virtualizer.getVirtualItems();

  return (
    <div>
      <div
        ref={parentRef}
        className={cn('overflow-y-auto', className)}
        style={{ maxHeight: collapsible && !expanded ? collapsedHeight : maxHeight }}
      >
        <div style={{ height: virtualizer.getTotalSize(), position: 'relative', width: '100%' }}>
          {virtualItems.map((virtualRow) => {
            const item = items[virtualRow.index];
            return (
              <div
                key={getKey(item, virtualRow.index)}
                ref={virtualizer.measureElement}
                data-index={virtualRow.index}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  transform: `translateY(${virtualRow.start}px)`,
                }}
              >
                {renderItem(item, virtualRow.index)}
              </div>
            );
          })}
        </div>
      </div>
      {collapsible ? (
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="mt-2 flex w-full items-center justify-center gap-1.5 rounded-md border border-border py-1.5 text-xs font-medium text-muted hover:bg-overlay"
        >
          {expanded ? (
            <>
              <ChevronUp className="h-3.5 w-3.5" aria-hidden="true" />
              show fewer
            </>
          ) : (
            <>
              <ChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
              show all {items.length}
            </>
          )}
        </button>
      ) : null}
    </div>
  );
}
