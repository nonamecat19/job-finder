import { useSearchParams } from 'react-router-dom';
import type { GenerationRunDto, GenerationSectionDto } from '@job-finder/shared';
import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import { Button, EmptyState, ErrorState, LoadingRegion, SkeletonBlock, Spinner, Surface } from '../../components/ui';
import { useProfiles } from '../profile/hooks';
import ProjectsBlock from './components/ProjectsBlock';
import SkillsBlock from './components/SkillsBlock';
import SummaryBlock from './components/SummaryBlock';
import VacancyPane, { type VacancyPaneInput } from './components/VacancyPane';
import WorkEntryBlock from './components/WorkEntryBlock';
import {
  useExportGenerationRun,
  useGenerationRun,
  useRerunGenerationRun,
  useReorderGenerationSection,
  useStartGenerationRun,
  useToggleGenerationItem,
} from './hooks';

// T020/T032: the two-pane shell — generated items on the left, assembled as
// Summary / Work Experience / Skills blocks (US1), vacancy and controls on
// the right. `/generate` is a 'fit' route (routes.tsx), so this owns the
// viewport and its panes scroll independently rather than the page.
export default function GenerateWorkspacePage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const runId = searchParams.get('runId') ?? undefined;

  const { data: run, isLoading, error } = useGenerationRun(runId);
  const { data: profiles } = useProfiles();
  const profileId = profiles?.[0]?.id;
  const startRun = useStartGenerationRun();
  const toggleItem = useToggleGenerationItem(runId);
  const reorderSection = useReorderGenerationSection(runId);
  const exportRun = useExportGenerationRun(runId);
  const rerunRun = useRerunGenerationRun(runId);

  const handleGenerate = (input: VacancyPaneInput) => {
    if (!profileId) return;
    startRun.mutate(
      {
        profileId,
        vacancy: input.vacancy,
        groundingLevel: input.groundingLevel,
        summaryOptionId: input.summaryOptionId,
      },
      {
        onSuccess: (data) => setSearchParams({ runId: data.runId }),
      },
    );
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title="Generate"
        description="Review your generated resume as an inspectable list while you tune the vacancy."
      />
      <div className="flex min-h-0 flex-1 flex-col gap-3 lg:flex-row">
        <Surface className="min-h-0 max-w-none flex-1 overflow-y-auto lg:basis-2/3">
          <SectionTitle>Generated resume</SectionTitle>
          <WorkspaceLeftPane
            runId={runId}
            run={run}
            isLoading={isLoading}
            error={error}
            onToggle={(itemId, selected) => toggleItem.mutate({ itemId, selected })}
            onEditText={(itemId, text) => toggleItem.mutate({ itemId, text })}
            onReorder={(sectionId, itemIds) => reorderSection.mutate({ sectionId, itemIds })}
            onRerun={() => rerunRun.mutate(undefined)}
          />
        </Surface>

        <div className="min-h-0 w-full shrink-0 overflow-y-auto lg:w-96">
          <VacancyPane
            onGenerate={handleGenerate}
            pending={startRun.isPending}
            disabled={!profileId}
            onExport={run ? () => exportRun.mutate() : undefined}
            exportPending={exportRun.isPending}
            exportDisabled={!run || run.state === 'running'}
            exportState={exportRun.data ?? run?.export}
            exportError={exportRun.isError ? (exportRun.error as Error).message : undefined}
            warnings={run ? exportWarnings(run) : undefined}
            onRerun={run ? (sections) => rerunRun.mutate(sections) : undefined}
            rerunPending={rerunRun.isPending}
            failedSections={run ? failedSections(run) : undefined}
          />
          {startRun.isError ? (
            <p className="mt-2 text-sm text-danger">{(startRun.error as Error).message}</p>
          ) : null}
          {rerunRun.isError ? (
            <p className="mt-2 text-sm text-danger">{(rerunRun.error as Error).message}</p>
          ) : null}
        </div>
      </div>
    </div>
  );
}

// failedSections is T078's per-section-retry input: every `failed` section,
// labelled for the control ("Acme Inc." for an experience block, "Summary" /
// "Skills" for the singleton ones).
function failedSections(run: GenerationRunDto): { id: string; label: string }[] {
  return run.sections
    .filter((s) => s.state === 'failed')
    .map((s) => ({
      id: s.id,
      label:
        s.entryKey ??
        s.entryLabel ??
        (s.kind === 'summary' ? 'Summary' : s.kind === 'skills' ? 'Skills' : s.kind === 'projects' ? 'Projects' : s.kind),
    }));
}

