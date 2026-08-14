import { Plus, Trash2, ToggleRight, ToggleLeft } from 'lucide-react';
import { useState } from 'react';
import type { EmployerBoardDto } from '@job-finder/shared';
import { ListRow } from '../../../components/layout';
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
      <div className="mb-3 rounded-xl border border-border bg-surface-secondary p-4">
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
          <div className="mt-2 rounded-lg border border-danger/30 bg-danger-soft p-2 text-xs text-danger">
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

      <div>
        {data?.employers.map((e) => (
          <RosterRow
            key={e.id}
            employer={e}
            onRemove={() => remove.mutate(e.id)}
          />
        ))}
      </div>
    </div>
  );
}

function RosterRow({ employer: e, onRemove }: { employer: EmployerBoardDto; onRemove: () => void }) {
  return (
    <ListRow
      title={
        <span className="inline-flex items-center gap-2">
          {e.displayName}
          <Chip>{e.vendor}</Chip>
          {e.enabled ? (
            <span title="enabled"><ToggleRight className="h-4 w-4 text-success" /></span>
          ) : (
            <span title="disabled"><ToggleLeft className="h-4 w-4 text-faint" /></span>
          )}
          {e.stale ? <Chip tone="red">stale</Chip> : null}
        </span>
      }
      meta={
        <>
          {e.employerIdentifier} · added via {e.addedVia}
          {e.lastSuccessAt ? ` · last success ${new Date(e.lastSuccessAt).toLocaleString()}` : ''}
          {e.lastPostingCount > 0 ? ` · ${e.lastPostingCount} postings` : ''}
        </>
      }
      aside={
        <Button variant="ghost" onClick={onRemove}>
          <Trash2 className="h-3 w-3" />
        </Button>
      }
    />
  );
}
