import { useSearchParams } from 'react-router-dom';
import type { GenerationRunDto, GenerationSectionDto } from '@job-finder/shared';
import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import { EmptyState, ErrorState, LoadingRegion, SkeletonBlock, Spinner, Surface } from '../../components/ui';
import { useProfiles } from '../profile/hooks';
import SkillsBlock from './components/SkillsBlock';
import SummaryBlock from './components/SummaryBlock';
import VacancyPane, { type VacancyPaneInput } from './components/VacancyPane';
import WorkEntryBlock from './components/WorkEntryBlock';
import { useGenerationRun, useReorderGenerationSection, useStartGenerationRun, useToggleGenerationItem } from './hooks';

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
          />
        </Surface>

        <div className="min-h-0 w-full shrink-0 overflow-y-auto lg:w-96">
          <VacancyPane onGenerate={handleGenerate} pending={startRun.isPending} disabled={!profileId} />
          {startRun.isError ? (
            <p className="mt-2 text-sm text-danger">{(startRun.error as Error).message}</p>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function WorkspaceLeftPane({
  runId,
  run,
  isLoading,
  error,
  onToggle,
  onEditText,
  onReorder,
}: {
  runId: string | undefined;
  run: GenerationRunDto | undefined;
  isLoading: boolean;
  error: unknown;
  onToggle: (itemId: string, selected: boolean) => void;
  onEditText: (itemId: string, text: string) => void;
  onReorder: (sectionId: string, itemIds: string[]) => void;
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
  const experienceSections = run.sections
    .filter((s): s is GenerationSectionDto => s.kind === 'experience')
    .sort((a, b) => a.position - b.position);

  return (
    <div className="space-y-4" data-testid="workspace-sections">
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
        <WorkEntryBlock key={section.id} section={section} onToggle={onToggle} onReorder={onReorder} />
      ))}

      {skillsSection ? <SkillsBlock section={skillsSection} onToggle={onToggle} onReorder={onReorder} /> : null}
    </div>
  );
}
