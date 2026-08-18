import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Maximize2, Minimize2 } from 'lucide-react';
import type { GenerationRunDto, GenerationSectionDto } from '@job-finder/shared';
import { PageHeader } from '../../components/layout/PageHeader';
import { Tile, TileEmpty, TileError, TileSkeleton } from '../../components/layout';
import { Button, Spinner, Tabs } from '../../components/ui';
import { PreviewHighlightProvider } from './preview/highlight';
import { useProfiles } from '../profile/hooks';
import CertificationsBlock from './components/CertificationsBlock';
import EducationBlock from './components/EducationBlock';
import GenerateSettingsTab, { GROUNDING_LEVELS } from './components/GenerateSettingsTab';
import JobPickerList from './components/JobPickerList';
import ProjectsBlock from './components/ProjectsBlock';
import ResumePreviewPane from './components/ResumePreviewPane';
import SkillsBlock from './components/SkillsBlock';
import SummaryBlock from './components/SummaryBlock';
import VacancySummaryBar from './components/VacancySummaryBar';
import WorkEntryBlock from './components/WorkEntryBlock';
import {
  useExportGenerationRun,
  useGenerationRun,
  useRerunGenerationRun,
  useReorderGenerationSection,
  useRewriteGenerationItem,
  useSetSectionEnabled,
  useStartGenerationRun,
  useToggleGenerationItem,
} from './hooks';

