import { createContext, useContext, useMemo, useState, type ReactNode } from 'react';

/**
 * Which item the pointer is on, and which pane it is on it in. The source
 * matters: the *other* pane is the one that scrolls to follow, and a pane
 * never scrolls itself out from under the pointer.
 */
export interface PreviewHover {
  /**
   * The item under the pointer, when the pointer is on one. A hover can be on
   * a block without being on any single item of it — the entry's heading, the
   * space between two of its bullets — and then only `blockKey` is set.
   */
  itemId?: string;
  /** The block under the pointer, when the pane knows it (the PDF side does). */
  blockKey?: string;
  source: 'list' | 'pdf';
}

interface HighlightContextValue {
  hover: PreviewHover | null;
  setHover: (hover: PreviewHover | null) => void;
}

// The default makes the context optional: ItemRow and the preview render
// perfectly well outside a provider (and in isolation in tests), just without
// cross-pane highlighting.
const HighlightContext = createContext<HighlightContextValue>({ hover: null, setHover: () => {} });

export function PreviewHighlightProvider({ children }: { children: ReactNode }) {
  const [hover, setHover] = useState<PreviewHover | null>(null);
  const value = useMemo(() => ({ hover, setHover }), [hover]);
  return <HighlightContext.Provider value={value}>{children}</HighlightContext.Provider>;
}

export function usePreviewHighlight(): HighlightContextValue {
  return useContext(HighlightContext);
}
