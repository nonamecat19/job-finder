import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { LlmTaskSettingDto } from '@job-finder/shared';
import { api } from '../../api';
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
