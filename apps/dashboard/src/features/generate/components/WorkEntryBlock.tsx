import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable';
import type { GenerationSectionDto } from '@job-finder/shared';
import ItemRow from './ItemRow';

export interface WorkEntryBlockProps {
  section: GenerationSectionDto; // kind === 'experience'
  onToggle: (itemId: string, selected: boolean) => void;
  onReorder: (sectionId: string, orderedItemIds: string[]) => void;
  /** Present so an included (selected) origin="ai" item can be edited in place (T056, FR-015). */
  onEditText?: (itemId: string, text: string) => void;
}

// T029/T048: one work entry — its label, the profile's ranked achievements in
// `position` order, and an explicit empty state when the master profile has
// zero bullets for this role. Never a fabricated bullet standing in for one
// — an empty section renders as empty, not as invented content.
//
// T048 adds the ranked/unranked visual split (research.md R2): the ranking
// stage selects the top min(N, A) of its K candidates, leaves the rest of the
// K ranking unselected, and appends any bullet beyond K in master order,
// unselected — all already delivered in `position` order by the seeding
// (SeedRankedItems/SeedFromMaster). The client's only observable signal for
// "included vs not" is `selected`, so that is the divider: selected items
// first (the resume as it stands), then everything else the user can
// promote — one continuous, draggable list, not two separate ones.
export default function WorkEntryBlock({ section, onToggle, onReorder, onEditText }: WorkEntryBlockProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

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
    <div className="rounded-md border border-border bg-surface-secondary/60 p-3" data-testid="work-entry-block">
      <div className="mb-2 flex items-center justify-between text-sm font-semibold">
        <span>{section.entryLabel ?? section.entryKey ?? 'Experience'}</span>
        {section.state !== 'ready' ? <span className="text-xs font-normal text-muted">{section.state}</span> : null}
      </div>

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
                />
              ))}
              {selectedItems.length > 0 && unselectedItems.length > 0 ? (
                <li
                  data-testid="unselected-divider"
                  className="pt-1 text-[11px] font-medium uppercase tracking-wide text-faint"
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
      <div className="mt-3 border-t border-dashed border-border/70 pt-2" data-testid="suggestion-group">
        <div className="mb-1.5 text-[11px] font-medium uppercase tracking-wide text-faint">AI suggestions</div>
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
              />
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
