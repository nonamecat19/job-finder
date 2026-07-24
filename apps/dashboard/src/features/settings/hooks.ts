import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { AiFeatureSettingDto, LlmTaskSettingDto } from '@job-finder/shared';
import { api } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';

export function useBootstrapExtension() {
  return useMutation({
    mutationFn: () => api.ext.bootstrap(),
  });
}

// Cerebras free-tier model toggle (001-cerebras-model-toggle).
export function useLlmSettings() {
  return useQuery({
    queryKey: queryKeys.llmSettings.get,
    queryFn: api.settings.getLlm,
  });
}

export function useLlmModels() {
  return useQuery({
    queryKey: queryKeys.llmSettings.models,
    queryFn: api.settings.llmModels,
  });
}

export function useUpdateLlmSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (tasks: LlmTaskSettingDto[]) => api.settings.putLlm(tasks),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.llmSettings.all }),
  });
}

// Per-feature "run automatically when a job's match score is high enough"
// toggle and threshold (resume/cover-letter generation, salary inference).
export function useAiFeatureSettings() {
  return useQuery({
    queryKey: queryKeys.aiFeatures.get,
    queryFn: api.settings.getAiFeatures,
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
