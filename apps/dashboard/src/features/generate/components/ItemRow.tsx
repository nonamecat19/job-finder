import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { GripVertical } from 'lucide-react';
import type { GenerationItemDto } from '@job-finder/shared';
import { Checkbox } from '../../../components/ui';
import { cn } from '../../../lib/utils';
import OriginBadge from './OriginBadge';

export interface ItemRowProps {
  item: GenerationItemDto;
  onToggle: (selected: boolean) => void;
  /** Present only for an origin="ai" item once included (FR-015). */
  onEditText?: (text: string) => void;
}

// T028: checkbox, effective text, origin badge, a dnd-kit drag handle, and an
// `unavailable` presentation (FR-022 — the source item's master bullet no
// longer resolves, but the row still renders rather than silently
// disappearing).
export default function ItemRow({ item, onToggle, onEditText }: ItemRowProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: item.id });
  const style = { transform: CSS.Transform.toString(transform), transition };
  const editable = item.origin === 'ai' && item.selected && !!onEditText;

  return (
    <li
      ref={setNodeRef}
      style={style}
      data-testid="item-row"
      data-item-id={item.id}
      data-origin={item.origin}
      data-selected={item.selected}
      data-unavailable={item.unavailable}
      className={cn(
        'flex items-start gap-2 rounded-md border border-border/60 bg-surface px-2 py-1.5 text-sm',
        isDragging ? 'opacity-60' : undefined,
        item.unavailable ? 'border-dashed border-danger/40 bg-danger-soft/30' : undefined,
      )}
    >
      <button
        type="button"
        {...attributes}
        {...listeners}
        aria-label="drag to reorder"
        disabled={item.unavailable}
        className="mt-0.5 shrink-0 cursor-grab touch-none text-faint disabled:cursor-not-allowed disabled:opacity-30"
      >
        <GripVertical className="h-4 w-4" />
      </button>

      <Checkbox
        aria-label={item.selected ? 'included' : 'not included'}
        checked={item.selected}
        disabled={item.unavailable}
        onChange={(e) => onToggle(e.target.checked)}
        className="mt-1 shrink-0"
      />

      <div className="min-w-0 flex-1">
        {item.unavailable ? (
          <p className="text-xs font-medium text-danger">
            No longer in your profile — this bullet was removed or changed since this run started.
          </p>
        ) : null}
        {editable ? (
          <textarea
            value={item.text}
            onChange={(e) => onEditText?.(e.target.value)}
            className="w-full resize-y rounded border border-border bg-surface-secondary px-2 py-1 text-sm outline-none focus:border-accent"
            rows={2}
          />
        ) : (
          <p className={cn(item.selected ? undefined : 'text-faint line-through', 'break-words')}>{item.text}</p>
        )}
      </div>

      <div className="flex shrink-0 items-center gap-1.5">
        {item.edited ? <span className="text-xs text-faint">edited</span> : null}
        <OriginBadge origin={item.origin} kind={item.kind} />
      </div>
    </li>
  );
}
