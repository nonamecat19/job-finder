// blockMap.ts — ties the rendered PDF back to the run's items.
//
// The preview PDF is compiled from the run's selection, so every line of it
// came from an item the user can see in the left pane. Nothing in the PDF
// carries that item's id, though — Typst emits glyphs, not provenance. So the
// link is rebuilt here from the one thing both sides share: the text. Each
// item's text is matched against the page's extracted text layer, and the
// text pieces it covers are unioned into per-line rectangles the viewer can
// draw and hit-test.
//
// Rectangles are normalized (0..1 of the page box) so they survive zoom and
// re-render without recomputation.

/** A rectangle in normalized page space: 0..1 of the page's width/height, origin top-left. */
export interface NormRect {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** One run of text extracted from a PDF page, with its normalized box. */
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

/**
 * Where one *block* of the document landed: a whole experience entry, a whole
 * project, the skills list, the summary. A block is the unit the reader sees —
 * hovering one bullet of a role and lighting up that bullet alone reads as a
 * stray underline, where lighting the entry reads as "this is the part of the
 * page you are editing".
 */
export interface PreviewBlock {
  key: string;
  /** The run's items that landed in this block, in document order. */
  itemIds: string[];
  /** The whole block: every item's rectangles, and its heading if it has one. */
  rects: BlockRect[];
  /**
   * The same rectangles kept per item, so the viewer can nest a hover target
   * per bullet inside the block's own — point at the entry and get the entry,
   * point at one of its lines and get that line.
   */
  items: PreviewItemRects[];
}

export interface PreviewItemRects {
  itemId: string;
  rects: BlockRect[];
}

export interface MatchableItem {
  id: string;
  text: string;
  /** Which block this text belongs to — items sharing a key share a highlight. */
  blockKey: string;
  /**
   * Further text that follows `text` on the page in an order this side cannot
   * predict — the skills of a group, which the template lays out in the
   * profile's order rather than the run's. Each part is consumed from the page
   * wherever it turns up next, so the match covers the whole group instead of
   * stopping at its label.
   */
  parts?: string[];
}

/**
 * Everything that isn't a letter or digit is dropped on both sides of the
 * comparison: the PDF's typography (bullet glyphs, en-dashes, ligature-driven
 * spacing, hyphenation at line breaks) differs from the raw item text in
 * exactly those characters and never in the alphanumerics.
 */
export function normalizeForMatch(text: string): string {
  return text.toLowerCase().replace(/[^a-z0-9]+/g, '');
}

/** Below this many alphanumerics a match is more likely coincidence than provenance. */
const MIN_MATCH_CHARS = 6;
/**
 * Prefix lengths tried, longest first, when the whole text doesn't match:
 * the rendered document is often a subset of the item's text — a skill group
 * with entries switched off, a template that truncates — and a prefix still
 * identifies the block. The floor stays well above MIN_MATCH_CHARS so a short
 * prefix can't wander onto a neighbouring bullet.
 */
const PREFIX_FALLBACKS = [60, 40, 30, 20, 12];
/** Two pieces belong to the same visual line when their tops are within this fraction of the page. */
const LINE_TOLERANCE = 0.006;

interface IndexedPage {
  pageIndex: number;
  /** Concatenated normalized text of the whole page. */
  norm: string;
  /** For each character of `norm`, the index of the piece it came from. */
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

/**
 * Maps each item to the rectangles its text occupies in the rendered PDF.
 *
 * Items are matched in document order with a moving cursor, so repeated text
 * (the same bullet under two roles, a skill named in both a group and a
 * project) attaches to the occurrence that follows the previous match rather
 * than always to the first one on the page. Items whose text isn't found —
 * anything the user switched off, or content the template rewrote beyond
 * recognition — are simply absent from the result.
 */
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

/**
 * Walks forward from `from` for as long as the page's next characters are one
 * of `parts` (each usable once), and returns where that run ends. Order is
 * whatever the template chose; a part that isn't there simply doesn't consume
 * anything, so a group whose last skill was dropped still ends cleanly at the
 * next group's label.
 */
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

/** How far above its first line a block's heading may sit and still be part of it. */
const HEADING_GAP = 0.045;
/** Slack on any left-margin comparison — the same margin never lands on the same x twice. */
const INDENT_EPSILON = 0.004;
/** A line further right of the block's margin than this is not indented under it, it is elsewhere. */
const CONTINUATION_INDENT = 0.09;
/** Vertical gap allowed between a block's last line and the line hanging under it. */
const CONTINUATION_GAP = 0.02;

/**
 * Grows every block from "the glyphs its text matched" to the block as the
 * reader sees it: whole visual lines (so the bullet glyph, the dates on the
 * right of an entry's header, the trailing punctuation are all inside it), and
 * the line immediately above — the company/role header, the project's name —
 * as long as no other block already owns it.
 *
 * Doing this geometrically rather than by matching the header's text is what
 * makes it reliable: a header is short, repeats elsewhere in the document, and
 * is laid out by the template rather than carried by any item of the run.
 */
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

