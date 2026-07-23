import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../api';
import { queryKeys } from '../../lib/queryKeys';

export function useActivity(limit?: number) {
  return useQuery({
    queryKey: queryKeys.activity.list(limit),
    queryFn: () => api.activity.list(limit),
    refetchInterval: 2000,
  });
}

export function useRetryActivity() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (op?: string) => api.activity.retry(op),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.activity.all }),
  });
}
