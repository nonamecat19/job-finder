import { createContext, ReactNode, useCallback, useContext, useEffect, useState } from 'react';

export type Theme = 'light' | 'dark';

const STORAGE_KEY = 'jf-theme';
const RESUME_DARK_KEY = 'jf-resume-dark';

function readStoredTheme(): Theme | null {
  if (typeof window.localStorage === 'undefined') return null;
  const stored = window.localStorage.getItem(STORAGE_KEY);
  return stored === 'light' || stored === 'dark' ? stored : null;
}

function readStoredResumeDark(): boolean {
  if (typeof window.localStorage === 'undefined') return false;
  return window.localStorage.getItem(RESUME_DARK_KEY) === 'true';
}

function systemTheme(): Theme {
  return typeof window.matchMedia === 'function' && window.matchMedia('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light';
}

interface ThemeContextValue {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  /**
   * Whether rendered resume documents are shown inverted — a dark sheet with
   * light text. It rides along with the dashboard theme but is a separate
   * preference: the document itself is still printed on white paper, so a
   * dark dashboard doesn't imply the reader wants a dark page.
   */
  resumeDark: boolean;
  setResumeDark: (resumeDark: boolean) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => readStoredTheme() ?? systemTheme());
  const [resumeDark, setResumeDarkState] = useState<boolean>(readStoredResumeDark);

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);

  const setTheme = useCallback((next: Theme) => {
    if (typeof window.localStorage !== 'undefined') {
      window.localStorage.setItem(STORAGE_KEY, next);
    }
    setThemeState(next);
  }, []);

  const setResumeDark = useCallback((next: boolean) => {
    if (typeof window.localStorage !== 'undefined') {
      window.localStorage.setItem(RESUME_DARK_KEY, String(next));
    }
    setResumeDarkState(next);
  }, []);

  return (
    <ThemeContext.Provider value={{ theme, setTheme, resumeDark, setResumeDark }}>{children}</ThemeContext.Provider>
  );
}

/**
 * How a rendered document is shown on a dark sheet: its luminance flipped,
 * hues kept. Pair it with `bg-paper-dark` and `mix-blend-screen` on the
 * flipped element — the flip alone turns white stock pure black, and screening
 * it over the token colour lands the page on the design system's dark surface
 * while leaving the (now light) text alone.
 */
export const RESUME_DARK_FILTER = 'invert(1) hue-rotate(180deg)';

export function useTheme() {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used within ThemeProvider');
  return ctx;
}
