import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query';
import { emitToast, toErrorMessage } from './toastBus';

export function createDashboardQueryClient() {
  return new QueryClient({
    queryCache: new QueryCache({
      onError: (error) => {
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
