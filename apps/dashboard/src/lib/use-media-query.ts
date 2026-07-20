import { useCallback, useSyncExternalStore } from 'react';

/**
 * SSR/jsdom-safe media query hook.
 *
 * Returns `false` in environments without `window.matchMedia` (SSR, jsdom without
 * a polyfill), which keeps rendering deterministic in tests.
 */
export function useMediaQuery(query: string): boolean {
  const subscribe = useCallback(
    (onStoreChange: () => void) => {
      if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
        return () => {};
      }
      const mql = window.matchMedia(query);
      mql.addEventListener('change', onStoreChange);
      return () => mql.removeEventListener('change', onStoreChange);
    },
    [query],
  );

  const getSnapshot = useCallback(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return false;
    }
    return window.matchMedia(query).matches;
  }, [query]);

  return useSyncExternalStore(subscribe, getSnapshot, () => false);
}

/** Tailwind `md` breakpoint (768px) — matches the `md:` variants used in the app shell. */
export const MD_BREAKPOINT_QUERY = '(min-width: 768px)';
