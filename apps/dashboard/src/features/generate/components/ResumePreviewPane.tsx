import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FileDown } from 'lucide-react';
import type { GenerationExportDto, GenerationRunDto, ProfileDto } from '@job-finder/shared';
import { Button, EmptyState, Spinner } from '../../../components/ui';
import { api } from '../../../lib/api';
import { cn } from '../../../lib/utils';
import PdfPreviewCanvas from '../preview/PdfPreviewCanvas';
import { matchableItems, type MatchableItem } from '../preview/blockMap';
import { PreviewScheduler, type PreviewState } from '../wasm/previewPipeline';
import OverflowReport from './OverflowReport';

export interface ResumePreviewPaneProps {
  run: GenerationRunDto | undefined;
  profile: ProfileDto | undefined;

  onRemoveItem?: (itemId: string) => void;

  onReorder?: (sectionId: string, itemIds: string[]) => void;
  onExport?: () => void;
  exportPending?: boolean;
  exportDisabled?: boolean;
  exportState?: GenerationExportDto;
  exportError?: string;
  warnings?: string[];
}

export default function ResumePreviewPane({
  run,

  profile: _profile,
  onRemoveItem,
  onReorder,
  onExport,
  exportPending,
  exportDisabled,
  exportState,
  exportError,
  warnings,
}: ResumePreviewPaneProps) {
  const preview = useResumePreview(run);

  const [rendered, setRendered] = useState<{ runId: string; count: number } | null>(null);
  const pageCount = rendered && rendered.runId === run?.id ? rendered.count : null;
  const reportPageCount = useCallback(
    (count: number) => {
      if (run) setRendered({ runId: run.id, count });
    },
    [run],
  );

  const items = useMemo(() => (run ? matchableItems(run.sections) : []), [run]);

  if (!run) {
    return <EmptyState>Generate a resume to preview it here.</EmptyState>;
  }
  if (run.state === 'running') {
    return <p className="py-8 text-center text-sm text-muted">rendering your resume…</p>;
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      {}
      <div className="min-h-0 flex-1 overflow-hidden rounded-xl border border-border bg-surface-secondary">
        <PreviewSurface
          state={preview}
          items={items}
          onRemoveItem={onRemoveItem}
          onReorder={onReorder}
          onPageCount={reportPageCount}
        />
      </div>

      <PageBudget pages={pageCount} target={run.shapeConfig.targetPages} />

      {warnings?.map((warning) => (
        <p key={warning} className="rounded-lg bg-warning-soft px-2.5 py-2 text-xs text-warning">
          {warning}
        </p>
      ))}

      {exportState?.status === 'blocked' && exportState.report ? (
        <OverflowReport report={exportState.report} />
      ) : run.export.status === 'blocked' && run.export.report ? (
        <OverflowReport report={run.export.report} />
      ) : null}

      {onExport ? (
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs text-faint">
            {exportState?.status === 'blocked'
              ? 'Export is blocked until this fits your page target.'
              : exportState?.status === 'exported'
                ? 'Exported — it contains exactly the items you included.'
                : 'Live preview — reflects your current selection.'}
          </span>
          <div className="flex shrink-0 items-center gap-2">
            {exportState?.status === 'exported' && exportState.documentId ? (
              <a href={api.documents.pdfUrl(exportState.documentId)} download>
                <Button variant="secondary">
                  Download PDF <FileDown className="h-4 w-4" aria-hidden="true" />
                </Button>
              </a>
            ) : null}
            <Button disabled={exportDisabled || exportPending} onClick={onExport}>
              Export PDF
            </Button>
            {exportPending ? <Spinner label="rendering your resume…" /> : null}
          </div>
        </div>
      ) : null}
      {exportError ? <p className="text-center text-xs text-danger">{exportError}</p> : null}
    </div>
  );
}

function PageBudget({ pages, target }: { pages: number | null; target: number }) {
  if (pages === null) return null;
  const over = pages > target;
  return (
    <p
      className={cn('text-xs', over ? 'text-warning' : 'text-faint')}
      data-testid="page-budget"
    >
      {pages} page{pages === 1 ? '' : 's'} of {target} target
      {over ? ' — over budget' : null}
    </p>
  );
}

function PreviewSurface({
  state,
  items,
  onRemoveItem,
  onReorder,
  onPageCount,
}: {
  state: PreviewState;
  items: MatchableItem[];
  onRemoveItem?: (itemId: string) => void;
  onReorder?: (sectionId: string, itemIds: string[]) => void;
  onPageCount?: (count: number) => void;
}) {
  switch (state.status) {
    case 'idle':
      return null;
    case 'loading':
      return (
        <div className="flex h-full items-center justify-center p-8">
          <Spinner label="rendering your resume…" />
        </div>
      );
    case 'error':
      return (
        <div className="flex h-full flex-col items-center justify-center gap-1 p-8 text-center">
          <p className="text-sm font-medium text-danger">Preview isn't available right now.</p>
          <p className="text-xs text-faint">{state.message}</p>
          <p className="text-xs text-faint">Export still works — use Export PDF below.</p>
        </div>
      );
    case 'ready':

      return (
        <PdfPreviewCanvas
          pdfBytes={state.pdfBytes}
          items={items}
          onRemoveItem={onRemoveItem}
          onReorder={onReorder}
          onPageCount={onPageCount}
        />
      );
  }
}

function useResumePreview(run: GenerationRunDto | undefined): PreviewState {
  const [state, setState] = useState<PreviewState>({ status: 'idle' });
  const schedulerRef = useRef<PreviewScheduler | null>(null);

  const contentSignature = useMemo(() => (run ? JSON.stringify(run.sections) : ''), [run]);

  useEffect(() => {
    if (!schedulerRef.current) {
      schedulerRef.current = new PreviewScheduler(setState);
    }
    return () => {
      schedulerRef.current?.dispose();
      schedulerRef.current = null;
    };
  }, [run?.id]);

  useEffect(() => {

    if (!run || run.state === 'running') return;
    schedulerRef.current?.schedule(run.id);

    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [run?.id, run?.state, contentSignature]);

  return state;
}
