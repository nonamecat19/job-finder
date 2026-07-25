import { useState, type ReactNode } from 'react';
import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, useSortable, arrayMove } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { GripVertical, ChevronUp, ChevronDown, Trash2, Plus } from 'lucide-react';
import type { EntryType, Section } from '@job-finder/shared';
import { Button, Input, Surface } from '../../../components/ui';
import { ConfirmDialog } from './ConfirmDialog';
import { SectionEditor, EntryTypePicker } from './SectionEditor';
import { ENTRY_TYPE_LABELS } from './entries/entryFormRegistry';

interface SectionListProps {
  sections: Section[];
  onChange: (sections: Section[]) => void;
}

// Ordered list of resume sections: add (custom names allowed, FR-005),
// rename, delete (confirmed, FR-011), and reorder via drag or up/down
// buttons (FR-006). Each section's entries are delegated to SectionEditor.
export function SectionList({ sections, onChange }: SectionListProps) {
  const [pendingDelete, setPendingDelete] = useState<number | null>(null);
  const [newSectionName, setNewSectionName] = useState('');
  const [newSectionType, setNewSectionType] = useState<EntryType>('bullet');
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

  const updateSection = (i: number, section: Section) => {
    const next = [...sections];
    next[i] = section;
    onChange(next);
  };

  const moveSection = (i: number, dir: -1 | 1) => {
    const j = i + dir;
    if (j < 0 || j >= sections.length) return;
    onChange(arrayMove(sections, i, j));
  };

  const confirmDelete = () => {
    if (pendingDelete === null) return;
    onChange(sections.filter((_, idx) => idx !== pendingDelete));
    setPendingDelete(null);
  };

  const addSection = () => {
    const name = newSectionName.trim();
    if (!name) return;
    onChange([...sections, { name, entryType: newSectionType, entries: [] }]);
    setNewSectionName('');
  };

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    onChange(arrayMove(sections, Number(active.id), Number(over.id)));
  };

  return (
    <div className="space-y-4">
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={sections.map((_, i) => String(i))} strategy={verticalListSortingStrategy}>
          {sections.map((section, i) => (
            <SortableSectionCard key={i} id={String(i)}>
              {(dragHandleProps) => (
                <Surface>
                  <div className="flex items-center gap-2">
                    <span {...dragHandleProps} className="cursor-grab touch-none text-faint">
                      <GripVertical className="h-4 w-4" />
                    </span>
                    <Input
                      value={section.name}
                      onChange={(e) => updateSection(i, { ...section, name: e.target.value })}
                      className="max-w-xs font-semibold"
                      aria-label="section name"
                    />
                    <span className="text-xs text-faint">{ENTRY_TYPE_LABELS[section.entryType] ?? section.entryType}</span>
                    <div className="ml-auto flex items-center gap-1">
                      <button
                        type="button"
                        onClick={() => moveSection(i, -1)}
                        disabled={i === 0}
                        aria-label="move section up"
                        className="text-faint disabled:opacity-30"
                      >
                        <ChevronUp className="h-4 w-4" />
                      </button>
                      <button
                        type="button"
                        onClick={() => moveSection(i, 1)}
                        disabled={i === sections.length - 1}
                        aria-label="move section down"
                        className="text-faint disabled:opacity-30"
                      >
                        <ChevronDown className="h-4 w-4" />
                      </button>
                      <Button variant="ghost" onClick={() => setPendingDelete(i)} aria-label="delete section">
                        <Trash2 className="h-4 w-4 text-danger" />
                      </Button>
                    </div>
                  </div>
                  <div className="mt-3">
                    <SectionEditor section={section} onChange={(s) => updateSection(i, s)} />
                  </div>
                </Surface>
              )}
            </SortableSectionCard>
          ))}
        </SortableContext>
      </DndContext>

      <Surface className="border-dashed">
        <div className="flex flex-wrap items-end gap-2">
          <div className="flex-1">
            <Input
              value={newSectionName}
              onChange={(e) => setNewSectionName(e.target.value)}
              placeholder="New section name (e.g. Volunteering)"
            />
          </div>
          <EntryTypePicker value={newSectionType} onChange={(v) => setNewSectionType(v as EntryType)} />
          <Button onClick={addSection} disabled={!newSectionName.trim()}>
            <Plus className="h-4 w-4" /> add section
          </Button>
        </div>
      </Surface>

      <ConfirmDialog
        open={pendingDelete !== null}
        title="Delete section?"
        description="This section and all of its entries will be permanently removed."
        confirmLabel="Delete"
        onConfirm={confirmDelete}
        onCancel={() => setPendingDelete(null)}
      />
    </div>
  );
}

function SortableSectionCard({
  id,
  children,
}: {
  id: string;
  children: (dragHandleProps: Record<string, unknown>) => ReactNode;
}) {
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({ id });
  const style = { transform: CSS.Transform.toString(transform), transition };
  return (
    <div ref={setNodeRef} style={style}>
      {children({ ...attributes, ...listeners })}
    </div>
  );
}
