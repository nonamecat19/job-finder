import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { FileDown, FileText } from 'lucide-react';
import type { GenerationRunDto } from '@job-finder/shared';
import { PageHeader } from '../../components/layout/PageHeader';
import { DashboardGrid, IconTile, ListRow, Tile } from '../../components/layout';
import { Button, Chip, EmptyState } from '../../components/ui';
import { api } from '../../lib/api';
import { postAgeLabel } from '../../lib/time';
import { useSummaryModel } from '../generate/hooks';
import { useDeleteGenerationRun, useGenerationRuns } from './hooks';

// The library of what the generator has already produced: every recent run,
// newest first, with the config it was made under and its exported PDF. The
// runs come from `GET /v1/generations` — the same list endpoint the workspace
// uses, which returns whole runs, so the config panel needs no extra fetch.
export default function ResumesPage() {
  const { data: runs, isLoading, error, refetch } = useGenerationRuns();
  const [pickedId, setPickedId] = useState<string | undefined>(undefined);

  // The selection is derived, not stored: a picked run that no longer exists
  // (deleted, or not yet fetched) falls back to the newest one, so there is no
  // effect resyncing state against the list.
  const selected = runs?.find((r) => r.id === pickedId) ?? runs?.[0];
  const selectedId = selected?.id;
  const listState = isLoading ? 'loading' : error ? 'error' : runs?.length ? 'ready' : 'empty';

  return (
    <div>
      <PageHeader
        title="Resumes"
        description="Every resume the generator has produced recently, with the config behind it."
      />
      <DashboardGrid>
        <Tile
          span="wide"
          title="Recent runs"
          state={listState}
          error={error}
          onRetry={() => refetch()}
          emptyMessage="No resumes generated yet."
          scroll
          scrollLabel="Recent generation runs"
        >
          <div className="space-y-1">
            {runs?.map((run) => (
              <RunRow
                key={run.id}
                run={run}
                selected={run.id === selectedId}
                onSelect={() => setPickedId(run.id)}
              />
            ))}
          </div>
        </Tile>
        <Tile span="wide" title="Run config">
          {selected ? <RunDetail run={selected} /> : <EmptyState>Pick a run to see its config.</EmptyState>}
        </Tile>
      </DashboardGrid>
    </div>
  );
}

function vacancyLabel(run: GenerationRunDto): string {
  const parts = [run.vacancy.title, run.vacancy.company].filter(Boolean) as string[];
  return parts.length ? parts.join(' — ') : 'Untitled vacancy';
}

const STATE_TONE: Record<string, 'green' | 'red' | 'slate'> = {
  ready: 'green',
  exported: 'green',
  failed: 'red',
  error: 'red',
  blocked: 'red',
};

function StateChip({ state }: { state: string }) {
  return <Chip tone={STATE_TONE[state] ?? 'slate'}>{state}</Chip>;
}

function RunRow({
  run,
  selected,
  onSelect,
}: {
  run: GenerationRunDto;
  selected: boolean;
  onSelect: () => void;
}) {
  const age = postAgeLabel(run.createdAt);
  const meta = [age, run.groundingLevel, run.export.documentId ? 'PDF ready' : null]
    .filter(Boolean)
    .join(' · ');

  return (
    <ListRow
      leading={<IconTile icon={FileText} tint={run.export.documentId ? 'mint' : 'blue'} size="sm" />}
      title={vacancyLabel(run)}
      meta={meta}
      aside={<StateChip state={run.state} />}
      selected={selected}
      onClick={onSelect}
    />
  );
}

function RunDetail({ run }: { run: GenerationRunDto }) {
  const { data: summaryModel } = useSummaryModel();
  const remove = useDeleteGenerationRun();

  const summaryWriter = useMemo(() => {
    if (!run.summaryOptionId) return 'default';
    return summaryModel?.options.find((o) => o.id === run.summaryOptionId)?.label ?? run.summaryOptionId;
  }, [run.summaryOptionId, summaryModel]);

  const shape = run.shapeConfig;
  const selectedItems = run.sections.reduce(
    (total, section) => total + section.items.filter((item) => item.selected).length,
    0,
  );

  const workspaceHref = `/generate?${new URLSearchParams({
    ...(run.jobId ? { jobId: run.jobId } : {}),
    runId: run.id,
  }).toString()}`;

  return (
    <div className="space-y-4">
      <div>
        <div className="text-sm font-semibold text-foreground">{vacancyLabel(run)}</div>
        <div className="mt-1 text-xs text-muted">
          Generated {new Date(run.createdAt).toLocaleString()}
          {run.updatedAt !== run.createdAt ? ` · edited ${new Date(run.updatedAt).toLocaleString()}` : ''}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <StateChip state={run.state} />
        <StateChip state={run.export.status} />
        {run.masterChanged ? <Chip tone="red">master changed</Chip> : null}
        {run.summarySubstituted ? <Chip>summary substituted</Chip> : null}
      </div>

      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-xs sm:grid-cols-3">
        <ConfigField label="Grounding" value={run.groundingLevel} />
        <ConfigField label="Summary writer" value={summaryWriter} />
        <ConfigField label="Selected items" value={selectedItems} />
        <ConfigField label="Target pages" value={shape.targetPages} />
        <ConfigField label="Summary lines" value={shape.summaryLines} />
        <ConfigField
          label="Experience bullets"
          value={`${shape.experienceBulletsMin}–${shape.experienceBulletsMax}`}
        />
        <ConfigField label="Skills" value={shape.skillsEnabled ? `max ${shape.skillsMaxGroups} groups` : 'off'} />
        <ConfigField
          label="Projects"
          value={
            shape.projectsEnabled
              ? `${shape.projectsMin}–${shape.projectsMax}, ${shape.projectBulletsMax} bullets`
              : 'off'
          }
        />
        <ConfigField
          label="Certifications"
          value={
            shape.certificationsEnabled ? `${shape.certificationsMin}–${shape.certificationsMax}` : 'off'
          }
        />
      </dl>

      {run.sections.length ? (
        <div>
          <div className="mb-1.5 text-xs font-semibold uppercase tracking-[var(--tracking-wide)] text-muted">
            Sections
          </div>
          <ul className="space-y-1 text-xs text-muted">
            {run.sections.map((section) => (
              <li key={section.id} className="flex items-center justify-between gap-3">
                <span className="truncate">{section.entryLabel ?? section.kind}</span>
                <span className="shrink-0 font-mono text-faint">
                  {section.items.filter((i) => i.selected).length}/{section.items.length} · {section.state}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <Link to={workspaceHref}>
          <Button>Open in workspace</Button>
        </Link>
        {run.export.documentId ? (
          <a href={api.documents.pdfUrl(run.export.documentId)} target="_blank" rel="noreferrer">
            <Button variant="secondary">
              <FileDown className="h-4 w-4" aria-hidden="true" />
              Open PDF
            </Button>
          </a>
        ) : null}
        <Button
          variant="danger"
          disabled={remove.isPending}
          onClick={() => remove.mutate(run.id)}
        >
          Delete
        </Button>
      </div>
    </div>
  );
}

function ConfigField({ label, value }: { label: string; value: string | number }) {
  return (
    <div>
      <dt className="uppercase tracking-[var(--tracking-wide)] text-faint">{label}</dt>
      <dd className="mt-0.5 font-medium text-foreground">{value}</dd>
    </div>
  );
}
