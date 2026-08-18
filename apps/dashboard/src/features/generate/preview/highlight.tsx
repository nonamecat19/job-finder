import { createContext, useContext, useMemo, useState, type ReactNode } from 'react';

export interface PreviewHover {

  itemId?: string;

  blockKey?: string;
  source: 'list' | 'pdf';
}

interface HighlightContextValue {
  hover: PreviewHover | null;
  setHover: (hover: PreviewHover | null) => void;
}

const HighlightContext = createContext<HighlightContextValue>({ hover: null, setHover: () => {} });

export function PreviewHighlightProvider({ children }: { children: ReactNode }) {
  const [hover, setHover] = useState<PreviewHover | null>(null);
  const value = useMemo(() => ({ hover, setHover }), [hover]);
  return <HighlightContext.Provider value={value}>{children}</HighlightContext.Provider>;
}

export function usePreviewHighlight(): HighlightContextValue {
  return useContext(HighlightContext);
}
