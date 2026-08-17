import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { GripVertical, Sparkles } from 'lucide-react';
import { useState } from 'react';
import type { GenerationItemDto } from '@job-finder/shared';
import { Button, Checkbox, Spinner } from '../../../components/ui';
import { cn } from '../../../lib/utils';
import OriginBadge from './OriginBadge';

export interface ItemRowProps {
  item: GenerationItemDto;
  onToggle: (selected: boolean) => void;
  /** Present only for an origin="ai" item once included (FR-015). */
  onEditText?: (text: string) => void;
  /**
   * Per-skill toggle inside a skill group: the entries the user wants left
   * out, sent as the whole set. Present only where item.skillEntries is.
   */
  onDropEntries?: (droppedEntries: string[]) => void;
  /**
   * Rewrite: 2-3 grounded alternate phrasings of this bullet. Present only
   * for a selected, origin="ai" achievement item — the server rejects the
   * request for anything else (only an AI-origin item's text can ever be
   * saved, per FR-009).
   */
  onRewrite?: () => Promise<string[]>;
}

// T028: checkbox, effective text, origin badge, a dnd-kit drag handle, and an
// `unavailable` presentation (FR-022 — the source item's master bullet no
// longer resolves, but the row still renders rather than silently
// disappearing).
export default function ItemRow({ item, onToggle, onEditText, onDropEntries, onRewrite }: ItemRowProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: item.id });
  const style = { transform: CSS.Transform.toString(transform), transition };
  const editable = item.origin === 'ai' && item.selected && !!onEditText;
  const rewritable = item.origin === 'ai' && item.kind === 'achievement' && item.selected && !!onRewrite;
  const [rewriting, setRewriting] = useState(false);
  const [variants, setVariants] = useState<string[] | null>(null);
  const entries = item.skillEntries ?? [];
  // Per-skill chips replace the flat "Label: a, b, c" line for a skill group.
  // Only while the group itself is in: a switched-off group is not a place to
  // fine-tune which skills it would have shown.
  const perSkill = entries.length > 0 && !!onDropEntries && item.selected && !item.unavailable;
  const label = item.text.split(':')[0];

  const toggleEntry = (text: string, selected: boolean) => {
    const dropped = entries.filter((e) => (e.text === text ? !selected : !e.selected)).map((e) => e.text);
    onDropEntries?.(dropped);
  };

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
        'flex items-start gap-2 rounded-lg px-2 py-1.5 text-sm',
        'transition-[color,background-color] duration-[160ms] ease-[cubic-bezier(0.2,0,0.2,1)]',
        item.selected && !item.unavailable ? 'bg-surface-tertiary' : undefined,
        isDragging ? 'opacity-60' : undefined,
        item.unavailable ? 'bg-danger-soft/60' : undefined,
      )}
    >
      <button
        type="button"
        {...attributes}
        {...listeners}
        aria-label="drag to reorder"
        disabled={item.unavailable}
        className="mt-0.5 shrink-0 cursor-grab touch-none text-faint hover:text-muted disabled:cursor-not-allowed disabled:opacity-30"
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
        {perSkill ? (
          <div data-testid="skill-entries">
            <p className="break-words font-medium text-foreground">{label}</p>
            <div className="mt-1 flex flex-wrap gap-1">
              {entries.map((entry) => (
                <button
                  key={entry.text}
                  type="button"
                  aria-pressed={entry.selected}
                  aria-label={entry.text}
                  data-testid="skill-entry"
                  data-selected={entry.selected}
                  onClick={() => toggleEntry(entry.text, !entry.selected)}
                  className={cn(
                    'rounded-full px-2 py-0.5 text-xs ring-1 ring-inset transition-colors',
                    entry.selected
                      ? 'bg-accent-soft text-accent ring-accent/25'
                      : 'bg-surface-tertiary text-faint ring-border line-through',
                  )}
                >
                  {entry.text}
                </button>
              ))}
            </div>
          </div>
        ) : editable ? (
          <textarea
            value={item.text}
            onChange={(e) => onEditText?.(e.target.value)}
            className="w-full resize-y rounded-lg border border-border bg-surface px-2 py-1 text-sm outline-none focus:border-border-strong focus:ring-[3px] focus:ring-accent-soft"
            rows={2}
          />
        ) : (
          <p className={cn(item.selected ? 'text-foreground' : 'text-faint line-through', 'break-words')}>
            {item.text}
          </p>
        )}

        {rewriting ? (
          <div className="mt-1.5 flex items-center gap-2 text-xs text-muted">
            <Spinner /> rewriting this bullet…
          </div>
        ) : null}

        {variants && !rewriting ? (
          <div className="mt-1.5 flex flex-col gap-1" data-testid="rewrite-variants">
            {variants.length === 0 ? (
              <p className="text-xs text-muted">No alternative phrasing available for this bullet.</p>
            ) : (
              variants.map((v) => (
                <div
                  key={v}
                  className="flex items-start gap-2 rounded-lg border border-dashed border-border-strong bg-surface-secondary px-2 py-1.5"
                >
                  <p className="min-w-0 flex-1 text-xs text-muted">{v}</p>
                  <Button
                    variant="secondary"
                    onClick={() => {
                      onEditText?.(v);
                      setVariants(null);
                    }}
                  >
                    use
                  </Button>
                </div>
              ))
            )}
          </div>
        ) : null}
      </div>

      <div className="flex shrink-0 items-center gap-1.5">
        {item.edited ? <span className="font-mono text-[11px] text-faint">edited</span> : null}
        {rewritable ? (
          <button
            type="button"
            title="rewrite this bullet"
            aria-label="rewrite this bullet"
            disabled={rewriting}
            onClick={async () => {
              setRewriting(true);
              setVariants(null);
              try {
                const result = await onRewrite!();
                setVariants(result);
              } finally {
                setRewriting(false);
              }
            }}
            className="rounded-lg p-1 text-faint hover:bg-surface-tertiary hover:text-accent disabled:opacity-50"
          >
            <Sparkles className="h-3.5 w-3.5" />
          </button>
        ) : null}
        <OriginBadge origin={item.origin} kind={item.kind} />
      </div>
    </li>
  );
}
