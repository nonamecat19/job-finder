/** Returns the first element any selector in the list matches. */
export function queryFirst<T extends Element>(root: ParentNode, selectors: string[]): T | null {
  for (const selector of selectors) {
    const el = root.querySelector<T>(selector);
    if (el) return el;
  }
  return null;
}

/** Every element matched by any selector in the list, in selector order. */
export function queryAll<T extends Element>(root: ParentNode, selectors: string[]): T[] {
  const out: T[] = [];
  for (const selector of selectors) {
    for (const el of Array.from(root.querySelectorAll<T>(selector))) {
      if (!out.includes(el)) out.push(el);
    }
  }
  return out;
}

/**
 * Visibility check for interactive fields.
 *
 * Never apply this to input[type=file]: those are routinely display:none behind
 * a styled label, and filtering them out is exactly how a working adapter
 * appears broken.
 */
export function isVisible(el: Element): boolean {
  const html = el as HTMLElement;
  if (html.hidden) return false;
  if (!html.isConnected) return false;
  const style = getComputedStyle(html);
  return style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0';
}

export function waitForElement<T extends Element>(
  selectors: string[],
  timeoutMs = 3000,
  root: ParentNode = document,
): Promise<T | null> {
  const found = queryFirst<T>(root, selectors);
  if (found) return Promise.resolve(found);

  return new Promise((resolve) => {
    const observer = new MutationObserver(() => {
      const el = queryFirst<T>(root, selectors);
      if (el) {
        observer.disconnect();
        clearTimeout(timer);
        resolve(el);
      }
    });
    const timer = setTimeout(() => {
      observer.disconnect();
      resolve(null);
    }, timeoutMs);
    observer.observe(document.documentElement, { childList: true, subtree: true, attributes: true });
  });
}

/** Text content of the first matching element, trimmed, or null. */
export function textOf(root: ParentNode, selectors: string[]): string | null {
  const el = queryFirst<HTMLElement>(root, selectors);
  const text = el?.textContent?.trim();
  return text ? text : null;
}
