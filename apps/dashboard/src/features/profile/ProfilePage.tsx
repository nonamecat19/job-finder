import { FileUp, Pencil, Trash2 } from 'lucide-react';
import { useRef, useState } from 'react';
import type { JsonResume, ProfileDto } from '@job-finder/shared';
import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import {
  Button,
  EmptyState,
  ErrorState,
  Field,
  Input,
  Spinner,
  Surface,
  Textarea,
} from '../../components/ui';
import { useCreateProfile, useDeleteProfile, useImportProfile, useProfiles, useUpdateProfile } from './hooks';

export default function ProfilePage() {
  const { data: profiles, isLoading, error } = useProfiles();
  const importProfile = useImportProfile();
  const fileRef = useRef<HTMLInputElement>(null);
  const [draft, setDraft] = useState<{ name: string; document: JsonResume } | null>(null);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    importProfile.mutate(file, {
      onSuccess: (res) => setDraft({ name: '', document: res.draft }),
    });
    e.target.value = '';
  };

  return (
    <div>
      <PageHeader
        title="Profile"
        description="Your resume data used for matching and document generation."
        actions={
          <>
            <input ref={fileRef} type="file" accept=".pdf" className="hidden" onChange={handleFileChange} />
            <Button onClick={() => fileRef.current?.click()} disabled={importProfile.isPending}>
              <FileUp className="h-4 w-4" /> import PDF
            </Button>
          </>
        }
      />

      {importProfile.isPending ? <Spinner label="importing PDF…" /> : null}
      {importProfile.error ? <ErrorState error={importProfile.error} /> : null}
      {error ? <ErrorState error={error} /> : null}

      {draft ? <ProfileDraft draft={draft} onSave={() => setDraft(null)} onCancel={() => setDraft(null)} /> : null}

      {isLoading ? <Spinner label="loading profiles…" /> : null}
      {profiles && profiles.length === 0 && !draft ? (
        <EmptyState>No profiles yet. Import a PDF resume or create one manually.</EmptyState>
      ) : null}

      <ul className="space-y-3">
        {profiles?.map((p) => (
          <ProfileCard key={p.id} profile={p} />
        ))}
      </ul>
    </div>
  );
}

function ProfileDraft({
  draft,
  onSave,
  onCancel,
}: {
  draft: { name: string; document: JsonResume };
  onSave: () => void;
  onCancel: () => void;
}) {
  const create = useCreateProfile();
  const [name, setName] = useState(draft.name);
  const [notes, setNotes] = useState('');

  return (
    <Surface className="mb-4 border-primary/30">
      <SectionTitle>New profile from import</SectionTitle>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Name">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. John Doe" />
        </Field>
        <Field label="Extra notes">
          <Input value={notes} onChange={(e) => setNotes(e.target.value)} placeholder="optional" />
        </Field>
      </div>
      <div className="mt-3 flex gap-2">
        <Button
          onClick={() => {
            if (!name.trim()) return;
            create.mutate(
              { name: name.trim(), document: draft.document, extraNotes: notes || undefined },
              { onSuccess: onSave },
            );
          }}
          disabled={!name.trim() || create.isPending}
        >
          save profile
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          cancel
        </Button>
      </div>
    </Surface>
  );
}

function ProfileCard({ profile }: { profile: ProfileDto }) {
  const update = useUpdateProfile();
  const remove = useDeleteProfile();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(profile.name);
  const [notes, setNotes] = useState(profile.extraNotes ?? '');

  const basics = profile.document.basics;

  if (editing) {
    return (
      <Surface className="border-primary/30">
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Name">
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Field label="Extra notes">
            <Input value={notes} onChange={(e) => setNotes(e.target.value)} />
          </Field>
        </div>
        <div className="mt-3 flex gap-2">
          <Button
            onClick={() =>
              update.mutate(
                { id: profile.id, body: { name, extraNotes: notes || null } },
                { onSuccess: () => setEditing(false) },
              )
            }
            disabled={update.isPending}
          >
            save
          </Button>
          <Button variant="ghost" onClick={() => setEditing(false)}>
            cancel
          </Button>
        </div>
      </Surface>
    );
  }

  return (
    <Surface>
      <div className="flex items-start justify-between">
        <div>
          <h3 className="font-semibold text-fg">{profile.name}</h3>
          {basics?.email ? <p className="text-sm text-muted">{basics.email}</p> : null}
          {basics?.summary ? <p className="mt-1 text-xs text-faint line-clamp-2">{basics.summary}</p> : null}
          {profile.extraNotes ? <p className="mt-1 text-xs text-faint">Notes: {profile.extraNotes}</p> : null}
          <p className="mt-1 text-xs text-faint">
            updated {new Date(profile.updatedAt).toLocaleString()}
          </p>
        </div>
        <div className="flex gap-1">
          <Button variant="ghost" onClick={() => setEditing(true)}>
            <Pencil className="h-4 w-4" />
          </Button>
          <Button variant="ghost" onClick={() => remove.mutate(profile.id)}>
            <Trash2 className="h-4 w-4 text-danger" />
          </Button>
        </div>
      </div>
      {profile.document.skills?.length ? (
        <div className="mt-2 flex flex-wrap gap-1">
          {profile.document.skills.slice(0, 8).map((s) => (
            <span key={s.name} className="rounded bg-overlay px-1.5 py-0.5 text-xs text-muted">
              {s.name}
            </span>
          ))}
        </div>
      ) : null}
    </Surface>
  );
}
