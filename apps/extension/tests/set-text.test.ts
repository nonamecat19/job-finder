import { describe, expect, it, vi } from 'vitest';

import { hasExistingText, setFieldText } from '@/content/set-text';

describe('setFieldText', () => {
  it('writes through the native prototype setter so React notices the change', () => {
    document.body.innerHTML = '<textarea></textarea>';
    const el = document.querySelector('textarea')!;
    const descriptor = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!;
    const spy = vi.fn(descriptor.set!);
    Object.defineProperty(HTMLTextAreaElement.prototype, 'value', { ...descriptor, set: spy });

    try {
      expect(setFieldText(el, 'Dear team')).toBe(true);

      expect(spy).toHaveBeenCalledWith('Dear team');
      expect(el.value).toBe('Dear team');
    } finally {
      Object.defineProperty(HTMLTextAreaElement.prototype, 'value', descriptor);
    }
  });

  it('dispatches a bubbling input event so validators and counters update', () => {
    document.body.innerHTML = '<form><textarea></textarea></form>';
    const el = document.querySelector('textarea')!;
    const seen: string[] = [];
    document.querySelector('form')!.addEventListener('input', () => seen.push('input'));

    setFieldText(el, 'hello');

    expect(seen).toEqual(['input']);
  });

  it('fills a contenteditable editor', () => {
    document.body.innerHTML = '<div contenteditable="true"></div>';
    const el = document.querySelector('div')!;

    expect(setFieldText(el, 'letter body')).toBe(true);
    expect(el.textContent).toBe('letter body');
  });

  it('refuses a field it cannot write to', () => {
    document.body.innerHTML = '<div></div>';
    expect(setFieldText(document.querySelector('div')!, 'x')).toBe(false);
  });
});

describe('hasExistingText', () => {
  it('ignores whitespace-only content', () => {
    document.body.innerHTML = '<textarea>   </textarea>';
    expect(hasExistingText(document.querySelector('textarea')!)).toBe(false);
  });

  it('reports real content so the user is warned it was replaced', () => {
    document.body.innerHTML = '<textarea>draft</textarea>';
    expect(hasExistingText(document.querySelector('textarea')!)).toBe(true);
  });
});
