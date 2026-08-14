import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable';
import type { GenerationSectionDto } from '@job-finder/shared';
import ItemRow from './ItemRow';

export interface SkillsBlockProps {
  section: GenerationSectionDto; // kind === 'skills'
  onToggle: (itemId: string, selected: boolean) => void;
  onReorder: (sectionId: string, orderedItemIds: string[]) => void;
  /** Present so an included (selected) origin="ai" skill can be edited in place (T056, FR-015). */
  onEditText?: (itemId: string, text: string) => void;
  /** Per-skill inclusion inside a profile skill group: the whole drop set. */
  onDropEntries?: (itemId: string, droppedEntries: string[]) => void;
}

// T031/T063: skill-group items with the same toggle affordance as
// achievements, grouped exactly the way WorkEntryBlock groups its items so the
// two surfaces read identically — the profile's own groups first (selected,
// then the ones skillsMaxGroups left out, which are shown unselected rather
// than removed, FR-011), then the AI-suggested skills in their own visually
// distinct group, off by default (FR-013).
export default function SkillsBlock({ section, onToggle, onReorder, onEditText, onDropEntries }: SkillsBlockProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

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
    <div className="rounded-xl bg-surface-secondary p-3" data-testid="skills-block">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-sm font-semibold text-foreground">Skills</span>
        {section.state !== 'ready' ? (
          <span className="font-mono text-[11px] text-muted uppercase tracking-[0.06em]">{section.state}</span>
        ) : null}
      </div>

      {profileItems.length === 0 ? (
        <p className="text-xs text-muted">No skill groups in your profile.</p>
      ) : (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={orderedItems.map((it) => it.id)} strategy={verticalListSortingStrategy}>
            <ul className="space-y-1.5">
              {selectedItems.map((item) => (
                <ItemRow
                  key={item.id}
                  item={item}
                  onToggle={(selected) => onToggle(item.id, selected)}
                  onDropEntries={onDropEntries ? (dropped) => onDropEntries(item.id, dropped) : undefined}
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
                  onDropEntries={onDropEntries ? (dropped) => onDropEntries(item.id, dropped) : undefined}
                />
              ))}
            </ul>
          </SortableContext>
        </DndContext>
      )}

      <div className="mt-3 border-t border-border pt-2" data-testid="suggestion-group">
        <div className="mb-1.5 px-2 font-mono text-[11px] font-medium text-faint uppercase tracking-[0.06em]">
          AI suggestions
        </div>
        {suggestionItems.length === 0 ? (
          <p className="text-xs text-muted">No AI-suggested skills in this run.</p>
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
