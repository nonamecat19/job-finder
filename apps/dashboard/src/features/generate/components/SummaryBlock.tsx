import { useEffect, useRef, useState } from 'react';
import type { GenerationSectionDto } from '@job-finder/shared';
import { Button, Switch, Textarea } from '../../../components/ui';
import { cn } from '../../../lib/utils';
import { usePreviewHighlight } from '../preview/highlight';
import OriginBadge from './OriginBadge';

export interface SummaryBlockProps {
  section: GenerationSectionDto; // kind === 'summary'
  onToggle: (itemId: string, selected: boolean) => void;
  onEditText: (itemId: string, text: string) => void;
  /** Re-runs generation for just this section, producing a fresh summary. */
  onRegenerate: () => void;
  onToggleEnabled: (sectionId: string, enabled: boolean) => void;
}

// T030: the single summary item as accept / edit / regenerate — not a
// checklist like the achievements, because there is exactly one summary and
// its state is binary (in the export or not).
export default function SummaryBlock({ section, onToggle, onEditText, onRegenerate, onToggleEnabled }: SummaryBlockProps) {
  const item = section.items[0];
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(item?.text ?? '');

  // Cross-pane highlighting, the same contract ItemRow implements for every
  // other row: this block is a row too, it just doesn't look like one.
  const { hover, setHover } = usePreviewHighlight();
  const highlighted = !!item && hover?.itemId === item.id;
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!item || hover?.itemId !== item.id || hover.source !== 'pdf') return;
    ref.current?.scrollIntoView({ block: 'center', behavior: 'smooth' });
  }, [hover, item]);

  if (!item) {
    return (
      <div className="rounded-xl bg-surface-secondary p-3" data-testid="summary-block">
        <div className="mb-2 flex items-center justify-between">
          <span className="font-mono text-xs font-medium tracking-[0.06em] text-muted uppercase">Summary</span>
          <Switch
            checked={section.enabled}
            onChange={(enabled) => onToggleEnabled(section.id, enabled)}
            label="include summary in export"
          />
        </div>
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
    <div
      ref={ref}
      data-testid="summary-block"
      data-highlighted={highlighted}
      onMouseEnter={() => setHover({ itemId: item.id, source: 'list' })}
      onMouseLeave={() => setHover(null)}
      className={cn(
        'rounded-xl bg-surface-secondary p-3',
        'transition-[color,background-color,box-shadow] duration-[160ms] ease-[cubic-bezier(0.2,0,0.2,1)]',
        highlighted ? 'bg-accent-soft ring-1 ring-accent/50' : undefined,
      )}
    >
      <div className="mb-2 flex items-center justify-between">
        <span className="font-mono text-xs font-medium tracking-[0.06em] text-muted uppercase">Summary</span>
        <div className="flex items-center gap-2">
          <OriginBadge origin={item.origin} kind={item.kind} />
          <Switch
            checked={section.enabled}
            onChange={(enabled) => onToggleEnabled(section.id, enabled)}
            label="include summary in export"
          />
        </div>
      </div>

      <div className={cn(!section.enabled && 'pointer-events-none opacity-50')}>
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
            <p className={item.selected ? 'text-sm text-foreground' : 'text-sm text-faint'}>
              {item.text}
            </p>
            <div className="mt-2 flex gap-2">
              {item.selected ? (
                <Button variant="secondary" onClick={onRegenerate}>
                  regenerate
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
    </div>
  );
}
