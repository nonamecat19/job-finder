import { describe, it, expect } from 'vitest';
import { mapItemsToBlocks, matchableItems, normalizeForMatch, type MatchableItem, type PdfPageText } from './blockMap';

function page(pageIndex: number, lines: { str: string; y: number; x?: number; w?: number }[]): PdfPageText {
  return {
    pageIndex,
    pieces: lines.map((l) => ({
      str: l.str,
      rect: { x: l.x ?? 0.1, y: l.y, w: l.w ?? 0.8, h: 0.02 },
    })),
  };
}

/** An item in a block of its own — the default for these geometry cases. */
function item(id: string, text: string, blockKey = id): MatchableItem {
  return { id, text, blockKey };
}

describe('normalizeForMatch', () => {
  it('drops everything the typography can change', () => {
    expect(normalizeForMatch('Shipped the thing — 40% faster!')).toBe('shippedthething40faster');
  });
});

describe('mapItemsToBlocks', () => {
  it('locates an item on the page that rendered it', () => {
    const pages = [page(0, [{ str: 'Shipped the thing', y: 0.2 }, { str: 'Cut latency in half', y: 0.3 }])];
    const blocks = mapItemsToBlocks(pages, [
      item('item-1', 'Shipped the thing'),
      item('item-2', 'Cut latency in half'),
    ]);

    expect(blocks).toHaveLength(2);
    expect(blocks[0]).toMatchObject({ key: 'item-1', itemIds: ['item-1'] });
    expect(blocks[0].rects[0]).toMatchObject({ pageIndex: 0, y: 0.2 });
    expect(blocks[1].rects[0]).toMatchObject({ pageIndex: 0, y: 0.3 });
  });

  it('unions a wrapped bullet into one rectangle per line', () => {
    const pages = [
      page(0, [
        { str: 'Shipped the thing that made the ', y: 0.2, x: 0.1, w: 0.7 },
        { str: 'whole pipeline twice as fast', y: 0.23, x: 0.1, w: 0.5 },
      ]),
    ];
    const blocks = mapItemsToBlocks(pages, [item('item-1', 'Shipped the thing that made the whole pipeline twice as fast')]);

    expect(blocks[0].rects).toHaveLength(2);
    expect(blocks[0].rects.map((r) => r.y)).toEqual([0.2, 0.23]);
  });

  it('matches across pieces split mid-word by the text layer', () => {
    const pages = [page(0, [{ str: 'Shipped the th', y: 0.2 }, { str: 'ing', y: 0.2, x: 0.5, w: 0.1 }])];
    const blocks = mapItemsToBlocks(pages, [item('item-1', 'Shipped the thing')]);
    // Same visual line, so both pieces collapse into one rectangle.
    expect(blocks[0].rects).toHaveLength(1);
    expect(blocks[0].rects[0].w).toBeCloseTo(0.8, 5);
  });

  it('attaches repeated text to the occurrence that follows the previous match', () => {
    const pages = [
      page(0, [
        { str: 'Owned the release process', y: 0.2 },
        { str: 'Owned the release process', y: 0.5 },
      ]),
    ];
    const blocks = mapItemsToBlocks(pages, [
      item('item-1', 'Owned the release process'),
      item('item-2', 'Owned the release process'),
    ]);
    expect(blocks.map((b) => b.rects[0].y)).toEqual([0.2, 0.5]);
  });

  it('falls back to a prefix when the tail of the text was not rendered', () => {
    const pages = [page(0, [{ str: 'Languages: Go, TypeScript', y: 0.4 }])];
    const blocks = mapItemsToBlocks(pages, [item('item-1', 'Languages: Go, TypeScript, Rust, Elixir, Haskell, and more')]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].rects[0].y).toBe(0.4);
  });

  it('spans pages, and skips items that are not in the document at all', () => {
    const pages = [page(0, [{ str: 'First page bullet', y: 0.2 }]), page(1, [{ str: 'Second page bullet', y: 0.1 }])];
    const blocks = mapItemsToBlocks(pages, [
      item('item-1', 'First page bullet'),
      item('item-2', 'Second page bullet'),
      item('item-3', 'Never rendered anywhere'),
    ]);
    expect(blocks.map((b) => [b.key, b.rects[0].pageIndex])).toEqual([
      ['item-1', 0],
      ['item-2', 1],
    ]);
  });

  it('ignores text too short to identify', () => {
    const pages = [page(0, [{ str: 'Go', y: 0.2 }])];
    expect(mapItemsToBlocks(pages, [item('item-1', 'Go')])).toEqual([]);
  });

  it('collects every item of one block into a single highlight', () => {
    const pages = [
      page(0, [
        { str: 'Shipped the thing', y: 0.2 },
        { str: 'Cut latency in half', y: 0.25 },
        { str: 'Something from another entry', y: 0.5 },
      ]),
    ];
    const blocks = mapItemsToBlocks(pages, [
      item('item-1', 'Shipped the thing', 'entry-1'),
      item('item-2', 'Cut latency in half', 'entry-1'),
      item('item-3', 'Something from another entry', 'entry-2'),
    ]);

    expect(blocks).toHaveLength(2);
    expect(blocks[0]).toMatchObject({ key: 'entry-1', itemIds: ['item-1', 'item-2'] });
    expect(blocks[0].rects.map((r) => r.y)).toEqual([0.2, 0.25]);
    expect(blocks[1]).toMatchObject({ key: 'entry-2', itemIds: ['item-3'] });
  });

  it('takes in the header line above a block, and never one another block owns', () => {
    const pages = [
      page(0, [
        { str: 'NethuntCRM, Senior Full Stack Developer', y: 0.18 },
        { str: 'Shipped the thing', y: 0.2 },
        { str: 'Cut latency in half', y: 0.22 },
        { str: 'MaybeWorks, Full Stack Developer', y: 0.3 },
        { str: 'Decomposed the monolith', y: 0.32 },
      ]),
    ];
    const blocks = mapItemsToBlocks(pages, [
      item('item-1', 'Shipped the thing', 'entry-1'),
      item('item-2', 'Cut latency in half', 'entry-1'),
      item('item-3', 'Decomposed the monolith', 'entry-2'),
    ]);

    // Each entry reaches up over its own header line…
    expect(blocks[0].rects.map((r) => r.y)).toEqual([0.18, 0.2, 0.22]);
    expect(blocks[1].rects.map((r) => r.y)).toEqual([0.3, 0.32]);
    // …and the line above entry-2's header is entry-1's last bullet, which
    // stays where it is.
    expect(blocks[1].rects.some((r) => r.y === 0.22)).toBe(false);
  });

  it('leaves a centred section title out of the block below it', () => {
    const pages = [
      page(0, [
        { str: 'Experience', y: 0.16, x: 0.45, w: 0.1 },
        { str: 'Shipped the thing', y: 0.2, x: 0.06, w: 0.8 },
      ]),
    ];
    const [block] = mapItemsToBlocks(pages, [item('item-1', 'Shipped the thing', 'entry-1')]);

    // The title is a page landmark, not this entry's header — it sits on no
    // block's left margin.
    expect(block.rects.map((r) => r.y)).toEqual([0.2]);
  });

  it('takes the bullets hanging under a project, and stops at the next one', () => {
    // What the template lays out under a project's name is not carried by any
    // item of the run — the item is the project line itself.
    const pages = [
      page(0, [
        { str: 'job-finder — Self-hosted AI job-search platform', y: 0.2, x: 0.06, w: 0.85 },
        { str: 'Architected a Go and TypeScript platform', y: 0.22, x: 0.085, w: 0.8 },
        { str: 'Built the async backbone on six queues', y: 0.24, x: 0.085, w: 0.8 },
        { str: 'go-orm — Code-generating ORM for Go', y: 0.27, x: 0.06, w: 0.85 },
        { str: 'Built a Go ORM with a Cobra CLI', y: 0.29, x: 0.085, w: 0.8 },
      ]),
    ];
    const blocks = mapItemsToBlocks(pages, [
      item('p1', 'job-finder — Self-hosted AI job-search platform'),
      item('p2', 'go-orm — Code-generating ORM for Go'),
    ]);

    expect(blocks[0].rects.map((r) => r.y)).toEqual([0.2, 0.22, 0.24]);
    // The second project reaches down over its own bullet and never up over
    // the first project's last one.
    expect(blocks[1].rects.map((r) => r.y)).toEqual([0.27, 0.29]);
  });

  it('grows a matched line out to the whole visual line it sits on', () => {
    const pages = [
      page(0, [
        { str: '•', y: 0.2, x: 0.05, w: 0.02 },
        { str: 'Shipped the thing', y: 0.2, x: 0.1, w: 0.4 },
      ]),
    ];
    const [block] = mapItemsToBlocks(pages, [item('item-1', 'Shipped the thing')]);

    // The bullet glyph is not part of any item's text, but it is part of the
    // line the reader sees.
    expect(block.rects[0].x).toBeCloseTo(0.05, 5);
    expect(block.rects[0].w).toBeCloseTo(0.45, 5);
  });
});

