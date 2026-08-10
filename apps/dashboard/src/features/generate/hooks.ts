import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { GenerationItemDto, GenerationRunDto } from '@job-finder/shared';
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

// T072: the export is a plain mutation, not a poll loop — the POST either
// comes back with the finished document or with the overflow report, and the
// run query is invalidated so the workspace picks up the new export state.
export function useExportGenerationRun(runId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: () => api.generations.export(runId!),
    onSettled: () => qc.invalidateQueries({ queryKey: generations.get(runId) }),
  });
}

// applyItemPatch returns a new run with one item's fields updated — the pure
// transform both the optimistic toggle and the optimistic reorder build on.
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

// applyReorder returns a new run with one section's items given fresh
// `position` values from `orderedItemIds`, re-sorted to match — the client
// never waits on the network to show the new order (SC-006).
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

// T033: toggle / edit an item optimistically — the checkbox and text reflect
// the change immediately, and the PATCH persists in the background. This is
// what makes SC-006 (<1s, zero model calls) true by construction: the UI
// never waits on the request to show the result.
export function useToggleGenerationItem(runId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (p: { itemId: string; selected?: boolean; position?: number; text?: string }) =>
      api.generations.patchItem(runId!, p.itemId, {
        selected: p.selected,
        position: p.position,
        text: p.text,
      }),
    onMutate: async (p) => {
      if (!runId) return undefined;
      await qc.cancelQueries({ queryKey: generations.get(runId) });
      const previous = qc.getQueryData<GenerationRunDto>(generations.get(runId));
      if (previous) {
        qc.setQueryData<GenerationRunDto>(
          generations.get(runId),
          applyItemPatch(previous, p.itemId, {
            ...(p.selected !== undefined ? { selected: p.selected } : {}),
            ...(p.position !== undefined ? { position: p.position } : {}),
            ...(p.text !== undefined ? { text: p.text, edited: true } : {}),
          }),
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

// T033's reorder half: a whole-section drag-drop, applied locally before the
// PATCH resolves.
export function useReorderGenerationSection(runId: string | undefined) {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (p: { sectionId: string; itemIds: string[] }) => api.generations.reorder(runId!, p.sectionId, p.itemIds),
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
