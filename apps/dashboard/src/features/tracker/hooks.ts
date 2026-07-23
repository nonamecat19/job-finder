import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';

export function useApplications(status?: string) {
  return useQuery({
    queryKey: queryKeys.applications.list(status),
    queryFn: () => api.applications.list(status),
  });
}

export function useUpdateApplication() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: { status?: string; notes?: string | null } }) =>
      api.applications.update(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.applications.all }),
  });
}

export function useStats() {
  return useQuery({
    queryKey: ['stats'] as const,
    queryFn: api.stats,
  });
}
