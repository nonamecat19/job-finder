import { useState, type ReactNode } from 'react';
import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, useSortable, arrayMove } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { GripVertical, ChevronUp, ChevronDown, Trash2, Plus } from 'lucide-react';
import type { Entry, Section } from '@job-finder/shared';
import { Button, Field, Select } from '../../../components/ui';
import { ConfirmDialog } from './ConfirmDialog';
import { getEntryForm, emptyEntryFor, ENTRY_TYPE_LABELS } from './entries/entryFormRegistry';
import { UnrecognizedEntryFallbackForm } from './entries/UnrecognizedEntryFallbackForm';

interface SectionEditorProps {
  section: Section;
  onChange: (section: Section) => void;
}

export function SectionEditor({ section, onChange }: SectionEditorProps) {
  const [pendingDelete, setPendingDelete] = useState<number | null>(null);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

  const setEntries = (entries: Entry[]) => onChange({ ...section, entries });

  const updateEntry = (i: number, entry: Entry) => {
    const next = [...section.entries];
    next[i] = entry;
    setEntries(next);
  };

  const moveEntry = (i: number, dir: -1 | 1) => {
    const j = i + dir;
    if (j < 0 || j >= section.entries.length) return;
    setEntries(arrayMove(section.entries, i, j));
  };

  const addEntry = () => setEntries([...section.entries, emptyEntryFor(section.entryType)]);

  const confirmDelete = () => {
    if (pendingDelete === null) return;
    setEntries(section.entries.filter((_, idx) => idx !== pendingDelete));
    setPendingDelete(null);
  };

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = Number(active.id);
    const newIndex = Number(over.id);
    setEntries(arrayMove(section.entries, oldIndex, newIndex));
  };

  const FormComponent = getEntryForm(section.entryType);

  return (
    <div className="space-y-3">
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={section.entries.map((_, i) => String(i))} strategy={verticalListSortingStrategy}>
          {section.entries.map((entry, i) => (
            <SortableEntryRow key={i} id={String(i)}>
              {(dragHandleProps) => (
                <div className="flex items-start gap-2 rounded-lg border border-border bg-surface-secondary p-3">
                  <div className="mt-1 flex flex-col items-center gap-1 text-faint">
                    <button
                      type="button"
                      onClick={() => moveEntry(i, -1)}
                      disabled={i === 0}
                      aria-label="move entry up"
                      className="disabled:opacity-30"
                    >
                      <ChevronUp className="h-4 w-4" />
                    </button>
                    <span {...dragHandleProps} className="cursor-grab touch-none">
                      <GripVertical className="h-4 w-4" />
                    </span>
                    <button
                      type="button"
                      onClick={() => moveEntry(i, 1)}
                      disabled={i === section.entries.length - 1}
                      aria-label="move entry down"
                      className="disabled:opacity-30"
                    >
                      <ChevronDown className="h-4 w-4" />
                    </button>
                  </div>
                  <div className="flex-1">
                    {FormComponent ? (
                      <FormComponent entry={entry} onChange={(e) => updateEntry(i, e)} />
                    ) : (
                      <UnrecognizedEntryFallbackForm entry={entry} onChange={(e) => updateEntry(i, e)} />
                    )}
                    {FormComponent && entry.unrecognized && Object.keys(entry.unrecognized).length > 0 ? (
                      <div className="mt-2 border-t border-border pt-2">
                        <UnrecognizedEntryFallbackForm entry={entry} onChange={(e) => updateEntry(i, e)} />
                      </div>
                    ) : null}
                  </div>
                  <Button variant="ghost" onClick={() => setPendingDelete(i)} aria-label="delete entry">
                    <Trash2 className="h-4 w-4 text-danger" />
                  </Button>
                </div>
              )}
            </SortableEntryRow>
          ))}
        </SortableContext>
      </DndContext>

      <Button variant="secondary" onClick={addEntry}>
        <Plus className="h-4 w-4" /> add {ENTRY_TYPE_LABELS[section.entryType]?.toLowerCase() ?? 'entry'}
      </Button>

      <ConfirmDialog
        open={pendingDelete !== null}
        title="Delete entry?"
        description="This entry will be permanently removed from the section."
        confirmLabel="Delete"
        onConfirm={confirmDelete}
        onCancel={() => setPendingDelete(null)}
      />
    </div>
  );
}

function SortableEntryRow({
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

export function EntryTypePicker({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <Field label="Entry type">
      <Select value={value} onChange={(e) => onChange(e.target.value)}>
        {Object.entries(ENTRY_TYPE_LABELS).map(([type, label]) => (
          <option key={type} value={type}>
            {label}
          </option>
        ))}
      </Select>
    </Field>
  );
}
