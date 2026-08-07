import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query';
import { ApiError } from './api';
import { emitToast, toErrorMessage } from './toastBus';

declare module '@tanstack/react-query' {
  interface Register {
    queryMeta: {
      silentOn404?: boolean;
    };
  }
}

export function createDashboardQueryClient() {
  return new QueryClient({
    queryCache: new QueryCache({
      onError: (error, query) => {
        if (query.meta?.silentOn404 && error instanceof ApiError && error.status === 404) return;
        emitToast({
          title: 'Something went wrong',
          description: toErrorMessage(error),
          variant: 'error',
        });
      },
    }),
    mutationCache: new MutationCache({
      onError: (error, _vars, _ctx, mutation) => {
        if (mutation.options.onError) return;
        emitToast({
          title: 'Action failed',
          description: toErrorMessage(error),
          variant: 'error',
        });
      },
    }),
    defaultOptions: {
      queries: {
        retry: 1,
        refetchOnWindowFocus: false,
      },
      mutations: {
        retry: false,
      },
    },
  });
}
