import { useSearchParams } from 'react-router-dom';
import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import { EmptyState, ErrorState, LoadingRegion, SkeletonBlock, Spinner, Surface } from '../../components/ui';
import { useProfiles } from '../profile/hooks';
import VacancyPane, { type VacancyPaneInput } from './components/VacancyPane';
import { useGenerationRun, useStartGenerationRun } from './hooks';

// T020: the two-pane shell — generated items on the left (a plain rendering
// for Phase 2; US1 replaces this with the addressable SummaryBlock /
// WorkEntryBlock / SkillsBlock components), vacancy and controls on the
// right. `/generate` is a 'fit' route (routes.tsx), so this owns the
// viewport and its panes scroll independently rather than the page.
export default function GenerateWorkspacePage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const runId = searchParams.get('runId') ?? undefined;

  const { data: run, isLoading, error } = useGenerationRun(runId);
  const { data: profiles } = useProfiles();
  const profileId = profiles?.[0]?.id;
  const startRun = useStartGenerationRun();

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
          <WorkspaceLeftPane runId={runId} run={run} isLoading={isLoading} error={error} />
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
}: {
  runId: string | undefined;
  run: ReturnType<typeof useGenerationRun>['data'];
  isLoading: boolean;
  error: unknown;
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
      {run.sections.map((section) => (
        <div key={section.id} className="rounded-md border border-border bg-surface-secondary/60 p-3">
          <div className="mb-2 flex items-center justify-between text-sm font-semibold capitalize">
            <span>{section.kind === 'experience' ? (section.entryLabel ?? section.entryKey) : section.kind}</span>
            {section.state !== 'ready' ? <span className="text-xs font-normal text-muted">{section.state}</span> : null}
          </div>
          {section.items.length === 0 ? (
            <p className="text-xs text-muted">
              {section.kind === 'experience'
                ? 'No bullets in your profile for this role.'
                : 'Nothing here yet.'}
            </p>
          ) : (
            <ul className="space-y-1 text-sm">
              {section.items.map((item) => (
                <li
                  key={item.id}
                  className={item.selected ? undefined : 'text-faint line-through'}
                  data-origin={item.origin}
                >
                  {item.text}
                </li>
              ))}
            </ul>
          )}
        </div>
      ))}
    </div>
  );
}