// exportWarnings is T073: what the user should know before they export, said
// before the export rather than after it. Two kinds — a section the user has
// emptied (the server exports it happily; it is their resume, and FR-019 only
// refuses a wholly empty document), and AI-written content they have included,
// which no grounding check ever verified (FR-016).
function exportWarnings(run: GenerationRunDto): string[] {
  const warnings: string[] = [];
  const selected = (section: GenerationSectionDto | undefined) =>
    section?.items.filter((i) => i.selected && !i.unavailable) ?? [];

  if (selected(run.sections.find((s) => s.kind === 'summary')).length === 0) {
    warnings.push('This resume has no summary — nothing is included in the summary section.');
  }
  if (selected(run.sections.find((s) => s.kind === 'skills')).length === 0) {
    warnings.push('This resume has no skills — every skill group is switched off.');
  }
  // Only warned about when the profile *has* projects: a resume without a
  // projects section is a normal resume, not an emptied one.
  const projectsSection = run.sections.find((s) => s.kind === 'projects');
  if (projectsSection && projectsSection.items.length > 0 && selected(projectsSection).length === 0) {
    warnings.push('This resume has no projects — every project is switched off.');
  }

  // The summary section is excluded: it is written by the run itself and
  // grounded by its own stage, and warning about it on every single export
  // would train the user to ignore the warning that matters — an unverified
  // *suggestion* they chose to include (FR-016).
  const aiIncluded = run.sections
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

function WorkspaceLeftPane({
  runId,
  run,
  isLoading,
  error,
  onToggle,
  onEditText,
  onReorder,
  onRerun,
}: {
  runId: string | undefined;
  run: GenerationRunDto | undefined;
  isLoading: boolean;
  error: unknown;
  onToggle: (itemId: string, selected: boolean) => void;
  onEditText: (itemId: string, text: string) => void;
  onReorder: (sectionId: string, itemIds: string[]) => void;
  onRerun: () => void;
}) {
  if (!runId) {
    return <EmptyState>Fill in a vacancy on the right and generate a resume to see it here.</EmptyState>;
  }
  if (isLoading) {
    return (
      <LoadingRegion label="loading workspace…" className="space-y-3">
        <SkeletonBlock className="h-24 w-full" />
        <SkeletonBlock className="h-24 w-full" />
      </LoadingRegion>
    );
  }
  if (error) {
    return <ErrorState error={error} />;
  }
  if (!run) {
    return null;
  }
  if (run.state === 'running') {
    return (
      <div className="flex items-center gap-2 py-8 text-sm text-muted" role="status">
        <Spinner label="generating your resume…" />
      </div>
    );
  }

  const summarySection = run.sections.find((s) => s.kind === 'summary');
  const skillsSection = run.sections.find((s) => s.kind === 'skills');
  const projectsSection = run.sections.find((s) => s.kind === 'projects');
  const experienceSections = run.sections
    .filter((s): s is GenerationSectionDto => s.kind === 'experience')
    .sort((a, b) => a.position - b.position);

  return (
    <div className="space-y-4" data-testid="workspace-sections">
      {run.masterChanged ? (
        <div
          className="flex items-center justify-between gap-2 rounded-md border border-warning/30 bg-warning-soft p-2 text-xs text-warning"
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
        <p className="rounded-md border border-warning/30 bg-warning-soft p-2 text-xs text-warning">
          Some sections did not finish generating. Completed sections are shown below.
        </p>
      ) : null}
      {run.state === 'failed' ? (
        <p className="rounded-md border border-danger/30 bg-danger-soft p-2 text-xs text-danger">
          This generation run failed.
        </p>
      ) : null}

      {summarySection ? <SummaryBlock section={summarySection} onToggle={onToggle} onEditText={onEditText} /> : null}

      {experienceSections.map((section) => (
        <WorkEntryBlock
          key={section.id}
          section={section}
          onToggle={onToggle}
          onReorder={onReorder}
          onEditText={onEditText}
        />
      ))}

      {skillsSection ? (
        <SkillsBlock section={skillsSection} onToggle={onToggle} onReorder={onReorder} onEditText={onEditText} />
      ) : null}

      {projectsSection ? (
        <ProjectsBlock section={projectsSection} onToggle={onToggle} onReorder={onReorder} />
      ) : null}
    </div>
  );
}
