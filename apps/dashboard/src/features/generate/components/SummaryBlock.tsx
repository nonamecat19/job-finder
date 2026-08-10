import { useState } from 'react';
import type { GenerationSectionDto } from '@job-finder/shared';
import { Button, Textarea } from '../../../components/ui';
import OriginBadge from './OriginBadge';

export interface SummaryBlockProps {
  section: GenerationSectionDto; // kind === 'summary'
  onToggle: (itemId: string, selected: boolean) => void;
  onEditText: (itemId: string, text: string) => void;
}

// T030: the single summary item as accept / edit / drop — not a checklist
// like the achievements, because there is exactly one summary and its state
// is binary (in the export or not).
export default function SummaryBlock({ section, onToggle, onEditText }: SummaryBlockProps) {
  const item = section.items[0];
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(item?.text ?? '');

  if (!item) {
    return (
      <div className="rounded-md border border-border bg-surface-secondary/60 p-3" data-testid="summary-block">
        <div className="mb-2 text-sm font-semibold">Summary</div>
        <p className="text-xs text-muted">
          {section.state === 'failed' ? 'Summary generation failed for this run.' : 'No summary yet.'}
        </p>
      </div>
    );
  }

  const save = () => {
    onEditText(item.id, draft);
    setEditing(false);
  };

  return (
    <div className="rounded-md border border-border bg-surface-secondary/60 p-3" data-testid="summary-block">
      <div className="mb-2 flex items-center justify-between text-sm font-semibold">
        <span>Summary</span>
        <OriginBadge origin={item.origin} kind={item.kind} />
      </div>

      {editing ? (
        <div className="space-y-2">
          <Textarea value={draft} onChange={(e) => setDraft(e.target.value)} className="h-24" />
          <div className="flex gap-2">
            <Button onClick={save}>save</Button>
            <Button
              variant="ghost"
              onClick={() => {
                setDraft(item.text);
                setEditing(false);
              }}
            >
              cancel
            </Button>
          </div>
        </div>
      ) : (
        <>
          <p className={item.selected ? 'text-sm' : 'text-sm text-faint line-through'}>{item.text}</p>
          <div className="mt-2 flex gap-2">
            {item.selected ? (
              <Button variant="secondary" onClick={() => onToggle(item.id, false)}>
                drop
              </Button>
            ) : (
              <Button onClick={() => onToggle(item.id, true)}>accept</Button>
            )}
            <Button
              variant="ghost"
              onClick={() => {
                setDraft(item.text);
                setEditing(true);
              }}
            >
              edit
            </Button>
          </div>
        </>
      )}
    </div>
  );
}