export default function GenerateWorkspacePage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const jobId = searchParams.get('jobId') ?? undefined;
  const runId = searchParams.get('runId') ?? undefined;

  const { data: run, isLoading, error } = useGenerationRun(runId);
  const { data: profiles } = useProfiles();
  const profileId = profiles?.[0]?.id;
  const startRun = useStartGenerationRun();
  const toggleItem = useToggleGenerationItem(runId);
  const reorderSection = useReorderGenerationSection(runId);
  const exportRun = useExportGenerationRun(runId);
  const rerunRun = useRerunGenerationRun(runId);
  const rewriteItem = useRewriteGenerationItem(runId);
  const setSectionEnabled = useSetSectionEnabled(runId);

  const [groundingLevel, setGroundingLevel] = useState<(typeof GROUNDING_LEVELS)[number]>('moderate');
  const [summaryOptionId, setSummaryOptionId] = useState<string | undefined>(undefined);

  const [previewFullscreen, setPreviewFullscreen] = useState(false);
  const [leftTab, setLeftTab] = useState<'resume' | 'job' | 'settings'>('resume');

  const vacancyJobId = run?.jobId ?? jobId;

  const handleGenerate = () => {
    if (!profileId || !vacancyJobId) return;
    startRun.mutate(
      { profileId, jobId: vacancyJobId, groundingLevel, summaryOptionId },
      { onSuccess: (data) => setSearchParams({ jobId: vacancyJobId, runId: data.runId }) },
    );
  };

  if (!vacancyJobId) {
    return (
      <div className="flex h-full min-h-0 flex-col">
        <PageHeader
          title="Generate resume"
          description="Pick a job to tailor a resume for."
        />
        <div className="mt-4 min-h-0 flex-1 overflow-y-auto">
          <JobPickerList onPick={(pickedId) => setSearchParams({ jobId: pickedId })} />
        </div>
      </div>
    );
  }

  const tileState = !run ? 'empty' : isLoading ? 'loading' : error ? 'error' : 'ready';

  const previewTile = (
    <Tile
      title="PDF preview"
      className="m-auto aspect-a4 h-full min-h-0 w-auto max-w-full"
      contentClassName="min-h-0 flex-1 pt-0"
      action={
        <button
          type="button"
          aria-label={previewFullscreen ? 'exit full screen preview' : 'full screen preview'}
          title={previewFullscreen ? 'exit full screen preview' : 'full screen preview'}
          onClick={() => setPreviewFullscreen((v) => !v)}
          className="rounded-lg p-1 text-faint hover:bg-surface-tertiary hover:text-foreground"
        >
          {previewFullscreen ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
        </button>
      }
    >
      <ResumePreviewPane
        run={run}
        profile={profiles?.find((p) => p.id === profileId)}
        onRemoveItem={(itemId) => toggleItem.mutate({ itemId, selected: false })}
        onReorder={(sectionId, itemIds) => reorderSection.mutate({ sectionId, itemIds })}
        onExport={run ? () => exportRun.mutate() : undefined}
        exportPending={exportRun.isPending}
        exportDisabled={!run || run.state === 'running'}
        exportState={exportRun.data ?? run?.export}
        exportError={exportRun.isError ? (exportRun.error as Error).message : undefined}
        warnings={run ? exportWarnings(run) : undefined}
      />
    </Tile>
  );

  const leftColumn = (
    <div className="flex min-h-0 w-full min-w-0 flex-col gap-4 lg:min-w-[26rem] lg:flex-1">
      {startRun.isError ? <p className="text-sm text-danger">{(startRun.error as Error).message}</p> : null}
      {rerunRun.isError ? <p className="text-sm text-danger">{(rerunRun.error as Error).message}</p> : null}

      <Tile
        action={
          <Tabs
            aria-label="Generate workspace view"
            tabs={[
              { id: 'resume', label: 'Generated resume' },
              { id: 'job', label: 'Job description' },
              { id: 'settings', label: 'Settings' },
            ]}
            active={leftTab}
            onChange={(id) => setLeftTab(id as typeof leftTab)}
          />
        }
        className="min-h-0 flex-1"
        scroll
        scrollLabel={leftTab === 'resume' ? 'Generated resume sections' : leftTab === 'job' ? 'Job description' : 'Settings'}
      >
        {leftTab === 'job' ? (
          <VacancySummaryBar jobId={vacancyJobId} bare />
        ) : leftTab === 'settings' ? (
          <GenerateSettingsTab
            run={run}
            onToggleEnabled={(sectionId, enabled) => setSectionEnabled.mutate({ sectionId, enabled })}
            groundingLevel={groundingLevel}
            onGroundingLevelChange={setGroundingLevel}
            summaryOptionId={summaryOptionId}
            onSummaryOptionIdChange={setSummaryOptionId}
          />
        ) : tileState === 'loading' ? (
          <TileSkeleton className="p-0" />
        ) : tileState === 'error' ? (
          <TileError error={error} />
        ) : tileState === 'empty' ? (
          <TileEmpty>
            <div className="flex flex-col items-center gap-3 text-center">
              <p className="max-w-[44ch] text-sm text-muted">
                No run yet for this vacancy. Generate a resume to review it here as an inspectable list.
              </p>
              <Button disabled={!profileId || startRun.isPending} onClick={handleGenerate}>
                Generate resume
              </Button>
              {startRun.isPending ? <Spinner label="starting generation…" /> : null}
            </div>
          </TileEmpty>
        ) : (
          <WorkspaceLeftPane
            run={run}
            onToggle={(itemId, selected) => toggleItem.mutate({ itemId, selected })}
            onEditText={(itemId, text) => toggleItem.mutate({ itemId, text })}
            onReorder={(sectionId, itemIds) => reorderSection.mutate({ sectionId, itemIds })}
            onDropEntries={(itemId, droppedEntries) => toggleItem.mutate({ itemId, droppedEntries })}
            onRerun={() => rerunRun.mutate(undefined)}
            onRerunSections={(sections) => rerunRun.mutate(sections)}
            onRewrite={(itemId) => rewriteItem.mutateAsync(itemId).then((r) => r.variants)}
            onToggleEnabled={(sectionId, enabled) => setSectionEnabled.mutate({ sectionId, enabled })}
          />
        )}
      </Tile>
    </div>
  );

  return (
    <PreviewHighlightProvider>
      <div className="flex h-full min-h-0 flex-col">
        <div className="flex min-h-0 flex-1 flex-col gap-4 lg:flex-row">
          {leftColumn}

          {}
          <div className="flex min-h-0 w-full shrink-0 flex-col lg:w-auto">
            {previewFullscreen ? (
              <div className="flex min-h-0 flex-1 items-center justify-center rounded-2xl border border-dashed border-border text-sm text-faint">
                Preview is full screen.
              </div>
            ) : (
              previewTile
            )}
          </div>
        </div>

        {previewFullscreen ? (
          <div className="fixed inset-0 z-50 flex bg-background p-4" data-testid="preview-fullscreen">
            {previewTile}
          </div>
        ) : null}
      </div>
    </PreviewHighlightProvider>
  );
}

