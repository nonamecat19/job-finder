import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * Scrolls `el`'s nearest scrolling ancestor so `el`'s top sits `offset`
 * pixels below the container's own top, instead of flush against it — flush
 * against the edge is where the container's own fade/border eats into it.
 * Falls back to a plain scrollIntoView when no scrolling ancestor is found.
 */
export function scrollIntoViewWithOffset(el: HTMLElement, offset: number, behavior: ScrollBehavior = 'smooth'): void {
  let parent = el.parentElement;
  while (parent) {
    const style = getComputedStyle(parent);
    if (/(auto|scroll)/.test(style.overflowY) && parent.scrollHeight > parent.clientHeight) break;
    parent = parent.parentElement;
  }
  if (!parent) {
    el.scrollIntoView({ block: 'start', behavior });
    return;
  }
  const elRect = el.getBoundingClientRect();
  const parentRect = parent.getBoundingClientRect();
  const delta = elRect.top - parentRect.top - offset;
  parent.scrollTo({ top: parent.scrollTop + delta, behavior });
}