  // A second pass, so a header is only claimed once every block's own lines
  // are accounted for — the line above one block is often the last line of
  // the one before it.
  for (const block of blocks) {
    const top = block.rects.reduce<BlockRect | null>((a, b) => (!a || b.y < a.y ? b : a), null);
    if (!top) continue;
    const heading = (lines.get(top.pageIndex) ?? [])
      .filter(
        (l) =>
          l.y < top.y &&
          top.y - l.y <= HEADING_GAP &&
          !owned.has(lineKey(l)) &&
          // A header is never indented further than what it heads: it sits on
          // the block's own left margin or out to the left of it. That is what
          // separates an entry's company/role line from the last bullet of the
          // entry before it, and from a centred section title ("Projects",
          // "Education"), both of which start further right.
          l.x <= top.x + INDENT_EPSILON,
      )
      .sort((a, b) => b.y - a.y)[0];
    if (!heading) continue;
    owned.add(lineKey(heading));
    mergeRects(block.rects, [heading]);
  }

  // A third pass for what hangs below: a project's bullets are laid out by the
  // template under the project's name, and no item of the run carries their
  // text, so they can only be found by where they sit — indented under the
  // block, unclaimed, one line after another until that stops being true.
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

  // Reading order, so the block's first rectangle is its first line — which is
  // what the viewer scrolls to.
  for (const block of blocks) {
    block.rects.sort((a, b) => a.pageIndex - b.pageIndex || a.y - b.y);
  }

  return blocks;
}

/** Every visual line of a page, as the box covering all of that line's text. */
function lineBoxesOf(page: IndexedPage): BlockRect[] {
  const lines: BlockRect[] = [];
  for (const piece of page.pieces) {
    if (!piece.str.trim()) continue;
    mergeRects(lines, [{ ...piece.rect, pageIndex: page.pageIndex }]);
  }
  return lines.sort((a, b) => a.y - b.y);
}

/** Adds `incoming` to `into`, folding rectangles that share a visual line. */
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

/**
 * Searches for `needle` starting at (cursorPage, cursorOffset), then the rest
 * of that page's later pages, then wraps to the beginning — and retries with
 * progressively shorter prefixes before giving up.
 */
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
    // Nothing after the cursor on the cursor's own page — the match may sit
    // before it (a re-ordered section), so try that page from the top too.
    const start = pages[cursorPage]?.norm.indexOf(candidate, 0) ?? -1;
    if (start !== -1) return { pageIndexInList: cursorPage, start, length: candidate.length };
  }
  return null;
}

/** Unions the pieces covering [start, start+length) into one rectangle per visual line. */
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
  items: {
    id: string;
    text: string;
    selected: boolean;
    unavailable?: boolean;
    skillEntries?: { text: string; selected: boolean }[];
  }[];
}

/** The run's currently-included items, in document order, ready to match. */
export function matchableItems(sections: MatchableSection[]): MatchableItem[] {
  // Document order as the template lays it out: summary, experience (by
  // position), skills, then everything else (projects, certifications).
  const rank = (kind: string) => (kind === 'summary' ? 0 : kind === 'experience' ? 1 : kind === 'skills' ? 2 : 3);
  return [...sections]
    .sort((a, b) => rank(a.kind) - rank(b.kind) || a.position - b.position)
    .flatMap((section) => {
      const included = section.items.filter((i) => i.selected && !i.unavailable);
      if (included.length === 0) return [];
      // A project, a skill group, a certification, and an education entry
      // each stand on their own — the reader sees one line of skills, not
      // "the skills section". Everything else is blocked by section: one
      // experience entry, the summary.
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

/**
 * What this item actually contributes to the page. A skill group is its label
 * followed by the skills still switched on — and only that: `item.text` also
 * carries the ones the user dropped, and even the kept ones come out in the
 * profile's order rather than the run's, so the group is matched as a label
 * plus a bag of parts instead of as one string.
 */
function matchableText(item: MatchableSection['items'][number]): { text: string; parts?: string[] } {
  const entries = item.skillEntries;
  if (!entries || entries.length === 0) return { text: item.text };
  return {
    text: item.text.split(':')[0],
    parts: entries.filter((e) => e.selected).map((e) => e.text),
  };
}
