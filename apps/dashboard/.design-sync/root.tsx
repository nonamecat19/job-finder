import type { CSSProperties, ReactNode } from 'react';

/**
 * Root wrapper for anything built with this design system.
 *
 * The component tokens (`--background`, `--foreground`, `--surface`, `--accent`, …)
 * are declared on `:root` in dark mode and re-declared under `[data-theme='light']`.
 * Components read them via Tailwind utilities (`bg-surface`, `text-muted`, …), so a
 * screen that is not painted with the app background renders dark-on-white and looks
 * broken. Wrap every screen in `ThemeRoot` — it paints the background/foreground and
 * sets the Inter type stack, and `theme="light"` switches the whole subtree.
 */
export function ThemeRoot({
  children,
  theme = 'dark',
  style,
  className,
}: {
  children: ReactNode;
  /** `'dark'` (default) or `'light'` — switches the token set for the subtree. */
  theme?: 'dark' | 'light';
  style?: CSSProperties;
  className?: string;
}) {
  return (
    <div
      data-theme={theme}
      className={className}
      style={{
        background: 'var(--background)',
        color: 'var(--foreground)',
        fontFamily: "var(--font-sans, 'Inter', ui-sans-serif, system-ui, sans-serif)",
        colorScheme: theme,
        ...style,
      }}
    >
      {children}
    </div>
  );
}
