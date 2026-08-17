import { QueryClientProvider } from '@tanstack/react-query';
import { ReactNode, useState } from 'react';
import { BrowserRouter } from 'react-router-dom';
import { createDashboardQueryClient } from '../lib/queryClient';
import { ToastProvider } from '../components/toast';
import { ThemeProvider } from '../lib/theme';

export function AppProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(createDashboardQueryClient);

  return (
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <ToastProvider>{children}</ToastProvider>
        </BrowserRouter>
      </QueryClientProvider>
    </ThemeProvider>
  );
}
