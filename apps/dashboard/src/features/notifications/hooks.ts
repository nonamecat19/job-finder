import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';

export function useUnseenNotificationCount() {
  return useQuery({
    queryKey: queryKeys.notifications.unseenCount,
    queryFn: api.notifications.unseenCount,
    refetchInterval: 30000,
  });
}

export function useNotifications(enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.notifications.list,
    queryFn: api.notifications.list,
    enabled,
  });
}

export function useMarkNotificationSeen() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.notifications.markSeen(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.notifications.all });
    },
  });
}
