import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query';
import { ApiError } from './api';
import { emitToast, toErrorMessage } from './toastBus';

declare module '@tanstack/react-query' {
  interface Register {
    queryMeta: {
      // Set on queries where a 404 means "not configured yet", not a failure.
      silentOn404?: boolean;
    };
  }
}

export function createDashboardQueryClient() {
  return new QueryClient({
    queryCache: new QueryCache({
      onError: (error, query) => {
        // Queries can opt out of the generic toast for a 404 (e.g. "not
        // configured yet" settings) by setting meta.silentOn404, since that
        // status means "expected empty state", not "unexpected failure".
        if (query.meta?.silentOn404 && error instanceof ApiError && error.status === 404) return;
        emitToast({
          title: 'Something went wrong',
          description: toErrorMessage(error),
          variant: 'error',
        });
      },
    }),
    mutationCache: new MutationCache({
      // Skip the global toast when a mutation defines its own onError so we
      // don't double-report; otherwise surface the failure automatically.
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
