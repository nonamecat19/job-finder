import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable';
import type { GenerationSectionDto } from '@job-finder/shared';
import ItemRow from './ItemRow';

export interface ProjectsBlockProps {
  section: GenerationSectionDto; // kind === 'projects'
  onToggle: (itemId: string, selected: boolean) => void;
  onReorder: (sectionId: string, orderedItemIds: string[]) => void;
}

// Projects read like skill groups: one row per project, ordered by vacancy
// relevance, with the ones past `projectsMax` shown unselected rather than
// removed so the user can promote them. There is no AI-suggestion group —
// nothing in this section is model-written, so there is nothing to suggest or
// edit in place: a project's name, dates and bullets come from the profile
// verbatim, and the density of its bullets is a profile setting.
export default function ProjectsBlock({ section, onToggle, onReorder }: ProjectsBlockProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

  const selectedItems = section.items.filter((it) => it.selected);
  const unselectedItems = section.items.filter((it) => !it.selected);
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
    <div className="rounded-xl bg-surface-secondary p-3" data-testid="projects-block">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-sm font-semibold text-foreground">Projects</span>
        {section.state !== 'ready' ? (
          <span className="font-mono text-[11px] text-muted uppercase tracking-[0.06em]">{section.state}</span>
        ) : null}
      </div>

      {section.items.length === 0 ? (
        <p className="text-xs text-muted">No projects in your profile.</p>
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
                  className="px-2 pt-2 pb-1 font-mono text-[11px] font-medium text-faint uppercase tracking-[0.06em]"
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
    </div>
  );
}
