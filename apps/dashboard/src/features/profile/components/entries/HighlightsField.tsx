import { Plus, Trash2 } from 'lucide-react';
import { Button, Field, Input } from '../../../../components/ui';

interface HighlightsFieldProps {
  highlights?: string[];
  onChange: (highlights: string[] | undefined) => void;
  className?: string;
}

export function HighlightsField({ highlights, onChange, className }: HighlightsFieldProps) {
  const items = highlights ?? [];

  const emit = (next: string[]) => onChange(next.length > 0 ? next : undefined);

  const updateAt = (i: number, value: string) => {
    const next = [...items];
    next[i] = value;
    emit(next);
  };
  const removeAt = (i: number) => emit(items.filter((_, idx) => idx !== i));
  const add = () => emit([...items, '']);

  return (
    <Field label="Highlights" className={className}>
      <div className="space-y-2">
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
    </Field>
  );
}
