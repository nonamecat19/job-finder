import type { CSSProperties, ReactNode } from 'react';

export function ThemeRoot({
  children,
  theme = 'dark',
  style,
  className,
}: {
  children: ReactNode;
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
