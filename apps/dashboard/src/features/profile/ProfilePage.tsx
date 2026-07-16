import { FileUp, Trash2 } from 'lucide-react';
import { useRef } from 'react';
import type { ProfileDto } from '@job-finder/shared';
import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import { Button, EmptyState, ErrorState, Spinner, Surface } from '../../components/ui';
import { useDeleteProfile, useProfiles, useUploadConfig } from './hooks';

export default function ProfilePage() {
  const { data: profiles, isLoading, error } = useProfiles();
  const uploadConfig = useUploadConfig();
  const fileRef = useRef<HTMLInputElement>(null);

  const profile = profiles?.[0];

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    uploadConfig.mutate(file);
    e.target.value = '';
  };

  return (
    <div>
      <PageHeader
        title="Profile"
        description="Your RenderCV config is the single source of truth for matching and document generation."
        actions={
          <>
            <input
              ref={fileRef}
              type="file"
              accept=".yaml,.yml"
              className="hidden"
              onChange={handleFileChange}
            />
            <Button onClick={() => fileRef.current?.click()} disabled={uploadConfig.isPending}>
              <FileUp className="h-4 w-4" /> {profile?.hasConfig ? 'replace config' : 'upload config'}
            </Button>
          </>
        }
      />

      {uploadConfig.isPending ? <Spinner label="rendering a test PDF…" /> : null}
      {uploadConfig.error ? <ErrorState error={uploadConfig.error} /> : null}
      {error ? <ErrorState error={error} /> : null}

      {isLoading ? <Spinner label="loading profile…" /> : null}

      {!isLoading && !profile ? (
        <EmptyState>Upload your RenderCV config (.yaml) to begin.</EmptyState>
      ) : null}

      {profile ? <ProfileCard profile={profile} /> : null}
    </div>
  );
}

function ProfileCard({ profile }: { profile: ProfileDto }) {
  const remove = useDeleteProfile();
  const summary = profile.rendercvConfig;
  const full = profile.rendercvFull;

  return (
    <Surface>
      <div className="flex items-start justify-between">
        <div>
          <h3 className="font-semibold text-fg">{profile.name}</h3>
          {summary?.headline ? <p className="text-sm text-muted">{summary.headline}</p> : null}
          {profile.extraNotes ? <p className="mt-1 text-xs text-faint">Notes: {profile.extraNotes}</p> : null}
          <p className="mt-1 text-xs text-faint">updated {new Date(profile.updatedAt).toLocaleString()}</p>
        </div>
        <Button variant="ghost" onClick={() => remove.mutate(profile.id)}>
          <Trash2 className="h-4 w-4 text-danger" />
        </Button>
      </div>

      {!profile.hasConfig ? (
        <p className="mt-2 text-sm text-danger">No valid config uploaded yet.</p>
      ) : null}

      {full ? (
        <div className="mt-3 space-y-3 text-sm">
          {Object.entries(full).map(([key, value]) => (
            <div key={key}>
              <SectionTitle>{key}</SectionTitle>
              <div className="mt-1 text-muted">
                <DataValue value={value} />
              </div>
            </div>
          ))}
        </div>
      ) : null}
    </Surface>
  );
}

function DataValue({ value }: { value: unknown }) {
  if (value === null || value === undefined || value === '') return null;

  if (Array.isArray(value)) {
    if (value.length === 0) return null;
    return (
      <ul className="ml-3 list-disc space-y-1 marker:text-faint">
        {value.map((item, i) => (
          <li key={i}>{isPlainObject(item) || Array.isArray(item) ? <DataValue value={item} /> : String(item)}</li>
        ))}
      </ul>
    );
  }

  if (isPlainObject(value)) {
    const entries = Object.entries(value).filter(
      ([, v]) => v !== null && v !== undefined && v !== '' && !(Array.isArray(v) && v.length === 0),
    );
    if (entries.length === 0) return null;
    return (
      <dl className="ml-3 space-y-1">
        {entries.map(([k, v]) => (
          <div key={k} className="flex gap-1">
            <dt className="shrink-0 text-faint">{k}:</dt>
            <dd className="flex-1">{isPlainObject(v) || Array.isArray(v) ? <DataValue value={v} /> : String(v)}</dd>
          </div>
        ))}
      </dl>
    );
  }

  return <>{String(value)}</>;
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
