import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import type { GenerationRunDto, GenerationSectionDto } from '@job-finder/shared';
import { PageHeader } from '../../components/layout/PageHeader';
import { Tile } from '../../components/layout';
import { Button, Field, Select, Spinner } from '../../components/ui';
import { useProfiles } from '../profile/hooks';
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
  useStartGenerationRun,
  useSummaryModel,
  useToggleGenerationItem,
} from './hooks';

const GROUNDING_LEVELS = ['strict', 'moderate', 'aggressive'] as const;

// T020/T032: the two-pane shell — generated items on the left, assembled as
// Summary / Work Experience / Skills blocks (US1), a read-only vacancy card
// and its pre-run controls above. Every run is job-backed now — there is no
// ad-hoc vacancy entry point on this page; `jobId` always comes from picking
// a job in Feed / Job Detail. `/generate` is a 'fit' route (routes.tsx), so
// this owns the viewport and its panes scroll independently rather than the
// page.
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
  const { data: summaryModel } = useSummaryModel();

  const [groundingLevel, setGroundingLevel] = useState<(typeof GROUNDING_LEVELS)[number]>('moderate');
  const [summaryOptionId, setSummaryOptionId] = useState<string | undefined>(undefined);

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

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title="Generate resume"
        description="Pick what goes in, rewrite what needs it, and watch the PDF change as you go."
      />

      <div className="mt-4 flex items-start gap-4">
        <div className="min-w-0 flex-1">
          <VacancySummaryBar jobId={vacancyJobId} />
        </div>
        {!run ? (
          <div className="flex w-60 shrink-0 flex-col gap-2">
            <Field label="Grounding">
              <Select
                aria-label="Grounding level"
                value={groundingLevel}
                onChange={(e) => setGroundingLevel(e.target.value as typeof groundingLevel)}
              >
                {GROUNDING_LEVELS.map((l) => (
                  <option key={l} value={l}>
                    {l}
                  </option>
                ))}
              </Select>
            </Field>
            {summaryModel ? (
              <Field label="Summary writer">
                <Select
                  aria-label="Summary writer"
                  value={summaryOptionId ?? summaryModel.optionId}
                  onChange={(e) => setSummaryOptionId(e.target.value)}
                >
                  {summaryModel.options.map((o) => (
                    <option key={o.id} value={o.id}>
                      {o.label} — {o.cost}
                    </option>
                  ))}
                </Select>
              </Field>
            ) : null}
          </div>
        ) : null}
      </div>

      {startRun.isError ? <p className="mt-2 text-sm text-danger">{(startRun.error as Error).message}</p> : null}

      <div className="mt-4 flex min-h-0 flex-1 flex-col gap-4 lg:flex-row">
        <Tile
          title="Generated resume"
          className="min-h-0 flex-1 lg:basis-2/3"
          scroll
          scrollLabel="Generated resume sections"
          state={tileState}
          emptyMessage={
            <div className="flex flex-col items-center gap-3 text-center">
              <p className="max-w-[44ch] text-sm text-muted">
                No run yet for this vacancy. Generate a resume to review it here as an inspectable list.
              </p>
              <Button disabled={!profileId || startRun.isPending} onClick={handleGenerate}>
                Generate resume
              </Button>
              {startRun.isPending ? <Spinner label="starting generation…" /> : null}
            </div>
          }
          error={error}
        >
          <WorkspaceLeftPane
            run={run}
            onToggle={(itemId, selected) => toggleItem.mutate({ itemId, selected })}
            onEditText={(itemId, text) => toggleItem.mutate({ itemId, text })}
            onReorder={(sectionId, itemIds) => reorderSection.mutate({ sectionId, itemIds })}
            onDropEntries={(itemId, droppedEntries) => toggleItem.mutate({ itemId, droppedEntries })}
            onRerun={() => rerunRun.mutate(undefined)}
            onRerunSections={(sections) => rerunRun.mutate(sections)}
            onRewrite={(itemId) => rewriteItem.mutateAsync(itemId).then((r) => r.variants)}
          />
        </Tile>

        <div className="flex min-h-0 w-full shrink-0 flex-col gap-4 overflow-y-auto lg:w-96">
          {rerunRun.isError ? (
            <p className="text-sm text-danger">{(rerunRun.error as Error).message}</p>
          ) : null}

          <Tile title="PDF preview" className="min-h-0 flex-1">
            <ResumePreviewPane
              run={run}
              profile={profiles?.find((p) => p.id === profileId)}
              onExport={run ? () => exportRun.mutate() : undefined}
              exportPending={exportRun.isPending}
              exportDisabled={!run || run.state === 'running'}
              exportState={exportRun.data ?? run?.export}
              exportError={exportRun.isError ? (exportRun.error as Error).message : undefined}
              warnings={run ? exportWarnings(run) : undefined}
            />
          </Tile>
        </div>
      </div>
    </div>
  );
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
}: {
  run: GenerationRunDto | undefined;
  onToggle: (itemId: string, selected: boolean) => void;
  onEditText: (itemId: string, text: string) => void;
  onReorder: (sectionId: string, itemIds: string[]) => void;
  onDropEntries: (itemId: string, droppedEntries: string[]) => void;
  onRerun: () => void;
  onRerunSections: (sections: string[]) => void;
  onRewrite: (itemId: string) => Promise<string[]>;
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

      {summarySection ? <SummaryBlock section={summarySection} onToggle={onToggle} onEditText={onEditText} /> : null}

      {experienceSections.map((section) => (
        <WorkEntryBlock
          key={section.id}
          section={section}
          onToggle={onToggle}
          onReorder={onReorder}
          onEditText={onEditText}
          onRewrite={onRewrite}
        />
      ))}

      {skillsSection ? (
        <SkillsBlock
          section={skillsSection}
          onToggle={onToggle}
          onReorder={onReorder}
          onEditText={onEditText}
          onDropEntries={onDropEntries}
        />
      ) : null}

      {projectsSection ? (
        <ProjectsBlock section={projectsSection} onToggle={onToggle} onReorder={onReorder} />
      ) : null}
    </div>
  );
}
