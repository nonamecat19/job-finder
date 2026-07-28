import { Toast } from '@heroui/react/toast';
import { useEffect, type ReactNode } from 'react';
import {
  emitToast,
  subscribeToast,
  toErrorMessage,
  type ToastInput,
  type ToastRecord,
} from '../lib/toastBus';

const VARIANT_MAP: Record<string, 'success' | 'warning' | 'danger' | 'default'> = {
  error: 'danger',
  success: 'success',
  info: 'default',
};

export function ToastProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    return subscribeToast((toast: ToastRecord) => {
      Toast.toast(toast.title, {
        description: toast.description,
        variant: VARIANT_MAP[toast.variant] ?? 'default',
        timeout: toast.duration,
      });
    });
  }, []);

  return (
    <>
      {children}
      <Toast.Provider placement="bottom end" maxVisibleToasts={5} />
    </>
  );
}

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
