import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { AutoGenerateSettingDto, LlmTaskSettingDto } from '@job-finder/shared';
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

// Auto-generate resume when a job's match score is very high.
export function useAutoGenerateSettings() {
  return useQuery({
    queryKey: queryKeys.autoGenerate.get,
    queryFn: api.settings.getAutoGenerate,
  });
}

export function useUpdateAutoGenerateSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AutoGenerateSettingDto) => api.settings.putAutoGenerate(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.autoGenerate.all }),
  });
}
