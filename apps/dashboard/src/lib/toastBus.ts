export type ToastVariant = 'error' | 'success' | 'info';

export interface ToastInput {
  title: string;
  description?: string;
  variant?: ToastVariant;
  /** Auto-dismiss after this many ms. Defaults per variant. */
  duration?: number;
}

export interface ToastRecord extends ToastInput {
  id: string;
  variant: ToastVariant;
}

type Listener = (toast: ToastRecord) => void;

const listeners = new Set<Listener>();

let counter = 0;
function nextId(): string {
  counter += 1;
  return `toast-${counter}-${Date.now()}`;
}

/** Emit a toast to every subscribed viewport. Safe to call outside React. */
export function emitToast(input: ToastInput): string {
  const record: ToastRecord = {
    id: nextId(),
    variant: input.variant ?? 'info',
    ...input,
  };
  for (const listener of listeners) listener(record);
  return record.id;
}

export function subscribeToast(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Normalise an unknown thrown value into a human-readable message. */
export function toErrorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (typeof error === 'string') return error;
  try {
    return JSON.stringify(error);
  } catch {
    return 'Unknown error';
  }
}
