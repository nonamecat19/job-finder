import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { AiFeatureSettingDto, ResumeShapeConfigDto } from '@job-finder/shared';
import { api } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';

// Per-feature "run automatically when a job's match score is high enough"
// toggle and threshold (resume/cover-letter generation, salary inference).
export function useAiFeatureSettings() {
  return useQuery({
    queryKey: queryKeys.aiFeatures.get,
    queryFn: api.settings.getAiFeatures,
    retry: false,
    meta: { silentOn404: true },
  });
}

export function useUpdateAiFeatureSetting() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ feature, enabled, threshold }: AiFeatureSettingDto) =>
      api.settings.putAiFeature(feature, { enabled, threshold }),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.aiFeatures.all }),
  });
}

// Shape of every generated resume: summary length, bullets per job, optional
// sections, project limits and the page target.
export function useResumeShape() {
  return useQuery({
    queryKey: queryKeys.resumeShape.get,
    queryFn: api.settings.getResumeShape,
    retry: false,
    meta: { silentOn404: true },
  });
}

export function useUpdateResumeShape() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ResumeShapeConfigDto) => api.settings.putResumeShape(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.resumeShape.all }),
  });
}

export function useResetResumeShape() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.settings.resetResumeShape(),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.resumeShape.all }),
  });
}