function exportWarnings(run: GenerationRunDto): string[] {
  const warnings: string[] = [];

  const enabledSections = run.sections.filter((s) => s.enabled !== false);
  const selected = (section: GenerationSectionDto | undefined) =>
    section?.items.filter((i) => i.selected && !i.unavailable) ?? [];

  if (selected(enabledSections.find((s) => s.kind === 'summary')).length === 0) {
    warnings.push('This resume has no summary — nothing is included in the summary section.');
  }
  if (selected(enabledSections.find((s) => s.kind === 'skills')).length === 0) {
    warnings.push('This resume has no skills — every skill group is switched off.');
  }

  for (const [kind, label] of [
    ['projects', 'projects'],
    ['certifications', 'certifications'],
    ['education', 'education'],
  ] as const) {
    const section = enabledSections.find((s) => s.kind === kind);
    if (section && section.items.length > 0 && selected(section).length === 0) {
      warnings.push(`This resume has no ${label} — every ${label.replace(/s$/, '')} is switched off.`);
    }
  }

  const aiIncluded = enabledSections
    .filter((s) => s.kind !== 'summary')
    .flatMap((s) => selected(s))
    .filter((i) => i.origin === 'ai').length;
  if (aiIncluded > 0) {
    warnings.push(
      `${aiIncluded} AI-written item${aiIncluded === 1 ? '' : 's'} ${aiIncluded === 1 ? 'is' : 'are'} included. ` +
        'AI-written content is unverified — read it before you send this resume.',
    );
  }

  return warnings;
}

const SECTION_KIND_LABELS: Record<string, string> = {
  summary: 'Summary',
  skills: 'Skills',
  projects: 'Projects',
  certifications: 'Certifications',
  education: 'Education',
};

function failedSections(run: GenerationRunDto): { id: string; label: string }[] {
  return run.sections
    .filter((s) => s.state === 'failed')
    .map((s) => ({
      id: s.id,
      label: s.entryKey ?? s.entryLabel ?? SECTION_KIND_LABELS[s.kind] ?? s.kind,
    }));
}

const RERUN_WARNING =
  "Re-running replaces the AI's ordering for this section. Your checkbox choices, positions and edits carry over where the same content still exists. Continue?";

