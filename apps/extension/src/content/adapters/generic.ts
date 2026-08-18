import { isVisible, queryAll, queryFirst } from '../dom';

/**
 * Structural fallbacks, used as the last entry of every adapter's selector
 * chain. Class names on these sites are churned regularly; the shape of an
 * apply form ("a form holding a big textarea and a file input") is stable.
 */

/** The apply form: the form that holds a visible textarea, else the one holding a file input. */
export function guessApplyForm(): HTMLFormElement | null {
  const forms = Array.from(document.querySelectorAll('form'));
  const withTextarea = forms.find((form) =>
    Array.from(form.querySelectorAll('textarea')).some((t) => isVisible(t)),
  );
  if (withTextarea) return withTextarea;
  return forms.find((form) => form.querySelector('input[type="file"]')) ?? null;
}

/**
 * Visibility is never used to filter a file input: sites hide them behind a
 * styled label and click them programmatically.
 */
export function guessFileInput(scope: ParentNode | null): HTMLInputElement | null {
  const root = scope ?? document;
  return (
    queryFirst<HTMLInputElement>(root, ['input[type="file"][accept*="pdf"]', 'input[type="file"]']) ??
    (root === document ? null : document.querySelector<HTMLInputElement>('input[type="file"]'))
  );
}

/** The letter field: the largest visible textarea in scope, then any visible one. */
export function guessLetterField(scope: ParentNode | null): HTMLElement | null {
  const root = scope ?? document;
  const areas = queryAll<HTMLTextAreaElement>(root, ['textarea']).filter((t) => isVisible(t));
  if (areas.length > 0) {
    return areas.reduce((biggest, t) => (rows(t) > rows(biggest) ? t : biggest));
  }
  const editable = queryAll<HTMLElement>(root, ['[contenteditable="true"]']).filter((e) => isVisible(e));
  return editable[0] ?? null;
}

function rows(el: HTMLTextAreaElement): number {
  const attr = Number(el.getAttribute('rows'));
  return Number.isFinite(attr) && attr > 0 ? attr : 2;
}

/** A button/link whose text matches any of the given labels (case-insensitive). */
export function findByText(labels: string[], selectors: string[] = ['button', 'a', 'input[type="submit"]']): HTMLElement | null {
  const wanted = labels.map((l) => l.toLowerCase());
  for (const el of queryAll<HTMLElement>(document, selectors)) {
    const text = (el.textContent ?? (el as HTMLInputElement).value ?? '').trim().toLowerCase();
    if (!text) continue;
    if (wanted.some((label) => text.includes(label)) && isVisible(el)) return el;
  }
  return null;
}
