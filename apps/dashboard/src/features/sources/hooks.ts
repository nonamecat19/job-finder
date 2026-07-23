import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';

export function useSources() {
  return useQuery({
    queryKey: queryKeys.sources.list,
    queryFn: api.sources.list,
  });
}

export function useUpdateSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ key, body }: { key: string; body: { enabled?: boolean; config?: Record<string, unknown> } }) =>
      api.sources.update(key, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.sources.all }),
  });
}

export function useTestSource() {
  return useMutation({
    mutationFn: (key: string) => api.sources.test(key),
  });
}

export function useSearches() {
  return useQuery({
    queryKey: queryKeys.searches.list,
    queryFn: api.searches.list,
  });
}

export function useCreateSearch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; query: import('@job-finder/shared').SearchQuery; cron?: string; enabled?: boolean }) =>
      api.searches.create(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.searches.all }),
  });
}

export function useUpdateSearch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<{ name: string; query: import('@job-finder/shared').SearchQuery; cron: string; enabled: boolean }> }) =>
      api.searches.update(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.searches.all }),
  });
}

export function useDeleteSearch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.searches.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.searches.all }),
  });
}

export function useRunSearch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.searches.run(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.searches.runs }),
  });
}

export function useRecentRuns() {
  return useQuery({
    queryKey: queryKeys.searches.runs,
    queryFn: api.searches.recentRuns,
  });
}

export function useSubscriptions(sourceKey?: string) {
  return useQuery({
    queryKey: [...queryKeys.subscriptions.list, sourceKey],
    queryFn: () => api.subscriptions.list(sourceKey),
  });
}

export function useCreateSubscription() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: import('@job-finder/shared').SubscriptionInput) => api.subscriptions.create(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.subscriptions.all }),
  });
}

export function useDeleteSubscription() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.subscriptions.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.subscriptions.all }),
  });
}

export function useRunSubscription() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.subscriptions.run(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.subscriptions.all }),
  });
}

export function useRunAllSubscriptions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.subscriptions.runAll(),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.subscriptions.all }),
  });
}
