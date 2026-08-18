export type ToastVariant = 'error' | 'success' | 'info';

export interface ToastInput {
  title: string;
  description?: string;
  variant?: ToastVariant;
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

export function toErrorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (typeof error === 'string') return error;
  try {
    return JSON.stringify(error);
  } catch {
    return 'Unknown error';
  }
}
