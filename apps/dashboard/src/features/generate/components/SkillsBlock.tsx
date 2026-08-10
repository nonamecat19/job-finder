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
}

// T031/T063: skill-group items with the same toggle affordance as
// achievements, grouped exactly the way WorkEntryBlock groups its items so the
// two surfaces read identically — the profile's own groups first (selected,
// then the ones skillsMaxGroups left out, which are shown unselected rather
// than removed, FR-011), then the AI-suggested skills in their own visually
// distinct group, off by default (FR-013).
export default function SkillsBlock({ section, onToggle, onReorder, onEditText }: SkillsBlockProps) {
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
    <div className="rounded-md border border-border bg-surface-secondary/60 p-3" data-testid="skills-block">
      <div className="mb-2 flex items-center justify-between text-sm font-semibold">
        <span>Skills</span>
        {section.state !== 'ready' ? <span className="text-xs font-normal text-muted">{section.state}</span> : null}
      </div>

      {profileItems.length === 0 ? (
        <p className="text-xs text-muted">No skill groups in your profile.</p>
      ) : (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={orderedItems.map((it) => it.id)} strategy={verticalListSortingStrategy}>
            <ul className="space-y-1.5">
              {selectedItems.map((item) => (
                <ItemRow key={item.id} item={item} onToggle={(selected) => onToggle(item.id, selected)} />
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
                <ItemRow key={item.id} item={item} onToggle={(selected) => onToggle(item.id, selected)} />
              ))}
            </ul>
          </SortableContext>
        </DndContext>
      )}

      <div className="mt-3 border-t border-dashed border-border/70 pt-2" data-testid="suggestion-group">
        <div className="mb-1.5 text-[11px] font-medium uppercase tracking-wide text-faint">AI suggestions</div>
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
