import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

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
