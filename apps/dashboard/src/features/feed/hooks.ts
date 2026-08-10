import { keepPreviousData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type JobFilters } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';

export function useInfiniteJobs(filters: Omit<JobFilters, 'page'>) {
  return useInfiniteQuery({
    queryKey: queryKeys.jobs.list(filters),
    queryFn: ({ pageParam }) => api.jobs.list({ ...filters, page: pageParam }),
    initialPageParam: 1,
    getNextPageParam: (lastPage) =>
      lastPage.page * lastPage.pageSize < lastPage.total ? lastPage.page + 1 : undefined,
    placeholderData: keepPreviousData,
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

export function useAddVacancyByUrl() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (url: string) => api.jobs.addManual(url),
    onSuccess: (result) => {
      if (result.outcome === 'created') {
        qc.invalidateQueries({ queryKey: queryKeys.jobs.all });
      }
    },
  });
}

export function useSaveManualVacancy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: api.jobs.saveManual,
    onSuccess: (result) => {
      if (result.outcome === 'created') {
        qc.invalidateQueries({ queryKey: queryKeys.jobs.all });
      }
    },
  });
}
