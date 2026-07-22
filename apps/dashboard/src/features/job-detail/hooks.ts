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

export function useJobKeywordDiff(id: string | undefined) {
  return useQuery({
    queryKey: queryKeys.jobs.keywordDiff(id),
    queryFn: () => api.jobs.keywordDiff(id!),
    enabled: !!id,
    retry: false,
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

export function useInterviewPrep(id: string | undefined) {
  return useQuery({
    queryKey: queryKeys.jobs.interviewPrep(id),
    queryFn: () => api.jobs.interviewPrep(id!),
    enabled: !!id,
    retry: false,
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

export function useCoachAssessment(jobId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.coach.assessment(jobId),
    queryFn: () => api.coach.assessment(jobId!),
    enabled: !!jobId,
    retry: false,
  });
}

export function useAssessCoach(jobId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: () => api.coach.assess(jobId!),
    onSuccess: (data) => qc.setQueryData(queryKeys.coach.assessment(jobId), data),
  });
}

export function useCompanyIntel(jobId: string) {
  return useQuery({
    queryKey: queryKeys.companies.detail(jobId),
    queryFn: () => api.companies.intel(jobId),
  });
}

export function useRefreshCompanyIntel(jobId: string) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: () => api.companies.refresh(jobId),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.companies.detail(jobId) }),
  });
}
