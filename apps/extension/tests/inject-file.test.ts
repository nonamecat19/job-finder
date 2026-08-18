import { describe, expect, it } from 'vitest';

import { fileFromBase64, injectFile } from '@/content/inject-file';
import { bytesToBase64 } from '@/shared/base64';

function makeInput(): HTMLInputElement {
  document.body.innerHTML = '<form><input type="file" /></form>';
  return document.querySelector('input')!;
}

describe('injectFile', () => {
  it('puts the file on the input and fires the events upload widgets listen for', () => {
    const input = makeInput();
    const events: string[] = [];
    input.addEventListener('input', () => events.push('input'));
    input.addEventListener('change', () => events.push('change'));

    injectFile(input, new File([new Uint8Array([1, 2])], 'cv.pdf', { type: 'application/pdf' }));

    expect(input.files?.length).toBe(1);
    expect(input.files?.[0].name).toBe('cv.pdf');
    expect(events).toEqual(['input', 'change']);
  });

  it('bubbles its events so a listener on the form sees them', () => {
    const input = makeInput();
    const form = document.querySelector('form')!;
    const seen: string[] = [];
    form.addEventListener('change', () => seen.push('change'));

    injectFile(input, new File([], 'cv.pdf', { type: 'application/pdf' }));

    expect(seen).toEqual(['change']);
  });

  it('falls back to defining files directly when DataTransfer is unavailable', () => {
    const input = makeInput();
    const original = globalThis.DataTransfer;
    // @ts-expect-error deliberately removing the API to exercise the fallback
    delete globalThis.DataTransfer;
    try {
      injectFile(input, new File([], 'cover.pdf', { type: 'application/pdf' }));
      expect(input.files?.length).toBe(1);
      expect(input.files?.item(0)?.name).toBe('cover.pdf');
    } finally {
      globalThis.DataTransfer = original;
    }
  });
});

describe('fileFromBase64', () => {
  it('rebuilds the exact bytes the worker sent', async () => {
    const bytes = new Uint8Array([37, 80, 68, 70]);
    const file = fileFromBase64(bytesToBase64(bytes), 'cv.pdf');

    expect(file.name).toBe('cv.pdf');
    expect(file.type).toBe('application/pdf');
    expect(new Uint8Array(await file.arrayBuffer())).toEqual(bytes);
  });
});

describe('DataTransfer path', () => {
  it('is used when the API exists, because that is what a real browser gives us', () => {
    const input = makeInput();
    const file = new File([], 'cv.pdf', { type: 'application/pdf' });
    const items: File[] = [];
    class FakeDataTransfer {
      items = { add: (f: File) => items.push(f) };
      get files() {
        return { length: items.length, 0: items[0], item: (i: number) => items[i] ?? null } as unknown as FileList;
      }
    }
    const original = globalThis.DataTransfer;

    (globalThis as { DataTransfer?: unknown }).DataTransfer = FakeDataTransfer;
    try {
      injectFile(input, file);
      expect(items).toEqual([file]);
      expect(input.files?.item(0)).toBe(file);
    } finally {
      globalThis.DataTransfer = original;
    }
  });
});
