import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Copy, EyeOff, ListTree, Minus, Plus, Scan } from 'lucide-react';
import { Spinner } from '../../../components/ui';
import { cn } from '../../../lib/utils';
import { RESUME_DARK_FILTER, useTheme } from '../../../lib/theme';
import {
  mapItemsToBlocks,
  type BlockRect,
  type MatchableItem,
  type NormRect,
  type PdfPageText,
  type PreviewBlock,
} from './blockMap';
import { buildPieces, loadPdfjs, type PdfDocumentLike, type PdfLoadingTaskLike } from './pdfjs';
import { usePreviewHighlight } from './highlight';
import BlockContextMenu, { type BlockMenuAction } from './BlockContextMenu';

export interface PdfPreviewCanvasProps {
  pdfBytes: Uint8Array;
  /** The run's included items, in document order — what the PDF's text is matched against. */
  items: MatchableItem[];
  /**
   * Drops an item from the run's selection — the same toggle the list's
   * checkbox drives, offered on the block itself. Absent means the preview is
   * read-only and the menu offers no such action.
   */
  onRemoveItem?: (itemId: string) => void;
  /**
   * Reorders a block's items from a drag dropped on the preview itself —
   * dragging one achievement of an experience entry onto another. Absent
   * means the preview is read-only and items aren't draggable. Only
   * multi-item blocks (an experience entry) have anything to reorder; a
   * project, skill group, certification or education entry is already a
   * block of one.
   */
  onReorder?: (sectionId: string, itemIds: string[]) => void;
  /**
   * The rendered page's width/height, reported once it is known, so the pane
   * around this viewer can take the sheet's own proportions (A4, Letter —
   * whatever the template actually produced).
   */
  onPageAspect?: (aspect: number) => void;
  /**
   * How many pages this render came out to, reported once the document is
   * parsed, so the pane can show it against the shape's page target while the
   * user edits — not only after an export is blocked.
   */
  onPageCount?: (count: number) => void;
}

interface RenderedPage {
  pageIndex: number;
  width: number;
  height: number;
}

