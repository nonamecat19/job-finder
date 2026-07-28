import { Plus, Trash2, ToggleRight, ToggleLeft } from 'lucide-react';
import { useState } from 'react';
import type { EmployerBoardDto } from '@job-finder/shared';
import {
  Button,
  Chip,
  EmptyState,
  ErrorState,
  Field,
  Input,
  LoadingRegion,
  SkeletonBlock,
} from '../../../components/ui';
import { useRegisterBoard, useRemoveBoard, useRoster } from '../hooks';

export default function RosterPanel() {
  const { data, isLoading, error } = useRoster();
  const register = useRegisterBoard();
  const remove = useRemoveBoard();
  const [url, setUrl] = useState('');

  const handleRegister = () => {
    if (!url.trim()) return;
    register.mutate(url.trim(), { onSuccess: () => setUrl('') });
  };

  const registerError = (() => {
    if (!register.error) return null;
    const msg = register.error instanceof Error ? register.error.message : String(register.error);
    if (msg.includes('unsupported_vendor')) {
      const m = msg.match(/\{[^}]+\}/);
      if (m) {
        try {
          const parsed = JSON.parse(m[0]);
          return `Unsupported board vendor. Supported: ${parsed.supportedVendors?.join(', ')}`;
        } catch { /* ignore */ }
      }
      return 'Unsupported board vendor.';
    }
    if (msg.includes('unreadable')) return 'Could not read the board URL.';
    return msg;
  })();

  return (
    <div>
      <div className="mb-3 rounded-lg border border-accent/30 bg-accent-soft p-3">
        <div className="flex items-end gap-2">
          <Field label="Add board URL" className="flex-1">
            <Input
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleRegister()}
              placeholder="https://boards.greenhouse.io/example"
              disabled={register.isPending}
            />
          </Field>
          <Button onClick={handleRegister} disabled={!url.trim() || register.isPending}>
            <Plus className="h-3 w-3" /> register
          </Button>
        </div>
        {registerError ? (
          <div className="mt-2 rounded-md border border-danger/30 bg-danger-soft p-2 text-xs text-danger">
            {registerError}
          </div>
        ) : null}
      </div>

      {isLoading ? (
        <LoadingRegion label="loading roster…" className="space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <SkeletonBlock key={i} className="h-12 w-full" />
          ))}
        </LoadingRegion>
      ) : null}
      {error ? <ErrorState error={error} /> : null}
      {data && data.employers.length === 0 ? <EmptyState>No registered employers yet.</EmptyState> : null}

      <ul className="space-y-2">
        {data?.employers.map((e) => (
          <RosterRow
            key={e.id}
            employer={e}
            onRemove={() => remove.mutate(e.id)}
          />
        ))}
      </ul>
    </div>
  );
}

function RosterRow({ employer: e, onRemove }: { employer: EmployerBoardDto; onRemove: () => void }) {
  return (
    <li className="flex flex-col gap-2 rounded-md border border-border bg-surface-secondary/60 p-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="font-medium text-foreground">{e.displayName}</span>
          <Chip>{e.vendor}</Chip>
          {e.enabled ? (
            <span title="enabled"><ToggleRight className="h-4 w-4 text-success" /></span>
          ) : (
            <span title="disabled"><ToggleLeft className="h-4 w-4 text-faint" /></span>
          )}
          {e.stale ? <Chip tone="red">stale</Chip> : null}
        </div>
        <div className="mt-1 text-xs text-muted">
          <span>{e.employerIdentifier}</span>
          <span className="mx-1">&#183;</span>
          <span>added via {e.addedVia}</span>
          {e.lastSuccessAt ? (
            <>
              <span className="mx-1">&#183;</span>
              <span>last success {new Date(e.lastSuccessAt).toLocaleString()}</span>
            </>
          ) : null}
          {e.lastPostingCount > 0 ? (
            <>
              <span className="mx-1">&#183;</span>
              <span>{e.lastPostingCount} postings</span>
            </>
          ) : null}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button variant="ghost" onClick={onRemove}>
          <Trash2 className="h-3 w-3" />
        </Button>
      </div>
    </li>
  );
}
