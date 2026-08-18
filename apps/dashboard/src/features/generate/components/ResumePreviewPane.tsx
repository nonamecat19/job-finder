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
  /** Drops an item from the selection — the preview's block context menu. */
  onRemoveItem?: (itemId: string) => void;
  /** Reorders a section's items from a drag dropped on the preview itself. */
  onReorder?: (sectionId: string, itemIds: string[]) => void;
  onExport?: () => void;
  exportPending?: boolean;
  exportDisabled?: boolean;
  exportState?: GenerationExportDto;
  exportError?: string;
  warnings?: string[];
}

// ResumePreviewPane renders the run's current selection through the same
// rendering engine the export/download PDF comes from (rendercv-go's Typst
// pipeline), running client-side via WebAssembly — not an approximated
// re-styling of the section text (046-real-resume-preview, replacing the
// prior styled-div preview).
export default function ResumePreviewPane({
  run,
  // profile stays part of the public prop contract (GenerateWorkspacePage.tsx
  // passes it) but the real PDF render carries its own header/contact info —
  // there is nothing left for this component to derive from it directly.
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
  // How many pages the current selection actually renders as, reported by the
  // viewer. Kept next to the export control so the page budget is visible
  // while editing, not only once an export comes back blocked.
  // Tagged with the run it was measured on so switching runs clears it during
  // render rather than through a reset effect.
  const [rendered, setRendered] = useState<{ runId: string; count: number } | null>(null);
  const pageCount = rendered && rendered.runId === run?.id ? rendered.count : null;
  const reportPageCount = useCallback(
    (count: number) => {
      if (run) setRendered({ runId: run.id, count });
    },
    [run],
  );
  // What the preview's text is matched against to build its hoverable blocks
  // (preview/blockMap.ts) — recomputed only when the selection itself changes.
  const items = useMemo(() => (run ? matchableItems(run.sections) : []), [run]);

  if (!run) {
    return <EmptyState>Generate a resume to preview it here.</EmptyState>;
  }
  if (run.state === 'running') {
    return <p className="py-8 text-center text-sm text-muted">rendering your resume…</p>;
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      {/* The surface takes whatever the tile gives it; the sheet inside is
          scaled to that width by the viewer (PdfPreviewCanvas). */}
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

// PageBudget is the live counterpart to OverflowReport: the rendered page
// count against the shape's target, shown on every render rather than only
// when an export is refused. Warning-toned once it is over — the export will
// be blocked, and knowing that before pressing the button is the point.
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
      // Deliberately not keyed on the render: the viewer keeps its zoom and
      // scroll position across edits, which is the whole point of a live
      // preview — a re-render swaps the bytes underneath it, not the pane.
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

/**
 * Drives the WASM preview pipeline off the run's id and current selection
 * content, debounced and coalesced (PreviewScheduler, spec FR-010) so rapid
 * edits settle on one render of the latest state instead of one per edit.
 * A fresh scheduler (and its blob: URL) is torn down whenever the run
 * changes or the component unmounts.
 */
function useResumePreview(run: GenerationRunDto | undefined): PreviewState {
  const [state, setState] = useState<PreviewState>({ status: 'idle' });
  const schedulerRef = useRef<PreviewScheduler | null>(null);

  // The signature that should trigger a re-render: the run's selection
  // content, not just its id — TanStack Query's optimistic toggle/edit
  // mutations (hooks.ts) update `run.sections` in the cache immediately,
  // well before the server round trip that would otherwise bump
  // `run.updatedAt`, and FR-003 wants the preview to follow that immediately
  // too (subject to the scheduler's own debounce).
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
    // Nothing to render: the component itself returns an EmptyState /
    // "rendering…" message before PreviewSurface is ever mounted for these
    // two cases, so there is no state for this effect to synchronize.
    if (!run || run.state === 'running') return;
    schedulerRef.current?.schedule(run.id);
    // contentSignature (derived from run.sections) is the real trigger;
    // run.id/run.state are read above but a run.id change already remounts
    // the scheduler in the effect above.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [run?.id, run?.state, contentSignature]);

  return state;
}
