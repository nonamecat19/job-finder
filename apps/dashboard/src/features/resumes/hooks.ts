import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { GenerationRunDto } from '@job-finder/shared';
import { api } from '../../lib/api';
import { generations } from '../generate/hooks';

const RUN_POLL_INTERVAL_MS = 3000;

const DEFAULT_LIMIT = 25;

export const generationRunsKey = (limit: number) => [...generations.all, 'list', limit] as const;

export function useGenerationRuns(limit: number = DEFAULT_LIMIT) {
  return useQuery({
    queryKey: generationRunsKey(limit),
    queryFn: () => api.generations.list({ limit }),
    refetchInterval: (query) => {
      const data = query.state.data as GenerationRunDto[] | undefined;
      return data?.some((run) => run.state === 'running') ? RUN_POLL_INTERVAL_MS : false;
    },
  });
}

export function useDeleteGenerationRun() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (runId: string) => api.generations.remove(runId),
    onSettled: () => qc.invalidateQueries({ queryKey: generations.all }),
  });
}
