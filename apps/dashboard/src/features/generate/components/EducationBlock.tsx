import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable';
import type { GenerationSectionDto } from '@job-finder/shared';
import { Switch } from '../../../components/ui';
import { cn } from '../../../lib/utils';
import ItemRow from './ItemRow';

export interface EducationBlockProps {
  section: GenerationSectionDto; // kind === 'education'
  onToggle: (itemId: string, selected: boolean) => void;
  onReorder: (sectionId: string, orderedItemIds: string[]) => void;
  onToggleEnabled: (sectionId: string, enabled: boolean) => void;
}

// Education reads exactly like projects/certifications: one row per degree,
// nothing model-written, so nothing to suggest or edit in place — an entry's
// institution, degree and dates come from the profile verbatim.
export default function EducationBlock({ section, onToggle, onReorder, onToggleEnabled }: EducationBlockProps) {
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
    <div className="rounded-xl bg-surface-secondary p-3" data-testid="education-block">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-sm font-semibold text-foreground">Education</span>
        <div className="flex items-center gap-2">
          {section.state !== 'ready' ? (
            <span className="font-mono text-[11px] text-muted uppercase tracking-[0.06em]">{section.state}</span>
          ) : null}
          <Switch
            checked={section.enabled}
            onChange={(enabled) => onToggleEnabled(section.id, enabled)}
            label="include education in export"
          />
        </div>
      </div>

      <div className={cn(!section.enabled && 'pointer-events-none opacity-50')}>
        {section.items.length === 0 ? (
          <p className="text-xs text-muted">No education in your profile.</p>
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
    </div>
  );
}
