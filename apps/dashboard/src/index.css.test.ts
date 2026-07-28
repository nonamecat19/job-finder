import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const cssPath = resolve(__dirname, './index.css');
const css = readFileSync(cssPath, 'utf-8');

function extractOklchValues(block: string): { name: string; l: number; c: number }[] {
  const re = /--([\w-]+):\s*oklch\(([\d.]+)\s+([\d.]+)\s+[\d.]+\)/g;
  const results: { name: string; l: number; c: number }[] = [];
  let m;
  while ((m = re.exec(block)) !== null) {
    results.push({ name: m[1], l: parseFloat(m[2]), c: parseFloat(m[3]) });
  }
  return results;
}

function extractTokenNames(block: string): string[] {
  const re = /--([\w-]+):/g;
  const names: string[] = [];
  let m;
  while ((m = re.exec(block)) !== null) {
    const name = m[1];
    if (!['breakpoint', 'container', 'radius', 'font', 'color'].some((p) => name.startsWith(p)) && name !== 'spacing') {
      names.push(name);
    }
  }
  return [...new Set(names)];
}

const NEUTRAL_TOKENS = [
  'background', 'background-secondary', 'background-tertiary',
  'foreground', 'surface', 'surface-secondary', 'surface-tertiary',
  'overlay', 'muted', 'faint', 'border', 'border-strong', 'separator',
];

const STATUS_TOKENS = ['success', 'warning', 'danger'];

describe('Token invariants (T1-T4)', () => {
  it('T1: every neutral token has chroma 0', () => {
    const darkBlock = css.match(/:root,\s*\[data-theme='dark'\]\s*\{([^}]+)\}/s)?.[1] ?? '';
    const lightBlock = css.match(/\[data-theme='light'\]\s*\{([^}]+)\}/s)?.[1] ?? '';

    for (const block of [darkBlock, lightBlock]) {
      const values = extractOklchValues(block);
      for (const v of values) {
        if (NEUTRAL_TOKENS.includes(v.name)) {
          expect(v.c, `Neutral token --${v.name} has chroma ${v.c}, expected 0`).toBe(0);
        }
      }
    }
  });

  it('T2: exactly one non-status chromatic token (--accent)', () => {
    const darkBlock = css.match(/:root,\s*\[data-theme='dark'\]\s*\{([^}]+)\}/s)?.[1] ?? '';
    const values = extractOklchValues(darkBlock);
    const chromatic = values.filter((v) => v.c > 0 && !STATUS_TOKENS.includes(v.name));
    expect(chromatic.map((v) => v.name)).toEqual(['accent']);
  });

  it('T4: both [data-theme] blocks define identical token sets', () => {
    const darkBlock = css.match(/:root,\s*\[data-theme='dark'\]\s*\{([^}]+)\}/s)?.[1] ?? '';
    const lightBlock = css.match(/\[data-theme='light'\]\s*\{([^}]+)\}/s)?.[1] ?? '';

    const darkNames = extractTokenNames(darkBlock).sort();
    const lightNames = extractTokenNames(lightBlock).sort();

    expect(darkNames).toEqual(lightNames);
  });
});
