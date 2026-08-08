import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';

// 034: the summary-model menu. One query serves the options and the current
// choice, because a menu fetched separately from the selection can disagree
// with it about which options exist.
export function useSummaryModel() {
  return useQuery({
    queryKey: ['settings', 'summary-model'],
    queryFn: () => api.settings.getSummaryModel(),
  });
}

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

export function useGenerateCoverLetter() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (resumeId: string) => api.documents.coverLetter(resumeId),
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
