import { FileUp, Trash2, Check, AlertCircle, UserRound, Settings2, Plus } from 'lucide-react';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import type { EntryType, Resume, Section } from '@job-finder/shared';
import { PageHeader, SectionTitle, Tile } from '../../components/layout';
import {
  Button,
  ErrorState,
  Field,
  Input,
  LoadingRegion,
  Spinner,
  SkeletonBlock,
  SkeletonLine,
  Tabs,
  type TabItem,
} from '../../components/ui';
import { ConfirmDialog } from './components/ConfirmDialog';
import { IdentityForm } from './components/IdentityForm';
import { SectionTabPanel } from './components/SectionTabPanel';
import { EntryTypePicker } from './components/SectionEditor';
import {
  useConfigStatus,
  useCreateProfile,
  useDeleteProfile,
  useProfiles,
  useResume,
  useUpdateResume,
  useUploadConfig,
} from './hooks';

const DEFAULT_PROFILE_NAME = 'My Profile';

export default function ProfilePage() {
  const { data: profiles, isLoading: profilesLoading, error: profilesError } = useProfiles();
  const profile = profiles?.[0];

  const createProfile = useCreateProfile();
  const creatingRef = useRef(false);

  useEffect(() => {
    if (!profilesLoading && !profile && !creatingRef.current) {
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
    // eslint-disable-next-line react-hooks/set-state-in-effect
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
        title={`Profile: ${profileName}`}
        description="Your resume, fully editable. Uploading a config is optional — it just pre-fills these fields."
        actions={
          <>
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
            <Button onClick={handleSave} disabled={updateResume.isPending || !draft}>
              {updateResume.isPending ? <Spinner /> : 'save resume'}
            </Button>
          </>
        }
      />

      <input ref={fileRef} type="file" accept=".yaml,.yml" className="hidden" onChange={handleFileChange} />

      {uploadConfig.isPending ? <Spinner label="rendering a test PDF…" /> : null}
      {uploadConfig.error ? <ErrorState error={uploadConfig.error} /> : null}
      {updateResume.error ? <ErrorState error={updateResume.error} /> : null}
      {error ? <ErrorState error={error} /> : null}

      {isLoading || !draft ? (
        <LoadingRegion label="loading resume…">
          <SkeletonLine width="w-1/3" className="h-5" />
          <SkeletonBlock className="mt-3 h-32 w-full" />
        </LoadingRegion>
      ) : (
        <ResumeTabs
          draft={draft}
          onChange={setDraft}
          fileRef={fileRef}
          isUploading={uploadConfig.isPending}
          isRemoving={removeProfile.isPending}
          onRequestDelete={() => setPendingRemove(true)}
        />
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

interface ResumeTabsProps {
  draft: Resume;
  onChange: (resume: Resume) => void;
  fileRef: React.RefObject<HTMLInputElement | null>;
  isUploading: boolean;
  isRemoving: boolean;
  onRequestDelete: () => void;
}

function ResumeTabs({ draft, onChange, fileRef, isUploading, isRemoving, onRequestDelete }: ResumeTabsProps) {
  const { tab: rawTab } = useParams();
  const navigate = useNavigate();
  const [newSectionName, setNewSectionName] = useState('');
  const [newSectionType, setNewSectionType] = useState<EntryType>('bullet');

  const tab = normalizeTab(rawTab, draft.sections.length);
  const setTab = (id: string) => navigate(id === 'details' ? '/profile' : `/profile/${id}`);

  const setSections = (sections: Section[]) => onChange({ ...draft, sections });

  const renameSection = (i: number, name: string) => {
    const sections = [...draft.sections];
    sections[i] = { ...sections[i], name };
    setSections(sections);
  };

  const updateSection = (i: number, section: Section) => {
    const sections = [...draft.sections];
    sections[i] = section;
    setSections(sections);
  };

  const moveSection = (i: number, dir: -1 | 1) => {
    const j = i + dir;
    if (j < 0 || j >= draft.sections.length) return;
    const sections = [...draft.sections];
    const [moved] = sections.splice(i, 1);
    sections.splice(j, 0, moved);
    setSections(sections);
    if (tab === `section-${i}`) setTab(`section-${j}`);
    else if (tab === `section-${j}`) setTab(`section-${i}`);
  };

  const deleteSection = (i: number) => {
    setSections(draft.sections.filter((_, idx) => idx !== i));
    if (tab === `section-${i}`) setTab('details');
  };

  const addSection = () => {
    const name = newSectionName.trim();
    if (!name) return;
    const sections = [...draft.sections, { name, entryType: newSectionType, entries: [] }];
    setSections(sections);
    setTab(`section-${sections.length - 1}`);
    setNewSectionName('');
  };

  const tabs: TabItem[] = [
    { id: 'details', label: 'Details', icon: <UserRound className="h-4 w-4" /> },
    ...draft.sections.map((s, i) => ({ id: `section-${i}`, label: s.name.trim() || 'Untitled' })),
    { id: 'add', label: 'Add section', icon: <Plus className="h-4 w-4" /> },
    { id: 'config', label: 'Config', icon: <Settings2 className="h-4 w-4" /> },
  ];

  return (
    <>
      <Tabs aria-label="Profile" variant="underline" className="mb-4" tabs={tabs} active={tab} onChange={setTab} />

      <Tile>
        <TabPanel id="details" active={tab}>
          <IdentityForm resume={draft} onChange={onChange} />
        </TabPanel>

        {draft.sections.map((section, i) => (
          <TabPanel key={i} id={`section-${i}`} active={tab}>
            <SectionTabPanel
              section={section}
              canMoveUp={i > 0}
              canMoveDown={i < draft.sections.length - 1}
              onRename={(name) => renameSection(i, name)}
              onMove={(dir) => moveSection(i, dir)}
              onDelete={() => deleteSection(i)}
              onChange={(s) => updateSection(i, s)}
            />
          </TabPanel>
        ))}

        <TabPanel id="add" active={tab}>
          <SectionTitle>Add section</SectionTitle>
          {draft.sections.length === 0 ? (
            <p className="mb-3 text-sm text-muted">
              This resume has no sections yet. Add one to start building — this is a valid starting point, not an error.
            </p>
          ) : null}
          <div className="flex flex-wrap items-end gap-2">
            <div className="flex-1">
              <Field label="Name">
                <Input
                  value={newSectionName}
                  onChange={(e) => setNewSectionName(e.target.value)}
                  placeholder="New section name (e.g. Volunteering)"
                />
              </Field>
            </div>
            <EntryTypePicker value={newSectionType} onChange={(v) => setNewSectionType(v as EntryType)} />
            <Button onClick={addSection} disabled={!newSectionName.trim()}>
              <Plus className="h-4 w-4" /> add section
            </Button>
          </div>
        </TabPanel>

        <TabPanel id="config" active={tab}>
          <div>
            <SectionTitle>Import config</SectionTitle>
            <p className="mb-3 text-sm text-muted">
              Import a RenderCV YAML config to pre-fill every field. Any existing resume content will be overwritten.
            </p>
            <Button variant="secondary" onClick={() => fileRef.current?.click()} disabled={isUploading}>
              <FileUp className="h-4 w-4" /> import config
            </Button>
          </div>

          <div className="mt-6 rounded-xl border border-danger/30 bg-danger-soft p-4">
            <h2 className="mb-1 [font:var(--type-section-title)] text-danger">Delete profile</h2>
            <p className="mb-3 text-sm text-muted">This permanently deletes the profile and its resume content.</p>
            <Button variant="danger" onClick={onRequestDelete} disabled={isRemoving}>
              <Trash2 className="h-4 w-4" /> delete profile
            </Button>
          </div>
        </TabPanel>
      </Tile>
    </>
  );
}

function TabPanel({ id, active, children }: { id: string; active: string; children: ReactNode }) {
  if (id !== active) return null;
  return (
    <div role="tabpanel" id={`panel-${id}`} aria-labelledby={`tab-${id}`} tabIndex={0}>
      {children}
    </div>
  );
}

function normalizeTab(raw: string | undefined, sectionCount: number): string {
  if (!raw) return 'details';
  if (raw === 'details' || raw === 'add' || raw === 'config') return raw;
  const match = /^section-(\d+)$/.exec(raw);
  if (match && Number(match[1]) < sectionCount) return raw;
  return 'details';
}
