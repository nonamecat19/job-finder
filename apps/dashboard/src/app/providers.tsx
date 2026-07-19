import { QueryClientProvider } from '@tanstack/react-query';
import { ReactNode, useState } from 'react';
import { BrowserRouter } from 'react-router-dom';
import { createDashboardQueryClient } from '../lib/queryClient';
import { ToastProvider } from '../components/toast';

export function AppProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(createDashboardQueryClient);

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ToastProvider>{children}</ToastProvider>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
