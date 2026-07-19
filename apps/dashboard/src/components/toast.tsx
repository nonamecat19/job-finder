import * as ToastPrimitive from '@radix-ui/react-toast';
import { AlertTriangle, CheckCircle2, Info, X } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { cn } from '../lib/utils';
import {
  emitToast,
  subscribeToast,
  toErrorMessage,
  type ToastInput,
  type ToastRecord,
  type ToastVariant,
} from '../lib/toastBus';

const DEFAULT_DURATION: Record<ToastVariant, number> = {
  error: 8000,
  success: 4000,
  info: 5000,
};

const VARIANT_STYLES: Record<ToastVariant, string> = {
  error: 'border-danger/40 bg-danger-soft',
  success: 'border-success/40 bg-success-soft',
  info: 'border-border-strong bg-elevated',
};

const VARIANT_ICON: Record<ToastVariant, typeof Info> = {
  error: AlertTriangle,
  success: CheckCircle2,
  info: Info,
};

const VARIANT_ICON_COLOR: Record<ToastVariant, string> = {
  error: 'text-danger',
  success: 'text-success',
  info: 'text-muted',
};

/**
 * Global toast host. Subscribes to the toast bus so toasts can be raised from
 * anywhere — React components (via useToast) or non-React code such as the
 * React Query cache error handlers.
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastRecord[]>([]);

  useEffect(() => {
    return subscribeToast((toast) => {
      setToasts((prev) => [...prev, toast]);
    });
  }, []);

  const dismiss = (id: string) => setToasts((prev) => prev.filter((t) => t.id !== id));

  return (
    <ToastPrimitive.Provider swipeDirection="right">
      {children}
      {toasts.map((toast) => {
        const Icon = VARIANT_ICON[toast.variant];
        return (
          <ToastPrimitive.Root
            key={toast.id}
            duration={toast.duration ?? DEFAULT_DURATION[toast.variant]}
            onOpenChange={(open) => {
              if (!open) dismiss(toast.id);
            }}
            className={cn(
              'flex items-start gap-3 rounded-xl border p-3 pr-2 text-sm shadow-lg shadow-black/20',
              'data-[state=open]:animate-in data-[state=closed]:animate-out data-[swipe=end]:animate-out',
              VARIANT_STYLES[toast.variant],
            )}
          >
            <Icon className={cn('mt-0.5 h-5 w-5 shrink-0', VARIANT_ICON_COLOR[toast.variant])} aria-hidden="true" />
            <div className="min-w-0 flex-1">
              <ToastPrimitive.Title className="font-semibold text-fg">{toast.title}</ToastPrimitive.Title>
              {toast.description ? (
                <ToastPrimitive.Description className="mt-0.5 break-words text-muted">
                  {toast.description}
                </ToastPrimitive.Description>
              ) : null}
            </div>
            <ToastPrimitive.Close
              className="rounded-md p-1 text-faint transition hover:bg-overlay hover:text-fg"
              aria-label="Dismiss"
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </ToastPrimitive.Close>
          </ToastPrimitive.Root>
        );
      })}
      <ToastPrimitive.Viewport className="fixed bottom-0 right-0 z-50 flex w-full max-w-sm flex-col gap-2 p-4 outline-none" />
    </ToastPrimitive.Provider>
  );
}

/** Convenience hook for raising toasts from React components. */
export function useToast() {
  return {
    toast: (input: ToastInput) => emitToast(input),
    error: (title: string, error?: unknown) =>
      emitToast({
        title,
        description: error === undefined ? undefined : toErrorMessage(error),
        variant: 'error',
      }),
    success: (title: string, description?: string) => emitToast({ title, description, variant: 'success' }),
  };
}
