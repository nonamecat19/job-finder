import { useQuery } from '@tanstack/react-query';
import { api } from '../../api';
import { queryKeys } from '../../lib/queryKeys';

export function useActivity(limit?: number) {
  return useQuery({
    queryKey: queryKeys.activity.list(limit),
    queryFn: () => api.activity.list(limit),
    refetchInterval: 2000,
  });
}
