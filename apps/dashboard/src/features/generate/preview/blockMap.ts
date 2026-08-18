

export interface NormRect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface PdfTextPiece {
  str: string;
  rect: NormRect;
}

export interface PdfPageText {
  pageIndex: number;
  pieces: PdfTextPiece[];
}

export interface BlockRect extends NormRect {
  pageIndex: number;
}

export interface PreviewBlock {
  key: string;

  itemIds: string[];

  rects: BlockRect[];

  items: PreviewItemRects[];
}

export interface PreviewItemRects {
  itemId: string;
  rects: BlockRect[];
}

export interface MatchableItem {
  id: string;
  text: string;

  blockKey: string;

  parts?: string[];
}

export function normalizeForMatch(text: string): string {
  return text.toLowerCase().replace(/[^a-z0-9]+/g, '');
}

const MIN_MATCH_CHARS = 6;

const PREFIX_FALLBACKS = [60, 40, 30, 20, 12];

const LINE_TOLERANCE = 0.006;

interface IndexedPage {
  pageIndex: number;

  norm: string;

  owner: number[];
  pieces: PdfTextPiece[];
}

function indexPage(page: PdfPageText): IndexedPage {
  let norm = '';
  const owner: number[] = [];
  page.pieces.forEach((piece, pieceIndex) => {
    const normalized = normalizeForMatch(piece.str);
    norm += normalized;
    for (let i = 0; i < normalized.length; i++) owner.push(pieceIndex);
  });
  return { pageIndex: page.pageIndex, norm, owner, pieces: page.pieces };
}

export function mapItemsToBlocks(pages: PdfPageText[], items: MatchableItem[]): PreviewBlock[] {
  const indexed = pages.map(indexPage);
  const blocks: PreviewBlock[] = [];
  const byKey = new Map<string, PreviewBlock>();
  let cursorPage = 0;
  let cursorOffset = 0;

  for (const item of items) {
    const needle = normalizeForMatch(item.text);
    if (needle.length < MIN_MATCH_CHARS) continue;

    const hit = findFrom(indexed, needle, cursorPage, cursorOffset);
    if (!hit) continue;

    const page = indexed[hit.pageIndexInList];
    const end = item.parts?.length ? consumeParts(page, hit.start + hit.length, item.parts) : hit.start + hit.length;
    const rects = rectsForRange(page, hit.start, end - hit.start);
    if (rects.length === 0) continue;

    cursorPage = hit.pageIndexInList;
    cursorOffset = end;

    let block = byKey.get(item.blockKey);
    if (!block) {
      block = { key: item.blockKey, itemIds: [], rects: [], items: [] };
      byKey.set(item.blockKey, block);
      blocks.push(block);
    }
    block.itemIds.push(item.id);
    block.items.push({ itemId: item.id, rects });
    mergeRects(block.rects, rects);
  }

  return expandToLines(blocks, indexed);
}

function consumeParts(page: IndexedPage, from: number, parts: string[]): number {
  const remaining = parts.map(normalizeForMatch).filter((p) => p.length > 0);
  let pos = from;
  for (;;) {
    const index = remaining.findIndex((part) => page.norm.startsWith(part, pos));
    if (index === -1) return pos;
    pos += remaining[index].length;
    remaining.splice(index, 1);
  }
}

const HEADING_GAP = 0.045;

const INDENT_EPSILON = 0.004;

const CONTINUATION_INDENT = 0.09;

const CONTINUATION_GAP = 0.02;

function expandToLines(blocks: PreviewBlock[], pages: IndexedPage[]): PreviewBlock[] {
  const lines = new Map(pages.map((page) => [page.pageIndex, lineBoxesOf(page)]));
  const owned = new Set<string>();
  const lineKey = (rect: BlockRect) => `${rect.pageIndex}:${rect.y.toFixed(4)}`;

  const lineFor = (rect: BlockRect): BlockRect | null =>
    lines.get(rect.pageIndex)?.find((l) => Math.abs(l.y - rect.y) <= LINE_TOLERANCE) ?? null;

  for (const block of blocks) {
    const grown: BlockRect[] = [];
    for (const rect of block.rects) {
      const line = lineFor(rect);
      mergeRects(grown, [line ?? rect]);
      if (line) owned.add(lineKey(line));
    }
    block.rects = grown;
  }

  for (const block of blocks) {
    const top = block.rects.reduce<BlockRect | null>((a, b) => (!a || b.y < a.y ? b : a), null);
    if (!top) continue;
    const heading = (lines.get(top.pageIndex) ?? [])
      .filter(
        (l) =>
          l.y < top.y &&
          top.y - l.y <= HEADING_GAP &&
          !owned.has(lineKey(l)) &&

          l.x <= top.x + INDENT_EPSILON,
      )
      .sort((a, b) => b.y - a.y)[0];
    if (!heading) continue;
    owned.add(lineKey(heading));
    mergeRects(block.rects, [heading]);
  }

  for (const block of blocks) {
    const bottom = block.rects.reduce<BlockRect | null>((a, b) => (!a || b.y > a.y ? b : a), null);
    const left = block.rects.reduce((min, r) => Math.min(min, r.x), Infinity);
    if (!bottom) continue;
    let y = bottom.y + bottom.h;
    for (const line of lines.get(bottom.pageIndex) ?? []) {
      if (line.y <= bottom.y || owned.has(lineKey(line))) continue;
      const indent = line.x - left;
      if (line.y - y > CONTINUATION_GAP || indent <= INDENT_EPSILON || indent > CONTINUATION_INDENT) break;
      owned.add(lineKey(line));
      mergeRects(block.rects, [line]);
      y = line.y + line.h;
    }
  }

  for (const block of blocks) {
    block.rects.sort((a, b) => a.pageIndex - b.pageIndex || a.y - b.y);
  }

  return blocks;
}