// PdfPreviewCanvas draws the preview PDF itself, page by page, instead of
// handing it to the browser's PDF plugin in an <iframe>. That costs a little
// machinery (pdf.js, a worker, a render pass per zoom change) and buys the
// thing an iframe can never give: the document's text layer, and with it the
// ability to say which item on the left produced which block of the page —
// so hovering either side lights up and scrolls to the other.
export default function PdfPreviewCanvas({
  pdfBytes,
  items,
  onRemoveItem,
  onReorder,
  onPageAspect,
  onPageCount,
}: PdfPreviewCanvasProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const canvasRefs = useRef<(HTMLCanvasElement | null)[]>([]);
  const docRef = useRef<PdfDocumentLike | null>(null);
  const taskRef = useRef<PdfLoadingTaskLike | null>(null);
  const { hover, setHover } = usePreviewHighlight();
  const { resumeDark } = useTheme();

  const [pages, setPages] = useState<RenderedPage[]>([]);
  const [pageText, setPageText] = useState<PdfPageText[]>([]);
  const [container, setContainer] = useState({ width: 0, height: 0 });
  const [zoom, setZoom] = useState(1);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [menu, setMenu] = useState<{
    key: string;
    itemIds: string[];
    /** Set when the menu was opened on one item rather than on the block. */
    itemId?: string;
    x: number;
    y: number;
  } | null>(null);

  const blocks = useMemo(() => mapItemsToBlocks(pageText, items), [pageText, items]);
  // Which block a hovered item belongs to, taken from the run's own structure
  // rather than from what matched: a bullet whose text the template reworded
  // beyond recognition still belongs to its entry, and hovering it should
  // still light that entry up.
  const blockKeyByItem = useMemo(() => new Map(items.map((i) => [i.id, i.blockKey])), [items]);
  // The PDF side names the block it is on directly; the list side names only
  // an item, and the block it belongs to is looked up.
  const hoveredKey = hover?.blockKey ?? (hover?.itemId ? blockKeyByItem.get(hover.itemId) : undefined);

  // Fit-the-whole-page is the baseline — a resume is judged as a page, and the
  // pane around this viewer is already cut to the sheet's shape, so one sheet
  // lands in it end to end. `zoom` multiplies that; the fit control toggles up
  // to fit-width, which is a ratio of the same baseline.
  const first = pages[0];
  const fitWidth = first && container.width > 0 ? (container.width - PAGE_GUTTER * 2) / first.width : 0;
  const fitPage =
    first && container.height > 0 ? Math.min(fitWidth, (container.height - PAGE_GUTTER * 2) / first.height) : 0;
  const widthZoom = fitPage > 0 ? fitWidth / fitPage : 1;
  const scale = fitPage > 0 ? fitPage * zoom : 0;

  useEffect(() => {
    const el = scrollRef.current;
    if (!el || typeof ResizeObserver === 'undefined') return;
    const measure = () => setContainer({ width: el.clientWidth, height: el.clientHeight });
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  // Load the document and read its geometry + text layer once per render of
  // the pipeline. Cancelled by `stale` rather than by destroying eagerly: the
  // load is async and a newer PDF may well arrive mid-flight.
  useEffect(() => {
    let stale = false;

    (async () => {
      setLoading(true);
      setLoadError(null);
      try {
        const pdfjs = await loadPdfjs();
        // getDocument transfers the buffer to the worker; hand it a copy so
        // the scheduler's cached bytes stay usable for the next render.
        const task = pdfjs.getDocument({ data: pdfBytes.slice() });
        const doc = await task.promise;
        if (stale) {
          void task.destroy();
          return;
        }
        void taskRef.current?.destroy();
        taskRef.current = task;
        docRef.current = doc;

        const geometry: RenderedPage[] = [];
        const text: PdfPageText[] = [];
        for (let i = 1; i <= doc.numPages; i++) {
          const page = await doc.getPage(i);
          if (stale) return;
          const viewport = page.getViewport({ scale: 1 });
          const content = await page.getTextContent();
          if (stale) return;
          geometry.push({ pageIndex: i - 1, width: viewport.width, height: viewport.height });
          text.push({ pageIndex: i - 1, pieces: buildPieces(pdfjs, content.items, viewport) });
        }
        if (stale) return;
        setPages(geometry);
        setPageText(text);
        setLoading(false);
        if (geometry[0]) onPageAspect?.(geometry[0].width / geometry[0].height);
        onPageCount?.(geometry.length);
      } catch (err) {
        if (stale) return;
        setLoadError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      }
    })();

    return () => {
      stale = true;
    };
    // onPageAspect/onPageCount are reports, not inputs: re-running this load because a
    // callback identity changed would re-parse the document for nothing.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pdfBytes]);

  useEffect(() => {
    return () => {
      void taskRef.current?.destroy();
      taskRef.current = null;
      docRef.current = null;
    };
  }, []);

  // Paint every page at the current scale. Render tasks are cancelled on
  // re-entry so a zoom drag doesn't queue a backlog of stale paints.
  useEffect(() => {
    const doc = docRef.current;
    if (!doc || scale <= 0 || pages.length === 0) return;
    const tasks: { cancel: () => void }[] = [];
    let stale = false;

    (async () => {
      const dpr = typeof window === 'undefined' ? 1 : window.devicePixelRatio || 1;
      for (const { pageIndex } of pages) {
        const canvas = canvasRefs.current[pageIndex];
        if (!canvas) continue;
        const page = await doc.getPage(pageIndex + 1);
        if (stale) return;
        const viewport = page.getViewport({ scale: scale * dpr });
        const context = canvas.getContext('2d');
        if (!context) continue;
        canvas.width = Math.floor(viewport.width);
        canvas.height = Math.floor(viewport.height);
        const task = page.render({ canvas, canvasContext: context, viewport });
        tasks.push(task);
        try {
          await task.promise;
        } catch {
          // A cancelled render is the expected outcome of a zoom/resize
          // landing mid-paint, not an error worth surfacing.
        }
        if (stale) return;
      }
    })();

    return () => {
      stale = true;
      for (const task of tasks) task.cancel();
    };
  }, [pages, scale]);

  // Follow the left pane: hovering an item there scrolls its block into view
  // here. The reverse direction is ItemRow's job.
  useEffect(() => {
    if (!hover || hover.source !== 'list') return;
    const block = blocks.find((b) => b.key === hoveredKey);
    const container = scrollRef.current;
    if (!block || !container) return;
    // Scroll to the top of the *page* the block starts on, not to the block
    // itself: aligning the block's own top can leave the reader looking at
    // the bottom half of the page before it and the top half of this one. A
    // clean page boundary reads as "page 2", not as a seam between two pages.
    const pageIndex = block.rects[0]?.pageIndex;
    if (pageIndex === undefined) return;
    const target = container.querySelector<HTMLElement>(`[data-page-index="${pageIndex}"]`);
    target?.scrollIntoView({ block: 'start', behavior: 'smooth' });
  }, [hover, hoveredKey, blocks]);

  // The two levels of the same gesture: the pointer over an entry's heading or
  // the gap between its bullets means the entry; over one of its lines it
  // means that line, and the entry stays lit underneath it.
  const onEnterBlock = useCallback(
    (block: PreviewBlock) => setHover({ blockKey: block.key, source: 'pdf' }),
    [setHover],
  );
  const onEnterItem = useCallback(
    (block: PreviewBlock, itemId: string) => setHover({ itemId, blockKey: block.key, source: 'pdf' }),
    [setHover],
  );

  // Dragging one achievement of an experience entry onto another reorders
  // them, same mutation the left pane's drag handle drives — this is just a
  // second grip on the same list. Native HTML5 drag-and-drop rather than
  // dnd-kit: the item boxes are absolutely positioned per rendered page, not
  // DOM list children a sortable context can measure.
  const dragItemRef = useRef<{ blockKey: string; itemId: string } | null>(null);
  const [dropTargetId, setDropTargetId] = useState<string | null>(null);

  const onItemDragStart = useCallback((block: PreviewBlock, itemId: string) => {
    dragItemRef.current = { blockKey: block.key, itemId };
  }, []);
  const onItemDragOver = useCallback((event: React.DragEvent, block: PreviewBlock, itemId: string) => {
    if (dragItemRef.current?.blockKey !== block.key || dragItemRef.current.itemId === itemId) return;
    event.preventDefault();
    setDropTargetId(itemId);
  }, []);
  const onItemDrop = useCallback(
    (event: React.DragEvent, block: PreviewBlock, targetItemId: string) => {
      event.preventDefault();
      setDropTargetId(null);
      const dragged = dragItemRef.current;
      dragItemRef.current = null;
      if (!dragged || dragged.blockKey !== block.key || dragged.itemId === targetItemId || !onReorder) return;
      const order = block.itemIds.filter((id) => id !== dragged.itemId);
      const at = order.indexOf(targetItemId);
      order.splice(at, 0, dragged.itemId);
      onReorder(block.key, order);
    },
    [onReorder],
  );
  const onItemDragEnd = useCallback(() => {
    dragItemRef.current = null;
    setDropTargetId(null);
  }, []);

  // Right-clicking acts on whatever the pointer is actually on — one bullet
  // inside an entry, or the entry as a whole. The page itself is a rendered
  // document with nothing to offer a context menu, so anywhere outside a block
  // the browser's own menu stands.
  const onContextMenu = useCallback((event: React.MouseEvent, block: PreviewBlock, itemId?: string) => {
    event.preventDefault();
    // The item overlay sits inside the block's, so without this the block
    // would open its own menu right after.
    event.stopPropagation();
    setMenu({ key: block.key, itemIds: block.itemIds, itemId, x: event.clientX, y: event.clientY });
  }, []);

  const menuActions = useMemo((): BlockMenuAction[] => {
    if (!menu) return [];
    const textOf = (id: string) => items.find((i) => i.id === id)?.text;
    const count = menu.itemIds.length;
    const onItem = menu.itemId !== undefined;
    const targetIds = onItem ? [menu.itemId!] : menu.itemIds;
    const targetText = targetIds.map(textOf).filter((t): t is string => !!t);

    const actions: BlockMenuAction[] = [
      {
        key: 'reveal',
        label: 'Show in list',
        icon: <ListTree className="h-3.5 w-3.5" />,
        // The list follows a 'pdf'-sourced hover (ItemRow), which is exactly
        // "scroll this item into view over there".
        onSelect: () => setHover({ itemId: targetIds[0], blockKey: menu.key, source: 'pdf' }),
      },
    ];
    if (targetText.length > 0) {
      actions.push({
        key: 'copy',
        label: onItem ? 'Copy text' : count > 1 ? 'Copy this block' : 'Copy text',
        icon: <Copy className="h-3.5 w-3.5" />,
        onSelect: () => void navigator.clipboard?.writeText(targetText.join('\n')),
      });
    }
    if (onRemoveItem) {
      actions.push({
        key: 'remove',
        label: 'Remove from resume',
        icon: <EyeOff className="h-3.5 w-3.5" />,
        danger: true,
        onSelect: () => targetIds.forEach((id) => onRemoveItem(id)),
      });
      // Offered alongside, not instead: right-clicking a bullet is the obvious
      // way to reach for its entry too, and the count says what that costs.
      if (onItem && count > 1) {
        actions.push({
          key: 'remove-block',
          label: `Remove all ${count} in this block`,
          icon: <EyeOff className="h-3.5 w-3.5" />,
          danger: true,
          onSelect: () => menu.itemIds.forEach((id) => onRemoveItem(id)),
        });
      }
    }
    return actions;
  }, [menu, items, onRemoveItem, setHover]);

  if (loadError) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-1 p-8 text-center">
        <p className="text-sm font-medium text-danger">This preview couldn't be drawn.</p>
        <p className="text-xs text-faint">{loadError}</p>
      </div>
    );
  }

  return (
    <div className="relative flex h-full min-h-0 flex-col">
      <div
        ref={scrollRef}
        data-testid="resume-preview"
        // Backstop: whatever the overlays inside did or failed to do, a pointer
        // that has left the preview entirely is not hovering anything in it.
        onMouseLeave={() => {
          if (hover?.source === 'pdf') setHover(null);
        }}
        className="min-h-0 flex-1 overflow-auto bg-surface-secondary"
      >
        {loading && pages.length === 0 ? (
          <div className="flex h-full items-center justify-center p-8">
            <Spinner label="rendering your resume…" />
          </div>
        ) : null}

        <div className="flex flex-col items-center gap-4" style={{ padding: PAGE_GUTTER }}>
          {pages.map((page) => (
            <div
              key={page.pageIndex}
              data-page-index={page.pageIndex}
              className={cn('relative shadow-overlay', resumeDark ? 'bg-paper-dark' : 'bg-paper')}
              style={{ width: page.width * scale, height: page.height * scale }}
            >
              <canvas
                ref={(el) => {
                  canvasRefs.current[page.pageIndex] = el;
                }}
                data-testid="resume-preview-page"
                data-dark={resumeDark ? 'true' : undefined}
                // A dark sheet is the page's luminance flipped — which turns
                // its white stock pure black. Screening the flipped canvas
                // over the sheet colour keeps the light text but lets the
                // design system's dark surface show through underneath,
                // instead of a black rectangle nothing else in the UI uses.
                className={cn('block h-full w-full', resumeDark && 'mix-blend-screen')}
                style={resumeDark ? { filter: RESUME_DARK_FILTER } : undefined}
              />
              {/* Two nested levels of hover target. The outer one is the block
                  — a whole experience entry, a project, the skills list — and
                  inside it sits one target per item, so the pointer selects
                  the entry from its heading or the space between its bullets,
                  and the individual bullet from the bullet itself. Both are
                  single boxes rather than one rectangle per line: a per-line
                  outline reads as underlined text, and its inter-line gaps
                  make the hover flicker as the pointer crosses them. */}
              {blocks.map((block) => {
                const box = unionOnPage(block.rects, page.pageIndex);
                if (!box) return null;
                const blockLit = hoveredKey === block.key || menu?.key === block.key;
                // A block of one — a project, a skill group, the summary — has
                // no inside to point at, so it is the item: one target, one
                // ring, at item strength.
                const only = block.items.length === 1 ? block.items[0].itemId : null;
                const onlyLit = !!only && (hover?.itemId === only || menu?.itemId === only);
                return (
                  <span
                    key={block.key}
                    data-block-id={block.key}
                    data-testid="preview-block"
                    aria-hidden="true"
                    onMouseEnter={() => (only ? onEnterItem(block, only) : onEnterBlock(block))}
                    onMouseLeave={() => setHover(null)}
                    onContextMenu={(e) => onContextMenu(e, block, only ?? undefined)}
                    className={cn(
                      'absolute rounded-[5px] transition-colors duration-[120ms]',
                      // An open menu keeps its block lit even though the
                      // pointer has left it for the menu itself.
                      only
                        ? onlyLit
                          ? 'bg-accent/25 ring-1 ring-accent/60'
                          : 'bg-transparent hover:bg-accent/10'
                        : blockLit
                          ? 'bg-accent/10 ring-1 ring-accent/30'
                          : 'bg-transparent',
                    )}
                    style={boxStyle(box, BLOCK_BLEED)}
                  >
                    {/* The block's own coordinate system, inset back out of
                        its bleed: item boxes are a fraction of the block's
                        box, and must stay strictly inside the element that
                        owns the hover — an item that stuck out past its
                        block's edge could be left while the block was already
                        left, and the hover would never be cleared. */}
                    <span className="pointer-events-none absolute" style={insetStyle(BLOCK_BLEED)}>
                    {(only ? [] : block.items).map((entry) => {
                      const itemBox = unionOnPage(entry.rects, page.pageIndex);
                      if (!itemBox) return null;
                      const itemLit = hover?.itemId === entry.itemId || menu?.itemId === entry.itemId;
                      const dropLit = dropTargetId === entry.itemId;
                      return (
                        <span
                          key={entry.itemId}
                          data-item-id={entry.itemId}
                          data-testid="preview-item"
                          draggable={!!onReorder}
                          onDragStart={onReorder ? () => onItemDragStart(block, entry.itemId) : undefined}
                          onDragOver={onReorder ? (e) => onItemDragOver(e, block, entry.itemId) : undefined}
                          onDrop={onReorder ? (e) => onItemDrop(e, block, entry.itemId) : undefined}
                          onDragEnd={onReorder ? onItemDragEnd : undefined}
                          onMouseEnter={(e) => {
                            e.stopPropagation();
                            onEnterItem(block, entry.itemId);
                          }}
                          // Back out to the block, not to nothing: the pointer
                          // is still inside the entry. Leaving the entry too
                          // fires the block's own handler right after.
                          onMouseLeave={(e) => {
                            e.stopPropagation();
                            onEnterBlock(block);
                          }}
                          onContextMenu={(e) => onContextMenu(e, block, entry.itemId)}
                          className={cn(
                            'pointer-events-auto absolute rounded-[4px] transition-colors duration-[120ms]',
                            onReorder && 'cursor-grab active:cursor-grabbing',
                            dropLit
                              ? 'bg-accent/35 ring-2 ring-accent'
                              : itemLit
                                ? 'bg-accent/25 ring-1 ring-accent/60'
                                : 'bg-transparent hover:bg-accent/10',
                          )}
                          style={boxStyle(relativeTo(itemBox, box), ITEM_BLEED)}
                        />
                      );
                    })}
                    </span>
                  </span>
                );
              })}
            </div>
          ))}
        </div>
      </div>

      <div className="pointer-events-auto absolute bottom-3 right-3 flex items-center gap-1 rounded-full border border-border bg-surface/90 px-1.5 py-1 shadow-tile backdrop-blur">
        <ZoomButton label="zoom out" onClick={() => setZoom((z) => clampZoom(z - 0.15))}>
          <Minus className="h-3.5 w-3.5" />
        </ZoomButton>
        <button
          type="button"
          aria-label="fit whole page"
          title={Math.abs(zoom - 1) < 0.01 ? 'fit width' : 'fit whole page'}
          onClick={() => setZoom((z) => (Math.abs(z - 1) < 0.01 ? clampZoom(widthZoom) : 1))}
          className="flex items-center gap-1 rounded-full px-2 py-0.5 font-mono text-[11px] text-muted hover:bg-surface-tertiary"
        >
          <Scan className="h-3.5 w-3.5" /> {Math.round(zoom * 100)}%
        </button>
        <ZoomButton label="zoom in" onClick={() => setZoom((z) => clampZoom(z + 0.15))}>
          <Plus className="h-3.5 w-3.5" />
        </ZoomButton>
      </div>

      {menu ? (
        <BlockContextMenu x={menu.x} y={menu.y} actions={menuActions} onClose={() => setMenu(null)} />
      ) : null}
    </div>
  );
}

// Enough of a margin for the sheet's shadow to read as a sheet, and no more —
// the page is meant to fill the pane it was given.
const PAGE_GUTTER = 8;

// Dark resume previews are an inversion of the painted page rather than a
// re-render: the PDF is a fixed document, so the only way to darken it here is
// to flip its luminance. The hue-rotate puts colours back where they started —
// without it, a blue heading comes out orange. Applied to the canvas alone, so
// the highlight overlays layered above it keep their accent colour.
/** Rectangles on one page, unioned into the single box that covers them. */
function unionOnPage(all: BlockRect[], pageIndex: number): NormRect | null {
  const rects = all.filter((r) => r.pageIndex === pageIndex);
  if (rects.length === 0) return null;
  const x = Math.min(...rects.map((r) => r.x));
  const y = Math.min(...rects.map((r) => r.y));
  const right = Math.max(...rects.map((r) => r.x + r.w));
  const bottom = Math.max(...rects.map((r) => r.y + r.h));
  return { x, y, w: right - x, h: bottom - y };
}

/** Re-expresses `box` as a fraction of `parent`, for a nested absolute layout. */
function relativeTo(box: NormRect, parent: NormRect): NormRect {
  return {
    x: parent.w > 0 ? (box.x - parent.x) / parent.w : 0,
    y: parent.h > 0 ? (box.y - parent.y) / parent.h : 0,
    w: parent.w > 0 ? box.w / parent.w : 1,
    h: parent.h > 0 ? box.h / parent.h : 1,
  };
}

// A hair of slack so a highlight covers its glyphs' ascenders and descenders
// rather than clipping them. The block's is the larger of the two so an item's
// box can never poke out of the block that contains it.
const BLOCK_BLEED = 5;
const ITEM_BLEED = 3;

function insetStyle(inset: number): React.CSSProperties {
  return { left: inset, top: inset, right: inset, bottom: inset };
}

/** `bleed` is the slack, in pixels, added on every side of the box. */
function boxStyle(box: NormRect, bleed: number): React.CSSProperties {
  return {
    left: `calc(${box.x * 100}% - ${bleed}px)`,
    top: `calc(${box.y * 100}% - ${bleed}px)`,
    width: `calc(${box.w * 100}% + ${bleed * 2}px)`,
    height: `calc(${box.h * 100}% + ${bleed * 2}px)`,
  };
}

function clampZoom(zoom: number): number {
  return Math.min(3, Math.max(0.4, Math.round(zoom * 100) / 100));
}

function ZoomButton({ label, onClick, children }: { label: string; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      className="rounded-full p-1 text-muted hover:bg-surface-tertiary hover:text-foreground"
    >
      {children}
    </button>
  );
}

