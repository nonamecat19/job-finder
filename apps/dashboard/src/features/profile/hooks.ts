import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../api';
import { queryKeys } from '../../lib/queryKeys';

export function useProfiles() {
  return useQuery({
    queryKey: queryKeys.profiles.list,
    queryFn: api.profiles.list,
  });
}

export function useCreateProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; document: import('@job-finder/shared').JsonResume; extraNotes?: string }) =>
      api.profiles.create(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.profiles.all }),
  });
}

export function useUpdateProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: { name?: string; document?: import('@job-finder/shared').JsonResume; extraNotes?: string | null } }) =>
      api.profiles.update(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.profiles.all }),
  });
}

export function useDeleteProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.profiles.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.profiles.all }),
  });
}

export function useImportProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => api.profiles.import(file),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.profiles.all }),
  });
}