function lineBoxesOf(page: IndexedPage): BlockRect[] {
  const lines: BlockRect[] = [];
  for (const piece of page.pieces) {
    if (!piece.str.trim()) continue;
    mergeRects(lines, [{ ...piece.rect, pageIndex: page.pageIndex }]);
  }
  return lines.sort((a, b) => a.y - b.y);
}

function mergeRects(into: BlockRect[], incoming: BlockRect[]): void {
  for (const rect of incoming) {
    const line = into.find((l) => l.pageIndex === rect.pageIndex && Math.abs(l.y - rect.y) <= LINE_TOLERANCE);
    if (!line) {
      into.push({ ...rect });
      continue;
    }
    const right = Math.max(line.x + line.w, rect.x + rect.w);
    const bottom = Math.max(line.y + line.h, rect.y + rect.h);
    line.x = Math.min(line.x, rect.x);
    line.y = Math.min(line.y, rect.y);
    line.w = right - line.x;
    line.h = bottom - line.y;
  }
}

interface Hit {
  pageIndexInList: number;
  start: number;
  length: number;
}

function findFrom(pages: IndexedPage[], needle: string, cursorPage: number, cursorOffset: number): Hit | null {
  const candidates = [needle, ...PREFIX_FALLBACKS.map((n) => needle.slice(0, n))].filter(
    (c, i, all) => c.length >= MIN_MATCH_CHARS && all.indexOf(c) === i,
  );

  for (const candidate of candidates) {
    for (let step = 0; step < pages.length; step++) {
      const pageIndexInList = (cursorPage + step) % pages.length;
      const from = step === 0 ? cursorOffset : 0;
      const start = pages[pageIndexInList].norm.indexOf(candidate, from);
      if (start !== -1) return { pageIndexInList, start, length: candidate.length };
    }

    const start = pages[cursorPage]?.norm.indexOf(candidate, 0) ?? -1;
    if (start !== -1) return { pageIndexInList: cursorPage, start, length: candidate.length };
  }
  return null;
}

function rectsForRange(page: IndexedPage, start: number, length: number): BlockRect[] {
  const pieceIndices = new Set<number>();
  for (let i = start; i < start + length && i < page.owner.length; i++) pieceIndices.add(page.owner[i]);

  const lines: BlockRect[] = [];
  for (const pieceIndex of [...pieceIndices].sort((a, b) => a - b)) {
    const { rect } = page.pieces[pieceIndex];
    const line = lines.find((l) => Math.abs(l.y - rect.y) <= LINE_TOLERANCE);
    if (!line) {
      lines.push({ ...rect, pageIndex: page.pageIndex });
      continue;
    }
    const right = Math.max(line.x + line.w, rect.x + rect.w);
    const bottom = Math.max(line.y + line.h, rect.y + rect.h);
    line.x = Math.min(line.x, rect.x);
    line.y = Math.min(line.y, rect.y);
    line.w = right - line.x;
    line.h = bottom - line.y;
  }
  return lines;
}

export interface MatchableSection {
  id: string;
  kind: string;
  entryLabel?: string;
  position: number;
  enabled?: boolean;
  items: {
    id: string;
    text: string;
    selected: boolean;
    unavailable?: boolean;
    skillEntries?: { text: string; selected: boolean }[];
  }[];
}

export function matchableItems(sections: MatchableSection[]): MatchableItem[] {

  const rank = (kind: string) => (kind === 'summary' ? 0 : kind === 'experience' ? 1 : kind === 'skills' ? 2 : 3);
  return [...sections]
    .filter((section) => section.enabled !== false)
    .sort((a, b) => rank(a.kind) - rank(b.kind) || a.position - b.position)
    .flatMap((section) => {
      const included = section.items.filter((i) => i.selected && !i.unavailable);
      if (included.length === 0) return [];

      const perItemBlocks =
        section.kind === 'projects' ||
        section.kind === 'skills' ||
        section.kind === 'certifications' ||
        section.kind === 'education';
      return included.map((item) => ({
        id: item.id,
        blockKey: perItemBlocks ? item.id : section.id,
        ...matchableText(item),
      }));
    });
}

function matchableText(item: MatchableSection['items'][number]): { text: string; parts?: string[] } {
  const entries = item.skillEntries;
  if (!entries || entries.length === 0) return { text: item.text };
  return {
    text: item.text.split(':')[0],
    parts: entries.filter((e) => e.selected).map((e) => e.text),
  };
}
