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

      {summary ? (
        <div className="mt-3 space-y-3">
          {summary.skillGroups?.length ? (
            <div>
              <SectionTitle>skills</SectionTitle>
              <div className="mt-1 flex flex-wrap gap-1">
                {summary.skillGroups.map((s: string) => (
                  <span key={s} className="rounded bg-overlay px-1.5 py-0.5 text-xs text-muted">
                    {s}
                  </span>
                ))}
              </div>
            </div>
          ) : null}

          {summary.experience?.length ? (
            <div>
              <SectionTitle>experience</SectionTitle>
              <ul className="mt-1 space-y-1 text-sm text-muted">
                {summary.experience.map((e: { company: string; highlightCount: number }) => (
                  <li key={e.company}>
                    {e.company} — {e.highlightCount} highlight{e.highlightCount === 1 ? '' : 's'}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      ) : null}
    </Surface>
  );
}