function WorkspaceLeftPane({
  run,
  onToggle,
  onEditText,
  onReorder,
  onDropEntries,
  onRerun,
  onRerunSections,
  onRewrite,
  onToggleEnabled,
}: {
  run: GenerationRunDto | undefined;
  onToggle: (itemId: string, selected: boolean) => void;
  onEditText: (itemId: string, text: string) => void;
  onReorder: (sectionId: string, itemIds: string[]) => void;
  onDropEntries: (itemId: string, droppedEntries: string[]) => void;
  onRerun: () => void;
  onRerunSections: (sections: string[]) => void;
  onRewrite: (itemId: string) => Promise<string[]>;
  onToggleEnabled: (sectionId: string, enabled: boolean) => void;
}) {
  if (!run) {
    return null;
  }
  if (run.state === 'running') {
    return (
      <div className="flex items-center gap-2 py-8 text-sm text-muted" role="status">
        <Spinner label="generating resume…" />
      </div>
    );
  }

  const summarySection = run.sections.find((s) => s.kind === 'summary');
  const skillsSection = run.sections.find((s) => s.kind === 'skills');
  const projectsSection = run.sections.find((s) => s.kind === 'projects');
  const certificationsSection = run.sections.find((s) => s.kind === 'certifications');
  const educationSection = run.sections.find((s) => s.kind === 'education');
  const experienceSections = run.sections
    .filter((s): s is GenerationSectionDto => s.kind === 'experience')
    .sort((a, b) => a.position - b.position);

  return (
    <div className="space-y-4" data-testid="workspace-sections">
      {run.masterChanged ? (
        <div
          className="flex items-center justify-between gap-3 rounded-xl bg-warning-soft px-3 py-2.5 text-xs text-warning"
          data-testid="master-changed-banner"
        >
          <span>
            Your profile has changed since this run started. Selections that no longer match your profile are shown
            as unavailable below. Re-run to pick up the current profile.
          </span>
          <Button
            variant="secondary"
            onClick={() => {
              if (window.confirm("Re-running replaces the AI's ordering for every section. Continue?")) onRerun();
            }}
          >
            Re-run
          </Button>
        </div>
      ) : null}
      {run.state === 'partial' ? (
        <p className="rounded-xl bg-warning-soft px-3 py-2.5 text-xs text-warning">
          Some sections did not finish generating. Completed sections are shown below.
        </p>
      ) : null}
      {run.state === 'failed' ? (
        <p className="rounded-xl bg-danger-soft px-3 py-2.5 text-xs text-danger">
          This generation run failed.
        </p>
      ) : null}

      {failedSections(run).length > 0 ? (
        <div className="space-y-1 rounded-xl border border-danger/30 bg-danger-soft px-3 py-2.5" data-testid="failed-sections">
          <p className="text-xs text-danger">These sections failed to generate:</p>
          {failedSections(run).map((s) => (
            <div key={s.id} className="flex items-center justify-between gap-2 text-xs">
              <span className="text-muted">{s.label}</span>
              <Button
                variant="secondary"
                onClick={() => {
                  if (window.confirm(RERUN_WARNING)) onRerunSections([s.id]);
                }}
              >
                Retry section
              </Button>
            </div>
          ))}
        </div>
      ) : null}

      {summarySection ? (
        <SummaryBlock
          section={summarySection}
          onToggle={onToggle}
          onEditText={onEditText}
          onRegenerate={() => {
            if (window.confirm(RERUN_WARNING)) onRerunSections([summarySection.id]);
          }}
          onToggleEnabled={onToggleEnabled}
        />
      ) : null}

      {experienceSections.map((section) => (
        <WorkEntryBlock
          key={section.id}
          section={section}
          onToggle={onToggle}
          onReorder={onReorder}
          onEditText={onEditText}
          onRewrite={onRewrite}
          onToggleEnabled={onToggleEnabled}
        />
      ))}

      {skillsSection ? (
        <SkillsBlock
          section={skillsSection}
          onToggle={onToggle}
          onReorder={onReorder}
          onEditText={onEditText}
          onDropEntries={onDropEntries}
          onToggleEnabled={onToggleEnabled}
        />
      ) : null}

      {projectsSection ? (
        <ProjectsBlock
          section={projectsSection}
          onToggle={onToggle}
          onReorder={onReorder}
          onToggleEnabled={onToggleEnabled}
        />
      ) : null}

      {certificationsSection ? (
        <CertificationsBlock
          section={certificationsSection}
          onToggle={onToggle}
          onReorder={onReorder}
          onToggleEnabled={onToggleEnabled}
        />
      ) : null}

      {educationSection ? (
        <EducationBlock
          section={educationSection}
          onToggle={onToggle}
          onReorder={onReorder}
          onToggleEnabled={onToggleEnabled}
        />
      ) : null}
    </div>
  );
}
