import { useEffect, useRef, type ReactNode } from 'react';

export interface BlockMenuAction {
  key: string;
  label: string;
  icon: ReactNode;
  danger?: boolean;
  onSelect: () => void;
}

export interface BlockContextMenuProps {
  /** Viewport coordinates of the click that opened it. */
  x: number;
  y: number;
  actions: BlockMenuAction[];
  onClose: () => void;
}

// A context menu for one block of the rendered page. Fixed to the viewport
// rather than to the page element: the preview is a scrolling, zooming surface
// and a menu anchored inside it would slide away from the pointer the moment
// either changes — which is also why any scroll simply closes it.
export default function BlockContextMenu({ x, y, actions, onClose }: BlockContextMenuProps) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Nudge back inside the viewport once the real size is known, instead of
    // guessing at a width the menu's own labels decide.
    const el = ref.current;
    if (!el) return;
    const box = el.getBoundingClientRect();
    if (box.right > window.innerWidth - 8) el.style.left = `${Math.max(8, window.innerWidth - box.width - 8)}px`;
    if (box.bottom > window.innerHeight - 8) el.style.top = `${Math.max(8, y - box.height)}px`;
  }, [x, y]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    const onPointer = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) onClose();
    };
    window.addEventListener('keydown', onKey);
    // Capture: the surface underneath stops propagation of its own pointer
    // handling in places, and a click anywhere should still dismiss this.
    window.addEventListener('pointerdown', onPointer, true);
    window.addEventListener('scroll', onClose, true);
    window.addEventListener('resize', onClose);
    return () => {
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('pointerdown', onPointer, true);
      window.removeEventListener('scroll', onClose, true);
      window.removeEventListener('resize', onClose);
    };
  }, [onClose]);

  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>('button')?.focus();
  }, []);

  return (
    <div
      ref={ref}
      role="menu"
      data-testid="preview-block-menu"
      style={{ left: x, top: y }}
      className="fixed z-50 min-w-[11rem] rounded-xl border border-border bg-surface p-1 shadow-overlay"
    >
      {actions.map((action) => (
        <button
          key={action.key}
          type="button"
          role="menuitem"
          onClick={() => {
            action.onSelect();
            onClose();
          }}
          className={
            'flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm outline-none ' +
            (action.danger
              ? 'text-danger hover:bg-danger-soft focus:bg-danger-soft'
              : 'text-foreground hover:bg-surface-tertiary focus:bg-surface-tertiary')
          }
        >
          <span className="shrink-0 text-faint">{action.icon}</span>
          {action.label}
        </button>
      ))}
    </div>
  );
}
