import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';

export function useAdHocDocuments() {
  return useQuery({
    queryKey: queryKeys.documents.adHoc,
    queryFn: () => api.documents.listAdHoc(),
  });
}

export function useTailorDocuments() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: api.documents.tailor,
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.documents.adHoc }),
  });
}

export function useSaveAdHocDocument(onSaved: () => void) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (p: { id: string; text: string }) => api.documents.update(p.id, p.text),
    onSuccess: () => {
      onSaved();
      qc.invalidateQueries({ queryKey: queryKeys.documents.adHoc });
    },
  });
}
