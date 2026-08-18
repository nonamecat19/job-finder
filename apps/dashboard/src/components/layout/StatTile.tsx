import { type ReactNode } from 'react';
import { ArrowUp, ArrowDown, type LucideIcon } from 'lucide-react';
import { cn } from '../../lib/utils';
import { Tile, type SpanRule, type TileTone } from './Tile';
import { IconTile, type IconTileTint } from './IconTile';

export type StatTileDelta = {
  value: ReactNode;
  direction: 'up' | 'down';
};

export type StatTileProps = {
  caption: ReactNode;
  value: ReactNode;
  delta?: StatTileDelta;
  icon?: LucideIcon;
  tint?: IconTileTint;
  span?: SpanRule;
  tone?: TileTone;
  className?: string;
};

export function StatTile({ caption, value, delta, icon: Icon, tint = 'blue', span, tone, className }: StatTileProps) {
  return (
    <Tile span={span} tone={tone} className={className}>
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1.5">
          <span className="[font:var(--type-caption)] uppercase tracking-[var(--tracking-wide)] text-muted">
            {caption}
          </span>
          <span className="[font:var(--type-figure)] tracking-[-0.01em] tabular-nums">{value}</span>
          {delta ? (
            <span
              className={cn(
                'inline-flex items-center gap-1 text-xs font-semibold tabular-nums',
                delta.direction === 'up' ? 'text-success' : 'text-danger',
              )}
            >
              {delta.direction === 'up' ? (
                <ArrowUp size={12} strokeWidth={2} aria-hidden="true" />
              ) : (
                <ArrowDown size={12} strokeWidth={2} aria-hidden="true" />
              )}
              {delta.value}
            </span>
          ) : null}
        </div>
        {Icon ? <IconTile icon={Icon} tint={tint} size="md" /> : null}
      </div>
    </Tile>
  );
}
