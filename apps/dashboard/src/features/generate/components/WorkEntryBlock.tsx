import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable';
import type { GenerationSectionDto } from '@job-finder/shared';
import ItemRow from './ItemRow';

export interface WorkEntryBlockProps {
  section: GenerationSectionDto; // kind === 'experience'
  onToggle: (itemId: string, selected: boolean) => void;
  onReorder: (sectionId: string, orderedItemIds: string[]) => void;
}

// T029: one work entry — its label, the profile's ranked achievements in
// `position` order, and an explicit empty state when the master profile has
// zero bullets for this role. Never a fabricated bullet standing in for one
// — an empty section renders as empty, not as invented content.
export default function WorkEntryBlock({ section, onToggle, onReorder }: WorkEntryBlockProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

  // Phase 3 has no AI-suggested achievements yet (that's US2/US3); scoping to
  // origin="profile" here is what "the client groups items by origin" means
  // in this block once suggestions do exist — a suggestion group renders
  // separately rather than interleaved.
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
    <div className="rounded-md border border-border bg-surface-secondary/60 p-3" data-testid="work-entry-block">
      <div className="mb-2 flex items-center justify-between text-sm font-semibold">
        <span>{section.entryLabel ?? section.entryKey ?? 'Experience'}</span>
        {section.state !== 'ready' ? <span className="text-xs font-normal text-muted">{section.state}</span> : null}
      </div>

      {profileItems.length === 0 ? (
        <p className="text-xs text-muted">No bullets in your profile for this role.</p>
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
