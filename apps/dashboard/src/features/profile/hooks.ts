import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';

export function useProfiles() {
  return useQuery({
    queryKey: queryKeys.profiles.list,
    queryFn: api.profiles.list,
  });
}

export function useConfigStatus() {
  return useQuery({
    queryKey: queryKeys.profiles.configStatus,
    queryFn: api.profiles.configStatus,
  });
}

export function useUpdateProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: { name?: string; extraNotes?: string | null } }) =>
      api.profiles.update(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.profiles.all }),
  });
}

export function useDeleteProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.profiles.remove(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.profiles.all });
      qc.invalidateQueries({ queryKey: queryKeys.profiles.configStatus });
    },
  });
}

export function useUploadConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => api.profiles.uploadConfig(file),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.profiles.all });
      qc.invalidateQueries({ queryKey: queryKeys.profiles.configStatus });
    },
  });
}
