import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable';
import type { GenerationSectionDto } from '@job-finder/shared';
import ItemRow from './ItemRow';

export interface SkillsBlockProps {
  section: GenerationSectionDto; // kind === 'skills'
  onToggle: (itemId: string, selected: boolean) => void;
  onReorder: (sectionId: string, orderedItemIds: string[]) => void;
}

// T031: skill-group items with the same toggle affordance as achievements —
// ItemRow is reused as-is so the two surfaces read identically (US4 extends
// this with an AI-suggested split, matching WorkEntryBlock's grouping).
export default function SkillsBlock({ section, onToggle, onReorder }: SkillsBlockProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));
  const profileItems = section.items.filter((it) => it.origin === 'profile');

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = profileItems.findIndex((it) => it.id === active.id);
    const newIndex = profileItems.findIndex((it) => it.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    const reordered = arrayMove(profileItems, oldIndex, newIndex);
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
          <SortableContext items={profileItems.map((it) => it.id)} strategy={verticalListSortingStrategy}>
            <ul className="space-y-1.5">
              {profileItems.map((item) => (
                <ItemRow key={item.id} item={item} onToggle={(selected) => onToggle(item.id, selected)} />
              ))}
            </ul>
          </SortableContext>
        </DndContext>
      )}
    </div>
  );
}
