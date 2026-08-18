import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { GenerationItemDto, GenerationRunDto } from '@job-finder/shared';
import { api } from '../../lib/api';

const RUN_POLL_INTERVAL_MS = 3000;

export const generations = {
  all: ['generations'] as const,
  get: (id: string | undefined) => ['generations', id] as const,
};

export function useSummaryModel() {
  return useQuery({
    queryKey: ['settings', 'summary-model'],
    queryFn: () => api.settings.getSummaryModel(),
  });
}

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

export function useLatestGenerationRunForJob(jobId: string | undefined) {
  return useQuery({
    queryKey: [...generations.all, 'list', jobId, 1] as const,
    queryFn: () => api.generations.list({ jobId, limit: 1 }),
    enabled: !!jobId,
    select: (runs) => runs[0],
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

export function useRerunGenerationRun(runId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (sections?: string[]) => api.generations.rerun(runId!, sections),
    onSettled: () => qc.invalidateQueries({ queryKey: generations.get(runId) }),
  });
}

export function useExportGenerationRun(runId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: () => api.generations.export(runId!),
    onSettled: () => qc.invalidateQueries({ queryKey: generations.get(runId) }),
  });
}

export function useRewriteGenerationItem(runId: string | undefined) {
  return useMutation({
    mutationFn: (itemId: string) => api.generations.rewriteItem(runId!, itemId),
  });
}

function applyItemPatch(
  run: GenerationRunDto,
  itemId: string,
  patch: Partial<Pick<GenerationItemDto, 'selected' | 'position' | 'text' | 'edited'>>,
): GenerationRunDto {
  return {
    ...run,
    sections: run.sections.map((section) => ({
      ...section,
      items: section.items.map((item) => (item.id === itemId ? { ...item, ...patch } : item)),
    })),
  };
}

function applyDroppedEntries(run: GenerationRunDto, itemId: string, droppedEntries: string[]): GenerationRunDto {
  const dropped = new Set(droppedEntries);
  return {
    ...run,
    sections: run.sections.map((section) => ({
      ...section,
      items: section.items.map((item) =>
        item.id === itemId && item.skillEntries
          ? { ...item, skillEntries: item.skillEntries.map((e) => ({ ...e, selected: !dropped.has(e.text) })) }
          : item,
      ),
    })),
  };
}

function applyReorder(run: GenerationRunDto, sectionId: string, orderedItemIds: string[]): GenerationRunDto {
  const positionById = new Map(orderedItemIds.map((id, i) => [id, i]));
  return {
    ...run,
    sections: run.sections.map((section) => {
      if (section.id !== sectionId) return section;
      const items = section.items
        .map((item) => (positionById.has(item.id) ? { ...item, position: positionById.get(item.id)! } : item))
        .sort((a, b) => a.position - b.position);
      return { ...section, items };
    }),
  };
}

export function useToggleGenerationItem(runId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (p: { itemId: string; selected?: boolean; position?: number; text?: string; droppedEntries?: string[] }) =>
      api.generations.patchItem(runId!, p.itemId, {
        selected: p.selected,
        position: p.position,
        text: p.text,
        droppedEntries: p.droppedEntries,
      }),
    onMutate: async (p) => {
      if (!runId) return undefined;
      await qc.cancelQueries({ queryKey: generations.get(runId) });
      const previous = qc.getQueryData<GenerationRunDto>(generations.get(runId));
      if (previous) {
        const patched = applyItemPatch(previous, p.itemId, {
          ...(p.selected !== undefined ? { selected: p.selected } : {}),
          ...(p.position !== undefined ? { position: p.position } : {}),
          ...(p.text !== undefined ? { text: p.text, edited: true } : {}),
        });
        qc.setQueryData<GenerationRunDto>(
          generations.get(runId),
          p.droppedEntries ? applyDroppedEntries(patched, p.itemId, p.droppedEntries) : patched,
        );
      }
      return { previous };
    },
    onError: (_err, _p, context) => {
      if (runId && context?.previous) qc.setQueryData(generations.get(runId), context.previous);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: generations.get(runId) }),
  });
}

export function useReorderGenerationSection(runId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (p: { sectionId: string; itemIds: string[] }) =>
      api.generations.reorder(runId!, p.sectionId, p.itemIds),
    onMutate: async (p) => {
      if (!runId) return undefined;
      await qc.cancelQueries({ queryKey: generations.get(runId) });
      const previous = qc.getQueryData<GenerationRunDto>(generations.get(runId));
      if (previous) {
        qc.setQueryData<GenerationRunDto>(generations.get(runId), applyReorder(previous, p.sectionId, p.itemIds));
      }
      return { previous };
    },
    onError: (_err, _p, context) => {
      if (runId && context?.previous) qc.setQueryData(generations.get(runId), context.previous);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: generations.get(runId) }),
  });
}

function applySectionEnabled(run: GenerationRunDto, sectionId: string, enabled: boolean): GenerationRunDto {
  return {
    ...run,
    sections: run.sections.map((section) => (section.id === sectionId ? { ...section, enabled } : section)),
  };
}

export function useSetSectionEnabled(runId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (p: { sectionId: string; enabled: boolean }) =>
      api.generations.setSectionEnabled(runId!, p.sectionId, p.enabled),
    onMutate: async (p) => {
      if (!runId) return undefined;
      await qc.cancelQueries({ queryKey: generations.get(runId) });
      const previous = qc.getQueryData<GenerationRunDto>(generations.get(runId));
      if (previous) {
        qc.setQueryData<GenerationRunDto>(generations.get(runId), applySectionEnabled(previous, p.sectionId, p.enabled));
      }
      return { previous };
    },
    onError: (_err, _p, context) => {
      if (runId && context?.previous) qc.setQueryData(generations.get(runId), context.previous);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: generations.get(runId) }),
  });
}
