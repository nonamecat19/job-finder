import { QueryClientProvider } from '@tanstack/react-query';
import { ReactNode, useState } from 'react';
import { BrowserRouter } from 'react-router-dom';
import { createDashboardQueryClient } from '../lib/queryClient';

export function AppProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(createDashboardQueryClient);

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>{children}</BrowserRouter>
    </QueryClientProvider>
  );
}
