import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import type { ManualAddResultDto, ManualVacancyDraftDto } from '@job-finder/shared';
import { Button, Field, Input, Textarea } from '../../components/ui';
import { useSaveManualVacancy } from './hooks';

const REQUIRED = ['title', 'company', 'description'] as const;
type RequiredField = (typeof REQUIRED)[number];

const LABELS: Record<RequiredField, string> = {
  title: 'Title',
  company: 'Company',
  description: 'Description',
};

export function ManualFillInDialog({
  draft,
  reason,
  onClose,
  onCreated,
}: {
  draft: ManualVacancyDraftDto;
  reason?: string | null;
  onClose: () => void;
  onCreated?: (jobId: string) => void;
}) {
  const [form, setForm] = useState({
    title: draft.title ?? '',
    company: draft.company ?? '',
    description: draft.description ?? '',
    location: draft.location ?? '',
    salaryRaw: draft.salaryRaw ?? '',
    remote: draft.remote,
  });
  const [missing, setMissing] = useState<RequiredField[]>([]);
  const [result, setResult] = useState<ManualAddResultDto | null>(null);
  const save = useSaveManualVacancy();
  const navigate = useNavigate();

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (save.isPending) return;

    const blank = REQUIRED.filter((f) => form[f].trim() === '');
    setMissing(blank);
    if (blank.length > 0) return;

    save.mutate(
      {
        ...draft,
        title: form.title.trim(),
        company: form.company.trim(),
        description: form.description,
        location: form.location.trim() || undefined,
        salaryRaw: form.salaryRaw.trim() || undefined,
        remote: form.remote,
      },
      {
        onSuccess: (r) => {
          setResult(r);
          if (r.job && (r.outcome === 'created' || r.outcome === 'duplicate')) {
            if (r.outcome === 'created') onCreated?.(r.job.id);
            else navigate(`/jobs/${r.job.id}`);
            onClose();
          }
        },
      },
    );
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Complete this vacancy"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
    >
      <form
        onSubmit={submit}
        className="max-h-full w-full max-w-2xl overflow-y-auto rounded-xl border border-border bg-surface p-5 shadow-lg"
      >
        <h2 className="text-lg font-semibold text-foreground">Complete this vacancy</h2>
        <p className="mt-1 text-sm text-muted">
          {reason ?? 'The page could not be read in full.'} Fill in what is missing and it joins your feed
          like any other vacancy.
        </p>
        <p className="mt-1 truncate text-xs text-faint" title={draft.url}>
          {draft.url}
        </p>

        {missing.length > 0 ? (
          <p role="alert" className="mt-3 text-sm text-danger">
            {missing.map((f) => LABELS[f]).join(', ')} {missing.length === 1 ? 'is' : 'are'} required.
          </p>
        ) : null}

        {result && result.outcome === 'failed' ? (
          <p role="alert" className="mt-3 text-sm text-danger">
            {result.reason}
          </p>
        ) : null}

        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          <Field label="Title">
            <Input
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
              aria-label="Title"
            />
          </Field>
          <Field label="Company">
            <Input
              value={form.company}
              onChange={(e) => setForm({ ...form, company: e.target.value })}
              aria-label="Company"
            />
          </Field>
          <Field label="Location">
            <Input
              value={form.location}
              onChange={(e) => setForm({ ...form, location: e.target.value })}
              aria-label="Location"
            />
          </Field>
          <Field label="Salary">
            <Input
              value={form.salaryRaw}
              onChange={(e) => setForm({ ...form, salaryRaw: e.target.value })}
              aria-label="Salary"
            />
          </Field>
        </div>

        <div className="mt-3">
          <Field label="Description">
            <Textarea
              rows={10}
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              aria-label="Description"
            />
          </Field>
        </div>

        <div className="mt-4 flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            cancel
          </Button>
          <Button type="submit" disabled={save.isPending}>
            {save.isPending ? 'saving…' : 'save vacancy'}
          </Button>
        </div>
      </form>
    </div>
  );
}
