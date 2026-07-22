import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../api';
import { queryKeys } from '../../lib/queryKeys';

export function useContacts() {
  return useQuery({
    queryKey: queryKeys.contacts.all,
    queryFn: api.contacts.list,
  });
}

export function useImportContactsCSV() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => api.contacts.import(file),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.contacts.all }),
  });
}

export function useGithubSync() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (contactId: string) => api.contacts.githubSync(contactId),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.contacts.all }),
  });
}
