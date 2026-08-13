import { useId, useState } from 'react';
import { Plus, Trash2, ChevronDown, ChevronRight } from 'lucide-react';
import { Button, Input } from '../../../../components/ui';

interface HighlightsFieldProps {
  highlights?: string[];
  onChange: (highlights: string[] | undefined) => void;
  className?: string;
}

export function HighlightsField({ highlights, onChange, className }: HighlightsFieldProps) {
  const items = highlights ?? [];
  const [open, setOpen] = useState(false);
  const panelId = useId();

  const emit = (next: string[]) => onChange(next.length > 0 ? next : undefined);

  const updateAt = (i: number, value: string) => {
    const next = [...items];
    next[i] = value;
    emit(next);
  };
  const removeAt = (i: number) => emit(items.filter((_, idx) => idx !== i));
  const add = () => {
    setOpen(true);
    emit([...items, '']);
  };

  return (
    <div className={className}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-controls={panelId}
        className="inline-flex items-center gap-1.5 rounded px-1 py-0.5 -mx-1 text-xs font-semibold uppercase tracking-wide text-faint transition hover:text-foreground"
      >
        {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        Highlights
        {items.length > 0 ? <span className="normal-case tracking-normal text-faint/70">({items.length})</span> : null}
      </button>

      {open ? (
        <div id={panelId} className="mt-2 space-y-2">
          {items.map((h, i) => (
            <div key={i} className="flex gap-2">
              <Input value={h} onChange={(e) => updateAt(i, e.target.value)} placeholder="Achievement or responsibility" />
              <Button variant="ghost" onClick={() => removeAt(i)} aria-label="remove highlight">
                <Trash2 className="h-4 w-4 text-danger" />
              </Button>
            </div>
          ))}
          <Button variant="secondary" onClick={add}>
            <Plus className="h-4 w-4" /> add highlight
          </Button>
        </div>
      ) : null}
    </div>
  );
}