describe('matchableItems', () => {
  it('lists the included items in the order the template lays them out', () => {
    const items = matchableItems([
      {
        id: 'sec-skills',
        kind: 'skills',
        position: 0,
        items: [{ id: 's1', text: 'Languages: Go', selected: true, unavailable: false }],
      },
      {
        id: 'sec-e2',
        kind: 'experience',
        position: 2,
        items: [{ id: 'e2', text: 'Second job bullet', selected: true, unavailable: false }],
      },
      {
        id: 'sec-e1',
        kind: 'experience',
        position: 1,
        items: [
          { id: 'e1', text: 'First job bullet', selected: true, unavailable: false },
          { id: 'e1-off', text: 'Left out', selected: false, unavailable: false },
          { id: 'e1-gone', text: 'No longer in profile', selected: true, unavailable: true },
        ],
      },
      { id: 'sec-sum', kind: 'summary', position: 0, items: [{ id: 'sum', text: 'A summary', selected: true }] },
    ]);

    expect(items.map((i) => i.id)).toEqual(['sum', 'e1', 'e2', 's1']);
  });

  it('blocks an experience entry by section and a project by itself', () => {
    const items = matchableItems([
      {
        id: 'sec-e1',
        kind: 'experience',
        entryLabel: 'NethuntCRM',
        position: 0,
        items: [
          { id: 'e1', text: 'First job bullet', selected: true },
          { id: 'e2', text: 'Second job bullet', selected: true },
        ],
      },
      {
        id: 'sec-proj',
        kind: 'projects',
        position: 0,
        items: [
          { id: 'p1', text: 'A project of some kind', selected: true },
          { id: 'p2', text: 'Another project entirely', selected: true },
        ],
      },
    ]);

    expect(items.map((i) => [i.id, i.blockKey])).toEqual([
      ['e1', 'sec-e1'],
      ['e2', 'sec-e1'],
      ['p1', 'p1'],
      ['p2', 'p2'],
    ]);
  });

  it('blocks a certification and an education entry each by itself', () => {
    const items = matchableItems([
      {
        id: 'sec-certs',
        kind: 'certifications',
        position: 0,
        items: [
          { id: 'c1', text: 'AWS SAA — Amazon, 2023', selected: true },
          { id: 'c2', text: 'CKA — CNCF, 2022', selected: true },
        ],
      },
      {
        id: 'sec-edu',
        kind: 'education',
        position: 0,
        items: [{ id: 'd1', text: 'BS Computer Science — MIT, 2020', selected: true }],
      },
    ]);

    expect(items.map((i) => [i.id, i.blockKey])).toEqual([
      ['c1', 'c1'],
      ['c2', 'c2'],
      ['d1', 'd1'],
    ]);
  });

  it('matches a skill group on the skills still switched on', () => {
    const [skills] = matchableItems([
      {
        id: 'sec-skills',
        kind: 'skills',
        position: 0,
        items: [
          {
            id: 's1',
            text: 'Languages: Go, TypeScript, Rust',
            selected: true,
            skillEntries: [
              { text: 'Go', selected: true },
              { text: 'TypeScript', selected: false },
              { text: 'Rust', selected: true },
            ],
          },
        ],
      },
    ]);

    // TypeScript was dropped by the user, so it is not on the page either —
    // and the kept two come out in the template's order, not this one, so they
    // are matched as parts rather than as one string.
    expect(skills).toMatchObject({ text: 'Languages', parts: ['Go', 'Rust'] });
  });

  it('matches a skill group whatever order the template puts its skills in', () => {
    const pages = [page(0, [{ str: 'Languages: Rust, Go, TypeScript', y: 0.4 }])];
    const [block] = mapItemsToBlocks(pages, [
      { id: 's1', blockKey: 's1', text: 'Languages', parts: ['Go', 'TypeScript', 'Rust'] },
    ]);

    // The whole line, not just the label it started from.
    expect(block.rects).toHaveLength(1);
    expect(block.rects[0].w).toBeCloseTo(0.8, 5);
  });
});

describe('nested item rectangles', () => {
  it('keeps each item of a block addressable on its own', () => {
    const pages = [
      page(0, [
        { str: 'Shipped the thing', y: 0.2 },
        { str: 'Cut latency in half', y: 0.25 },
      ]),
    ];
    const [block] = mapItemsToBlocks(pages, [
      item('item-1', 'Shipped the thing', 'entry-1'),
      item('item-2', 'Cut latency in half', 'entry-1'),
    ]);

    expect(block.items.map((i) => i.itemId)).toEqual(['item-1', 'item-2']);
    expect(block.items[0].rects.map((r) => r.y)).toEqual([0.2]);
    expect(block.items[1].rects.map((r) => r.y)).toEqual([0.25]);
    // The block still covers both of them.
    expect(block.rects.map((r) => r.y)).toEqual([0.2, 0.25]);
  });
});
