import { Plus } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import type { ManualAddResultDto } from '@job-finder/shared';
import { Button, Field, Input } from '../../components/ui';
import { ManualFillInDialog } from './ManualFillInDialog';
import { useAddVacancyByUrl } from './hooks';

const RECOVERY: Record<string, string> = {
  invalid_url: 'Check the address and try again.',
  not_a_posting: 'That is a search page — add it as a subscription on the Sources page instead.',
  no_reader: 'No source reads that site yet.',
  unreachable: 'Try again, or open the posting to check it still exists.',
  blocked: 'The site refused the request.',
  timed_out: 'Try again — the site was slow to respond.',
  incomplete: 'The page loaded but did not carry enough to store.',
};

export function AddVacancyForm({
  onNeedsFillIn,
  onCreated,
}: {
  onNeedsFillIn?: (result: ManualAddResultDto) => void;
  onCreated?: (jobId: string) => void;
}) {
  const [url, setUrl] = useState('');

  const [fillIn, setFillIn] = useState<ManualAddResultDto | null>(null);
  const add = useAddVacancyByUrl();
  const navigate = useNavigate();
  const result = add.data;

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = url.trim();

    if (!trimmed || add.isPending) return;
    add.mutate(trimmed, {
      onSuccess: (r) => {
        if (r.outcome === 'created') {
          setUrl('');
          if (r.job) onCreated?.(r.job.id);
          return;
        }
        if (r.outcome === 'duplicate' && r.job) {
          navigate(`/jobs/${r.job.id}`);
          return;
        }
        if (r.outcome === 'needs_fill_in') {
          if (r.draft) setFillIn(r);
          onNeedsFillIn?.(r);
        }
      },
    });
  };

  return (
    <>
      <form onSubmit={submit} aria-label="Add a vacancy by URL">
      <div className="flex flex-wrap items-end gap-2">
        <Field label="Add a vacancy by URL">
          <Input
            type="url"
            placeholder="https://djinni.co/jobs/123456-senior-go-engineer/"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            disabled={add.isPending}
            aria-label="Posting URL"
            className="w-full min-w-[20rem]"
          />
        </Field>
        <Button type="submit" disabled={add.isPending || url.trim() === ''}>
          <Plus className="h-4 w-4" aria-hidden="true" />
          {add.isPending ? 'adding…' : 'add'}
        </Button>
      </div>

      {add.isPending ? (
        <p role="status" className="mt-2 text-sm text-muted">
          Reading the posting…
        </p>
      ) : null}

      {add.isError ? (
        <p role="alert" className="mt-2 text-sm text-danger">
          The vacancy could not be added. Try again.
        </p>
      ) : null}

      {!add.isPending && result && !fillIn ? <Outcome result={result} /> : null}
      </form>

      {}
      {fillIn?.draft ? (
        <ManualFillInDialog
          draft={fillIn.draft}
          reason={fillIn.reason}
          onClose={() => setFillIn(null)}
          onCreated={(jobId) => {
            setUrl('');
            onCreated?.(jobId);
          }}
        />
      ) : null}
    </>
  );
}

function Outcome({ result }: { result: ManualAddResultDto }) {
  if (result.outcome === 'created') {
    return (
      <p role="status" className="mt-2 text-sm text-success">
        Added {result.job?.title ?? 'the vacancy'} — it is at the top of your feed.
      </p>
    );
  }
  if (result.outcome === 'duplicate') {
    return (
      <p role="status" className="mt-2 text-sm text-muted">
        {result.reason ?? 'This vacancy is already in your feed.'}
      </p>
    );
  }
  const recovery = result.kind ? RECOVERY[result.kind] : undefined;
  return (
    <p role="alert" className="mt-2 text-sm text-danger">
      {result.reason}
      {recovery ? <span className="ml-1 text-muted">{recovery}</span> : null}
    </p>
  );
}
