import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable';
import { useEffect, useRef } from 'react';
import type { GenerationSectionDto } from '@job-finder/shared';
import { Switch } from '../../../components/ui';
import ItemRow from './ItemRow';
import { usePreviewHighlight } from '../preview/highlight';
import { cn, scrollIntoViewWithOffset } from '../../../lib/utils';

const SCROLL_OFFSET = 24;

export interface WorkEntryBlockProps {
  section: GenerationSectionDto;
  onToggle: (itemId: string, selected: boolean) => void;
  onReorder: (sectionId: string, orderedItemIds: string[]) => void;

  onEditText?: (itemId: string, text: string) => void;

  onRewrite?: (itemId: string) => Promise<string[]>;
  onToggleEnabled: (sectionId: string, enabled: boolean) => void;
}

export default function WorkEntryBlock({
  section,
  onToggle,
  onReorder,
  onEditText,
  onRewrite,
  onToggleEnabled,
}: WorkEntryBlockProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));
  const { hover, setHover } = usePreviewHighlight();

  const containerRef = useRef<HTMLDivElement | null>(null);
  const headerRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    if (hover?.blockKey !== section.id || hover.source !== 'pdf') return;
    if (hover.itemId) {
      const row = containerRef.current?.querySelector<HTMLElement>(`[data-item-id="${hover.itemId}"]`);
      if (row && isVisibleInScrollParent(row)) return;
    }
    if (headerRef.current) scrollIntoViewWithOffset(headerRef.current, SCROLL_OFFSET);
  }, [hover, section.id]);

  const profileItems = section.items.filter((it) => it.origin === 'profile');
  const suggestionItems = section.items.filter((it) => it.origin === 'ai');
  const selectedItems = profileItems.filter((it) => it.selected);
  const unselectedItems = profileItems.filter((it) => !it.selected);
  const orderedItems = [...selectedItems, ...unselectedItems];

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = orderedItems.findIndex((it) => it.id === active.id);
    const newIndex = orderedItems.findIndex((it) => it.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    const reordered = arrayMove(orderedItems, oldIndex, newIndex);
    onReorder(
      section.id,
      reordered.map((it) => it.id),
    );
  };

  return (
    <div ref={containerRef} className="rounded-xl bg-surface-secondary p-3" data-testid="work-entry-block">
      <div
        ref={headerRef}
        className="mb-2 flex items-center justify-between"
        onMouseEnter={() => setHover({ blockKey: section.id, source: 'list' })}
        onMouseLeave={() => setHover(null)}
      >
        <span className="text-sm font-semibold text-foreground">
          {section.entryLabel ?? section.entryKey ?? 'Experience'}
        </span>
        <div className="flex items-center gap-2">
          {section.state !== 'ready' ? (
            <span className="font-mono text-[11px] text-muted uppercase tracking-[0.06em]">{section.state}</span>
          ) : null}
          <Switch
            checked={section.enabled}
            onChange={(enabled) => onToggleEnabled(section.id, enabled)}
            label="include this role in export"
          />
        </div>
      </div>

      <div className={cn(!section.enabled && 'pointer-events-none opacity-50')}>
      {profileItems.length === 0 ? (
        <p className="text-xs text-muted">No bullets in your profile for this role.</p>
      ) : (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={orderedItems.map((it) => it.id)} strategy={verticalListSortingStrategy}>
            <ul className="space-y-1.5">
              {selectedItems.map((item) => (
                <ItemRow
                  key={item.id}
                  item={item}
                  onToggle={(selected) => onToggle(item.id, selected)}
                  onEditText={onEditText ? (text) => onEditText(item.id, text) : undefined}
                  scrollOnHover={false}
                />
              ))}
              {selectedItems.length > 0 && unselectedItems.length > 0 ? (
                <li
                  data-testid="unselected-divider"
                  className="px-2 pt-2 pb-1 font-mono text-[11px] font-medium text-faint uppercase tracking-[0.06em]"
                >
                  Ranked, not included — promote to add
                </li>
              ) : null}
              {unselectedItems.map((item) => (
                <ItemRow
                  key={item.id}
                  item={item}
                  onToggle={(selected) => onToggle(item.id, selected)}
                  onEditText={onEditText ? (text) => onEditText(item.id, text) : undefined}
                  scrollOnHover={false}
                />
              ))}
            </ul>
          </SortableContext>
        </DndContext>
      )}

      {}
      <div className="mt-3 border-t border-border pt-2" data-testid="suggestion-group">
        <div className="mb-1.5 px-2 font-mono text-[11px] font-medium text-faint uppercase tracking-[0.06em]">
          AI suggestions
        </div>
        {suggestionItems.length === 0 ? (
          <p className="text-xs text-muted">No AI suggestions for this role in this run.</p>
        ) : (
          <ul className="space-y-1.5">
            {suggestionItems.map((item) => (
              <ItemRow
                key={item.id}
                item={item}
                onToggle={(selected) => onToggle(item.id, selected)}
                onEditText={onEditText ? (text) => onEditText(item.id, text) : undefined}
                onRewrite={onRewrite ? () => onRewrite(item.id) : undefined}
                scrollOnHover={false}
              />
            ))}
          </ul>
        )}
      </div>
      </div>
    </div>
  );
}

function isVisibleInScrollParent(el: HTMLElement): boolean {
  let parent = el.parentElement;
  while (parent) {
    const style = getComputedStyle(parent);
    if (/(auto|scroll)/.test(style.overflowY) && parent.scrollHeight > parent.clientHeight) break;
    parent = parent.parentElement;
  }
  if (!parent) return true;
  const elRect = el.getBoundingClientRect();
  const parentRect = parent.getBoundingClientRect();
  return elRect.top >= parentRect.top && elRect.bottom <= parentRect.bottom;
}
