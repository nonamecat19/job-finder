import { describe, expect, it } from 'vitest';

import { base64ToBytes, bytesToBase64 } from '@/shared/base64';

describe('base64 round trip', () => {
  it('preserves bytes', () => {
    const bytes = new Uint8Array([0, 1, 2, 250, 255, 128]);
    expect(Array.from(base64ToBytes(bytesToBase64(bytes)))).toEqual(Array.from(bytes));
  });

  it('handles a payload larger than the chunk size without overflowing the argument stack', () => {
    const bytes = new Uint8Array(300_000).map((_, i) => i % 256);
    const round = base64ToBytes(bytesToBase64(bytes));
    expect(round.length).toBe(bytes.length);
    expect(round[299_999]).toBe(bytes[299_999]);
  });
});
