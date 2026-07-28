import { FileUp, Trash2, Check, AlertCircle } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import type { Resume } from '@job-finder/shared';
import { PageHeader } from '../../components/layout/PageHeader';
import { DashboardGrid, Tile } from '../../components/layout';
import { Button, EmptyState, ErrorState, LoadingRegion, Spinner, SkeletonBlock, SkeletonLine } from '../../components/ui';
import { ConfirmDialog } from './components/ConfirmDialog';
import { IdentityForm } from './components/IdentityForm';
import { SectionList } from './components/SectionList';
import {
  useConfigStatus,
  useCreateProfile,
  useDeleteProfile,
  useProfiles,
  useResume,
  useUpdateResume,
  useUploadConfig,
} from './hooks';

// Profile.name (the record label) is distinct from Resume.name (the person's
// name on the resume itself, edited in IdentityForm) — this default is never
// shown to the user, it just satisfies the backend's required-name field.
const DEFAULT_PROFILE_NAME = 'My Profile';

export default function ProfilePage() {
  const { data: profiles, isLoading: profilesLoading, error: profilesError } = useProfiles();
  const profile = profiles?.[0];

  const createProfile = useCreateProfile();
  const creatingRef = useRef(false);

  useEffect(() => {
    if (!profilesLoading && !profile && !creatingRef.current) {
      // No profile yet: create a blank one silently so the user lands
      // directly on the full editable form + import button (FR-001, FR-012)
      // instead of being gated behind a "name it first" step.
      creatingRef.current = true;
      createProfile.mutate({ name: DEFAULT_PROFILE_NAME });
    }
  }, [profilesLoading, profile, createProfile]);

  if (profilesLoading || !profile) {
    return (
      <div>
        <PageHeader title="Profile" description="Your resume, fully editable." />
        <LoadingRegion label="loading profile…">
          <SkeletonLine width="w-1/3" className="h-5" />
          <SkeletonBlock className="mt-3 h-32 w-full" />
        </LoadingRegion>
        {profilesError ? <ErrorState error={profilesError} /> : null}
        {createProfile.error ? <ErrorState error={createProfile.error} /> : null}
      </div>
    );
  }

  return <ProfileEditor profileId={profile.id} profileName={profile.name} />;
}

function ProfileEditor({ profileId, profileName }: { profileId: string; profileName: string }) {
  const { data: resumeDto, isLoading, error } = useResume(profileId);
  const updateResume = useUpdateResume(profileId);
  const uploadConfig = useUploadConfig();
  const configStatus = useConfigStatus();
  const removeProfile = useDeleteProfile();

  const fileRef = useRef<HTMLInputElement>(null);
  const [draft, setDraft] = useState<Resume | null>(null);
  const [pendingUploadFile, setPendingUploadFile] = useState<File | null>(null);
  const [pendingRemove, setPendingRemove] = useState(false);
  const [saveState, setSaveState] = useState<'idle' | 'saved' | 'error'>('idle');

  useEffect(() => {
    if (resumeDto) setDraft(resumeDto.resume);
  }, [resumeDto]);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (configStatus.data?.hasExistingContent) {
      setPendingUploadFile(file);
    } else {
      uploadConfig.mutate(file);
    }
    e.target.value = '';
  };

  const confirmUpload = () => {
    if (pendingUploadFile) uploadConfig.mutate(pendingUploadFile);
    setPendingUploadFile(null);
  };

  const handleSave = () => {
    if (!draft) return;
    setSaveState('idle');
    updateResume.mutate(draft, {
      onSuccess: () => setSaveState('saved'),
      onError: () => setSaveState('error'),
    });
  };

  return (
    <div>
      <PageHeader
        title="Profile"
        description="Your resume, fully editable. Uploading a config is optional — it just pre-fills these fields."
        actions={
          <>
            <input ref={fileRef} type="file" accept=".yaml,.yml" className="hidden" onChange={handleFileChange} />
            <Button variant="secondary" onClick={() => fileRef.current?.click()} disabled={uploadConfig.isPending}>
              <FileUp className="h-4 w-4" /> import config
            </Button>
            <Button variant="ghost" onClick={() => setPendingRemove(true)}>
              <Trash2 className="h-4 w-4 text-danger" /> delete profile
            </Button>
          </>
        }
      />

      {uploadConfig.isPending ? <Spinner label="rendering a test PDF…" /> : null}
      {uploadConfig.error ? <ErrorState error={uploadConfig.error} /> : null}
      {error ? <ErrorState error={error} /> : null}

      {isLoading || !draft ? (
        <LoadingRegion label="loading resume…">
          <SkeletonLine width="w-1/3" className="h-5" />
          <SkeletonBlock className="mt-3 h-32 w-full" />
        </LoadingRegion>
      ) : (
        <DashboardGrid>
          <Tile
            span="full"
            title={`Profile: ${profileName}`}
            action={
              <div className="flex items-center gap-2">
                {saveState === 'saved' && !updateResume.isPending ? (
                  <span className="inline-flex items-center gap-1 text-sm text-success">
                    <Check className="h-4 w-4" /> saved
                  </span>
                ) : null}
                {saveState === 'error' ? (
                  <span className="inline-flex items-center gap-1 text-sm text-danger">
                    <AlertCircle className="h-4 w-4" /> save failed
                  </span>
                ) : null}
                <Button onClick={handleSave} disabled={updateResume.isPending}>
                  {updateResume.isPending ? <Spinner /> : 'save resume'}
                </Button>
              </div>
            }
          />
          {updateResume.error ? <Tile span="full"><ErrorState error={updateResume.error} /></Tile> : null}

          <Tile span="standard">
            <IdentityForm resume={draft} onChange={setDraft} />
          </Tile>

          {draft.sections.length === 0 ? (
            <Tile span="full">
              <EmptyState>
                This resume has no sections yet. Add one below to start building — this is a valid starting point, not an
                error.
              </EmptyState>
            </Tile>
          ) : null}
          <Tile span="wide">
            <SectionList sections={draft.sections} onChange={(sections) => setDraft({ ...draft, sections })} />
          </Tile>
        </DashboardGrid>
      )}

      <ConfirmDialog
        open={pendingUploadFile !== null}
        title="Replace existing resume content?"
        description="This profile already has resume content. Uploading a new config will overwrite it."
        confirmLabel="Replace"
        onConfirm={confirmUpload}
        onCancel={() => setPendingUploadFile(null)}
      />
      <ConfirmDialog
        open={pendingRemove}
        title="Delete this profile?"
        description="This permanently deletes the profile and its resume content."
        confirmLabel="Delete"
        onConfirm={() => {
          removeProfile.mutate(profileId);
          setPendingRemove(false);
        }}
        onCancel={() => setPendingRemove(false)}
      />
    </div>
  );
}
