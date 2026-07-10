import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { DocumentType } from '@job-finder/shared';
import { api } from '../../api';
import { queryKeys } from '../../lib/queryKeys';

export function useJobDetail(id: string | undefined) {
  return useQuery({
    queryKey: queryKeys.jobs.detail(id),
    queryFn: () => api.jobs.get(id!),
    enabled: !!id,
  });
}

export function useJobDocuments(id: string | undefined, polling: boolean) {
  return useQuery({
    queryKey: queryKeys.jobs.documents(id),
    queryFn: () => api.jobs.documents(id!),
    enabled: !!id,
    refetchInterval: polling ? 3000 : false,
  });
}

export function useGenerateDocument(jobId: string | undefined, beforeGenerate: (type: DocumentType) => void) {
  return useMutation({
    mutationFn: (type: DocumentType) => {
      beforeGenerate(type);
      return api.jobs.generate(jobId!, type);
    },
  });
}

export function useMarkJobApplied(jobId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const job = await api.jobs.get(jobId!);
      if (job.application) return api.applications.update(job.application.id, { status: 'applied' });
      await api.jobs.shortlist(jobId!);
      const nextJob = await api.jobs.get(jobId!);
      return api.applications.update(nextJob.application!.id, { status: 'applied' });
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.jobs.detail(jobId) }),
  });
}

export function useSaveDocument(jobId: string | undefined, onSaved: () => void) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (p: { id: string; text: string }) => api.documents.update(p.id, p.text),
    onSuccess: () => {
      onSaved();
      qc.invalidateQueries({ queryKey: queryKeys.jobs.documents(jobId) });
    },
  });
}
