import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { GenerationRunDto } from '@job-finder/shared';
import { api } from '../../lib/api';

// Same activity-poll interval as `useJobDocumentStatuses`
// (features/job-detail/hooks.ts) — there is no new polling mechanism, per
// contracts/rest-api.md's "Client wiring" note.
const RUN_POLL_INTERVAL_MS = 3000;

export const generations = {
  all: ['generations'] as const,
  get: (id: string | undefined) => ['generations', id] as const,
};

export function useGenerationRun(runId: string | undefined) {
  return useQuery({
    queryKey: generations.get(runId),
    queryFn: () => api.generations.get(runId!),
    enabled: !!runId,
    refetchInterval: (query) => {
      const data = query.state.data as GenerationRunDto | undefined;
      return data?.state === 'running' ? RUN_POLL_INTERVAL_MS : false;
    },
  });
}

export function useStartGenerationRun() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: api.generations.start,
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: generations.get(data.runId) });
      qc.invalidateQueries({ queryKey: generations.all });
    },
  });
}

export function useToggleGenerationItem(runId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (p: { itemId: string; selected?: boolean; position?: number; text?: string }) =>
      api.generations.patchItem(runId!, p.itemId, {
        selected: p.selected,
        position: p.position,
        text: p.text,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: generations.get(runId) }),
  });
}
