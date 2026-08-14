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
    names.push(m[1]);
  }
  return [...new Set(names)];
}

const lightBlock = css.match(/:root,\s*\[data-theme='light'\]\s*\{([^}]+)\}/s)?.[1] ?? '';
const darkBlock = css.match(/\[data-theme='dark'\]\s*\{([^}]+)\}/s)?.[1] ?? '';

const NEUTRAL_TOKENS = [
  'background', 'background-secondary', 'background-tertiary',
  'foreground', 'surface', 'surface-secondary', 'surface-tertiary',
  'overlay', 'muted', 'faint', 'border', 'border-strong', 'separator',
];

const STATUS_TOKENS = ['success', 'warning', 'danger'];

const TINT_TOKENS = ['violet', 'blue', 'mint', 'amber', 'rose'];

describe('Token invariants (T1-T5)', () => {
  it('T1: both theme blocks were found', () => {
    expect(lightBlock).not.toBe('');
    expect(darkBlock).not.toBe('');
  });

  it('T2: neutrals stay near-grey — a trace of blue chroma, never a colour', () => {
    for (const block of [lightBlock, darkBlock]) {
      for (const v of extractOklchValues(block)) {
        if (NEUTRAL_TOKENS.includes(v.name)) {
          expect(v.c, `Neutral token --${v.name} has chroma ${v.c}, expected <= 0.02`).toBeLessThanOrEqual(0.02);
        }
      }
    }
  });

  it('T3: accent is the only saturated non-status, non-tint token', () => {
    for (const block of [lightBlock, darkBlock]) {
      const chromatic = extractOklchValues(block)
        .filter((v) => v.c > 0.05)
        .filter((v) => !STATUS_TOKENS.includes(v.name))
        .filter((v) => !v.name.startsWith('tint-'));
      expect(chromatic.map((v) => v.name)).toEqual(['accent']);
    }
  });

  it('T4: both [data-theme] blocks define identical token sets', () => {
    expect(extractTokenNames(lightBlock).sort()).toEqual(extractTokenNames(darkBlock).sort());
  });

  it('T5: every pastel tint pairs a background with a same-hue foreground', () => {
    for (const block of [lightBlock, darkBlock]) {
      const names = extractTokenNames(block);
      for (const tint of TINT_TOKENS) {
        expect(names, `--tint-${tint} is missing`).toContain(`tint-${tint}`);
        expect(names, `--tint-${tint}-fg is missing`).toContain(`tint-${tint}-fg`);
      }
    }
  });
});

describe('Canvas invariants', () => {
  it('the canvas is one flat colour — no gradients anywhere', () => {
    expect(css).not.toMatch(/gradient\(/);
  });

  it('light is the default theme', () => {
    const html = readFileSync(resolve(__dirname, '../index.html'), 'utf-8');
    expect(html).toContain('data-theme="light"');
  });
});
