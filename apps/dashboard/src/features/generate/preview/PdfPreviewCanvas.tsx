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

  items: MatchableItem[];

  onRemoveItem?: (itemId: string) => void;

  onReorder?: (sectionId: string, itemIds: string[]) => void;

  onPageAspect?: (aspect: number) => void;

  onPageCount?: (count: number) => void;
}

interface RenderedPage {
  pageIndex: number;
  width: number;
  height: number;
}

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

    itemId?: string;
    x: number;
    y: number;
  } | null>(null);

  const blocks = useMemo(() => mapItemsToBlocks(pageText, items), [pageText, items]);

  const blockKeyByItem = useMemo(() => new Map(items.map((i) => [i.id, i.blockKey])), [items]);

  const hoveredKey = hover?.blockKey ?? (hover?.itemId ? blockKeyByItem.get(hover.itemId) : undefined);

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

  useEffect(() => {
    let stale = false;

    (async () => {
      setLoading(true);
      setLoadError(null);
      try {
        const pdfjs = await loadPdfjs();

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

    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pdfBytes]);

  useEffect(() => {
    return () => {
      void taskRef.current?.destroy();
      taskRef.current = null;
      docRef.current = null;
    };
  }, []);

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
          ;
        }
        if (stale) return;
      }
    })();

    return () => {
      stale = true;
      for (const task of tasks) task.cancel();
    };
  }, [pages, scale]);

  useEffect(() => {
    if (!hover || hover.source !== 'list') return;
    const block = blocks.find((b) => b.key === hoveredKey);
    const container = scrollRef.current;
    if (!block || !container) return;

    const pageIndex = block.rects[0]?.pageIndex;
    if (pageIndex === undefined) return;
    const target = container.querySelector<HTMLElement>(`[data-page-index="${pageIndex}"]`);
    target?.scrollIntoView({ block: 'start', behavior: 'smooth' });
  }, [hover, hoveredKey, blocks]);

  const onEnterBlock = useCallback(
    (block: PreviewBlock) => setHover({ blockKey: block.key, source: 'pdf' }),
    [setHover],
  );
  const onEnterItem = useCallback(
    (block: PreviewBlock, itemId: string) => setHover({ itemId, blockKey: block.key, source: 'pdf' }),
    [setHover],
  );

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

  const onContextMenu = useCallback((event: React.MouseEvent, block: PreviewBlock, itemId?: string) => {
    event.preventDefault();

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

                className={cn('block h-full w-full', resumeDark && 'mix-blend-screen')}
                style={resumeDark ? { filter: RESUME_DARK_FILTER } : undefined}
              />
              {}
              {blocks.map((block) => {
                const box = unionOnPage(block.rects, page.pageIndex);
                if (!box) return null;
                const blockLit = hoveredKey === block.key || menu?.key === block.key;

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
                    {}
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

const PAGE_GUTTER = 8;

function unionOnPage(all: BlockRect[], pageIndex: number): NormRect | null {
  const rects = all.filter((r) => r.pageIndex === pageIndex);
  if (rects.length === 0) return null;
  const x = Math.min(...rects.map((r) => r.x));
  const y = Math.min(...rects.map((r) => r.y));
  const right = Math.max(...rects.map((r) => r.x + r.w));
  const bottom = Math.max(...rects.map((r) => r.y + r.h));
  return { x, y, w: right - x, h: bottom - y };
}

function relativeTo(box: NormRect, parent: NormRect): NormRect {
  return {
    x: parent.w > 0 ? (box.x - parent.x) / parent.w : 0,
    y: parent.h > 0 ? (box.y - parent.y) / parent.h : 0,
    w: parent.w > 0 ? box.w / parent.w : 1,
    h: parent.h > 0 ? box.h / parent.h : 1,
  };
}

const BLOCK_BLEED = 5;
const ITEM_BLEED = 3;

function insetStyle(inset: number): React.CSSProperties {
  return { left: inset, top: inset, right: inset, bottom: inset };
}

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

