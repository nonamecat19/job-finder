import { type LucideIcon } from 'lucide-react';
import { cn } from '../../lib/utils';

export type IconTileTint = 'violet' | 'blue' | 'mint' | 'amber' | 'rose';
export type IconTileSize = 'sm' | 'md' | 'lg';

const TINT_CLASSES: Record<IconTileTint, string> = {
  violet: 'bg-tint-violet text-tint-violet-fg',
  blue: 'bg-tint-blue text-tint-blue-fg',
  mint: 'bg-tint-mint text-tint-mint-fg',
  amber: 'bg-tint-amber text-tint-amber-fg',
  rose: 'bg-tint-rose text-tint-rose-fg',
};

const BOX_CLASSES: Record<IconTileSize, string> = {
  sm: 'h-7 w-7 rounded-md',
  md: 'h-9 w-9 rounded-lg',
  lg: 'h-11 w-11 rounded-xl',
};

const GLYPH_SIZE: Record<IconTileSize, number> = {
  sm: 12,
  md: 16,
  lg: 20,
};

export type IconTileProps = {

  icon: LucideIcon;
  tint?: IconTileTint;
  size?: IconTileSize;
  className?: string;
};

export function IconTile({ icon: Icon, tint = 'blue', size = 'md', className }: IconTileProps) {
  return (
    <div
      className={cn(
        'flex shrink-0 items-center justify-center',
        BOX_CLASSES[size],
        TINT_CLASSES[tint],
        className,
      )}
    >
      <Icon size={GLYPH_SIZE[size]} strokeWidth={2} color="currentColor" />
    </div>
  );
}
