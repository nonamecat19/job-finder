import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { DocumentType } from '@job-finder/shared';
import { api } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';
import { emitToast, toErrorMessage } from '../../lib/toastBus';

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
    meta: { silentOn404: true },
  });
}

export function useJobDocuments(id: string | undefined) {
  return useQuery({
    queryKey: queryKeys.jobs.documents(id),
    queryFn: () => api.jobs.documents(id!),
    enabled: !!id,
  });
}

export function useJobDocumentStatuses(id: string | undefined, polling: boolean) {
  return useQuery({
    queryKey: queryKeys.jobs.documentStatuses(id),
    queryFn: () => api.jobs.documentStatuses(id!),
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
      if (job.application) {
        if (job.application.status === 'applied') return job.application;
        return api.applications.update(job.application.id, { status: 'applied' });
      }
      await api.jobs.shortlist(jobId!);
      const nextJob = await api.jobs.get(jobId!);
      return api.applications.update(nextJob.application!.id, { status: 'applied' });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.jobs.detail(jobId) });
      emitToast({ variant: 'success', title: 'Marked as applied' });
    },
    onError: (err) => {
      emitToast({ variant: 'error', title: 'Failed to mark as applied', description: toErrorMessage(err) });
    },
  });
}

export function useUnmarkJobApplied(jobId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const job = await api.jobs.get(jobId!);
      return api.applications.update(job.application!.id, { status: 'shortlisted' });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.jobs.detail(jobId) });
      emitToast({ variant: 'success', title: 'Unmarked as applied' });
    },
    onError: (err) => {
      emitToast({ variant: 'error', title: 'Failed to unmark as applied', description: toErrorMessage(err) });
    },
  });
}

export function useMarkJobNotFit(jobId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: () => api.jobs.hide(jobId!),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.jobs.detail(jobId) });
      emitToast({ variant: 'success', title: 'Marked as not fit' });
    },
    onError: (err) => {
      emitToast({ variant: 'error', title: 'Failed to mark as not fit', description: toErrorMessage(err) });
    },
  });
}

export function useUndoJobNotFit(jobId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: () => api.jobs.unhide(jobId!),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.jobs.detail(jobId) });
      emitToast({ variant: 'success', title: 'Undid not fit' });
    },
    onError: (err) => {
      emitToast({ variant: 'error', title: 'Failed to undo not fit', description: toErrorMessage(err) });
    },
  });
}

export function useInterviewPrep(id: string | undefined) {
  return useQuery({
    queryKey: queryKeys.jobs.interviewPrep(id),
    queryFn: () => api.jobs.interviewPrep(id!),
    enabled: !!id,
    retry: false,
    meta: { silentOn404: true },
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
    meta: { silentOn404: true },
  });
}

export function useAssessCoach(jobId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: () => api.coach.assess(jobId!),
    onSuccess: (data) => qc.setQueryData(queryKeys.coach.assessment(jobId), data),
  });
}

export function useJobContacts(jobId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.jobs.contacts(jobId),
    queryFn: () => api.jobs.contacts(jobId!),
    enabled: !!jobId,
  });
}

export function useRefreshJobContacts(jobId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: () => api.jobs.refreshContacts(jobId!),
    onSuccess: (data) => {
      qc.setQueryData(queryKeys.jobs.contacts(jobId), data);
    },
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

export function useRefreshGhostScore(jobId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: () => api.jobs.ghostScore(jobId!),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.jobs.detail(jobId) }),
    onError: (err) => emitToast({ title: 'Ghost re-score failed', description: toErrorMessage(err), variant: 'error' }),
  });
}

export function useOutreachTones(jobId: string | undefined) {
  return useQuery({
    queryKey: queryKeys.outreach.tones(jobId),
    queryFn: () => api.outreach.tones(jobId!),
    enabled: !!jobId,
    staleTime: Infinity,
  });
}

export function useGenerateOutreachDraft(jobId: string | undefined) {
  return useMutation({
    mutationFn: (body: { contactId?: string; tone?: string }) => api.outreach.generate(jobId!, body),
  });
}

export function usePostAgeSignal() {
  return useQuery({
    queryKey: queryKeys.postage.responseRate,
    queryFn: api.postage.responseRate,
    staleTime: 60000,
  });
}

export function useReenrichJob(jobId: string | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.jobs.reEnrich(jobId!),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.jobs.detail(jobId) });
      emitToast({ variant: 'success', title: 'Re-scrape enqueued', description: 'The vacancy will be re-scraped shortly.' });
    },
    onError: (err) => {
      emitToast({ variant: 'error', title: 'Re-scrape failed', description: toErrorMessage(err) });
    },
  });
}
