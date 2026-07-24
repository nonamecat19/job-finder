import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type JobFilters } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';

export function useJobs(filters: JobFilters) {
  return useQuery({
    queryKey: queryKeys.jobs.list(filters),
    queryFn: () => api.jobs.list(filters),
  });
}

export function useFeedSources() {
  return useQuery({
    queryKey: queryKeys.sources.list,
    queryFn: api.sources.list,
  });
}

export function useFeedSubscriptions() {
  return useQuery({
    queryKey: queryKeys.subscriptions.list,
    queryFn: () => api.subscriptions.list(),
  });
}

export function useShortlistJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.jobs.shortlist(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.jobs.all }),
  });
}

export function useHideJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.jobs.hide(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.jobs.all }),
  });
}

export function useClearJobs() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.jobs.clear(),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.jobs.all }),
  });
}
