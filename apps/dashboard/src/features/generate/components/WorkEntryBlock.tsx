import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable';
import { useEffect, useRef } from 'react';
import type { GenerationSectionDto } from '@job-finder/shared';
import { Switch } from '../../../components/ui';
import ItemRow from './ItemRow';
import { usePreviewHighlight } from '../preview/highlight';
import { cn, scrollIntoViewWithOffset } from '../../../lib/utils';

/** Keeps the scrolled-to header clear of the scroll pane's own top edge. */
const SCROLL_OFFSET = 24;

export interface WorkEntryBlockProps {
  section: GenerationSectionDto; // kind === 'experience'
  onToggle: (itemId: string, selected: boolean) => void;
  onReorder: (sectionId: string, orderedItemIds: string[]) => void;
  /** Present so an included (selected) origin="ai" item can be edited in place (T056, FR-015). */
  onEditText?: (itemId: string, text: string) => void;
  /** Present so a selected origin="ai" achievement can offer alternate phrasings. */
  onRewrite?: (itemId: string) => Promise<string[]>;
  onToggleEnabled: (sectionId: string, enabled: boolean) => void;
}

// T029/T048: one work entry — its label, the profile's ranked achievements in
// `position` order, and an explicit empty state when the master profile has
// zero bullets for this role. Never a fabricated bullet standing in for one
// — an empty section renders as empty, not as invented content.
//
// The ranked/unranked visual split (resume-generation.md § 2b): the ranking
// stage selects the top min(N, A) of its K candidates, leaves the rest of the
// K ranking unselected, and appends any bullet beyond K in master order,
// unselected — all already delivered in `position` order by the seeding
// (SeedRankedItems/SeedFromMaster). The client's only observable signal for
// "included vs not" is `selected`, so that is the divider: selected items
// first (the resume as it stands), then everything else the user can
// promote — one continuous, draggable list, not two separate ones.
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

  // Cross-pane highlighting: any hover that started in the PDF over this
  // entry — its heading, the gap between bullets, or one achievement — scrolls
  // the entry's top into view here rather than the single achievement, so the
  // reader always lands on the role being pointed at, not on one line of it.
  // Skipped when the achievement is already on screen, so a hover that stays
  // inside the visible list never jitters it.
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

  // T055: the client groups items by origin. A suggestion is never
  // interleaved with the profile's own ranked achievements — it renders in
  // its own visually distinct group, badged "AI · unverified" by ItemRow's
  // OriginBadge, off (unselected) by default (FR-013).
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

      {/* T055: the AI-suggested group — visually distinct from the profile's
          own achievements above, each item carrying ItemRow's "AI ·
          unverified" badge, unselected until the user acts (FR-013). A run
          that produced none still renders this explicit empty state rather
          than a missing or broken section. */}
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

/** Whether `el` is fully within the visible area of its nearest scrolling ancestor. */
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
