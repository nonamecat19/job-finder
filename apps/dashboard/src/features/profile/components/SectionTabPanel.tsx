import { useState } from 'react';
import { ChevronUp, ChevronDown, Trash2 } from 'lucide-react';
import type { Section } from '@job-finder/shared';
import { Button, Input } from '../../../components/ui';
import { ConfirmDialog } from './ConfirmDialog';
import { SectionEditor } from './SectionEditor';
import { ENTRY_TYPE_LABELS } from './entries/entryFormRegistry';

interface SectionTabPanelProps {
  section: Section;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onRename: (name: string) => void;
  onMove: (dir: -1 | 1) => void;
  onDelete: () => void;
  onChange: (section: Section) => void;
}

export function SectionTabPanel({
  section,
  canMoveUp,
  canMoveDown,
  onRename,
  onMove,
  onDelete,
  onChange,
}: SectionTabPanelProps) {
  const [pendingDelete, setPendingDelete] = useState(false);

  return (
    <div>
      <div className="flex items-center gap-2">
        <Input
          value={section.name}
          onChange={(e) => onRename(e.target.value)}
          className="max-w-xs font-semibold"
          aria-label="section name"
        />
        <span className="font-mono text-xs text-faint">{ENTRY_TYPE_LABELS[section.entryType] ?? section.entryType}</span>
        <div className="ml-auto flex items-center gap-1">
          <button
            type="button"
            onClick={() => onMove(-1)}
            disabled={!canMoveUp}
            aria-label="move section up"
            className="text-faint disabled:opacity-30"
          >
            <ChevronUp className="h-4 w-4" />
          </button>
          <button
            type="button"
            onClick={() => onMove(1)}
            disabled={!canMoveDown}
            aria-label="move section down"
            className="text-faint disabled:opacity-30"
          >
            <ChevronDown className="h-4 w-4" />
          </button>
          <Button variant="ghost" onClick={() => setPendingDelete(true)} aria-label="delete section">
            <Trash2 className="h-4 w-4 text-danger" />
          </Button>
        </div>
      </div>

      <div className="mt-3">
        <SectionEditor section={section} onChange={onChange} />
      </div>

      <ConfirmDialog
        open={pendingDelete}
        title="Delete section?"
        description="This section and all of its entries will be permanently removed."
        confirmLabel="Delete"
        onConfirm={() => {
          setPendingDelete(false);
          onDelete();
        }}
        onCancel={() => setPendingDelete(false)}
      />
    </div>
  );
}
