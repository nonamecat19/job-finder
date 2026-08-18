/**
 * Writes text into a field so that a framework-controlled form notices.
 *
 * A plain `el.value = text` bypasses React's value tracker: the DOM shows the
 * text, React's state does not, and the site submits an empty letter. Going
 * through the native prototype setter and then dispatching a bubbling `input`
 * is what makes the change real for React, Vue and the sites' own listeners.
 */
export function setFieldText(el: HTMLElement, text: string): boolean {
  if (el instanceof HTMLTextAreaElement || el instanceof HTMLInputElement) {
    const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
    if (setter) setter.call(el, text);
    else el.value = text;
    el.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
    el.dispatchEvent(new Event('change', { bubbles: true, composed: true }));
    return true;
  }

  // jsdom leaves isContentEditable undefined, and some editors set the attribute
  // on a wrapper without the property reflecting it — check both.
  if (el.isContentEditable || el.getAttribute('contenteditable') === 'true') {
    el.focus();
    const selection = window.getSelection();
    if (selection) {
      const range = document.createRange();
      range.selectNodeContents(el);
      selection.removeAllRanges();
      selection.addRange(range);
    }
    // execCommand keeps the editor's own undo stack and change events intact
    // where it exists; jsdom and a few editors leave it stubbed, hence the fallback.
    const inserted = typeof document.execCommand === 'function' && document.execCommand('insertText', false, text);
    if (!inserted) el.textContent = text;
    el.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
    return true;
  }

  return false;
}

/** True when the field already holds something the user would not want overwritten silently. */
export function hasExistingText(el: HTMLElement): boolean {
  const current =
    el instanceof HTMLTextAreaElement || el instanceof HTMLInputElement ? el.value : (el.textContent ?? '');
  return current.trim() !== '';
}
