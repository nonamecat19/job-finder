import { useEffect, useState } from 'react';

/**
 * SSR/jsdom-safe media query hook.
 *
 * Returns whether the given media query currently matches. When
 * `window.matchMedia` is unavailable (SSR or the jsdom test environment)
 * it resolves to `false`, so consumers fall back to the desktop layout.
 */
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState<boolean>(() => getMatches(query));

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return;
    }
    const mql = window.matchMedia(query);
    const handler = () => setMatches(mql.matches);
    handler();
    mql.addEventListener('change', handler);
    return () => mql.removeEventListener('change', handler);
  }, [query]);

  return matches;
}

function getMatches(query: string): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false;
  }
  return window.matchMedia(query).matches;
}

/** Tailwind `md` breakpoint (768px) — matches the `md:` variants used in the app shell. */
export const MD_BREAKPOINT_QUERY = '(min-width: 768px)';
