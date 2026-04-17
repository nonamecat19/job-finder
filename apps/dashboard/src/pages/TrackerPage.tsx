import { DndContext, DragEndEvent, useDraggable, useDroppable } from '@dnd-kit/core';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import type { ApplicationDto } from '@job-finder/shared';
import { api } from '../api';
import { Spinner } from '../components/ui';

const COLUMNS = ['shortlisted', 'docs_generated', 'applied', 'interview', 'offer', 'rejected'] as const;

const COLUMN_LABELS: Record<string, string> = {
  shortlisted: 'Shortlisted',
  docs_generated: 'Docs ready',
  applied: 'Applied',
  interview: 'Interview',
  offer: 'Offer',
  rejected: 'Rejected',
};

function daysIn(app: ApplicationDto): number {
  const last = app.events.length ? app.events[app.events.length - 1].at : app.updatedAt;
  return Math.floor((Date.now() - new Date(last).getTime()) / 86_400_000);
}

function Card({ app }: { app: ApplicationDto }) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({ id: app.id });
  const style = transform
    ? { transform: `translate(${transform.x}px, ${transform.y}px)`, zIndex: 50 }
    : undefined;
  return (
    <div
      ref={setNodeRef}
      style={style}
      {...listeners}
      {...attributes}
      className={`cursor-grab rounded border border-slate-200 bg-white p-2 text-sm shadow-sm ${
        isDragging ? 'opacity-70 shadow-lg' : ''
      }`}
    >
      <div className="font-medium">{app.job?.company}</div>
      <Link
        to={`/jobs/${app.jobId}`}
        className="block truncate text-xs text-sky-700 hover:underline"
        onPointerDown={(e) => e.stopPropagation()}
      >
        {app.job?.title}
      </Link>
      <div className="mt-1 flex items-center gap-2 text-xs text-slate-400">
        <span>{daysIn(app)}d</span>
        {app.notes && <span title={app.notes}>📝</span>}
        {app.job?.matchResult?.score != null && <span>fit {app.job.matchResult.score}</span>}
      </div>
    </div>
  );
}

function Column({ status, apps }: { status: string; apps: ApplicationDto[] }) {
  const { setNodeRef, isOver } = useDroppable({ id: status });
  return (
    <div
      ref={setNodeRef}
      className={`flex min-h-64 w-52 shrink-0 flex-col gap-2 rounded-lg border p-2 ${
        isOver ? 'border-sky-400 bg-sky-50' : 'border-slate-200 bg-slate-100/60'
      }`}
    >
      <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">
        {COLUMN_LABELS[status]} <span className="text-slate-400">({apps.length})</span>
      </div>
      {apps.map((a) => (
        <Card key={a.id} app={a} />
      ))}
    </div>
  );
}

export default function TrackerPage() {
  const qc = useQueryClient();
  const { data: apps, isLoading } = useQuery({ queryKey: ['applications'], queryFn: () => api.applications.list() });

  const move = useMutation({
    mutationFn: (p: { id: string; status: string }) => api.applications.update(p.id, { status: p.status }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['applications'] }),
  });

  const onDragEnd = (e: DragEndEvent) => {
    const appId = String(e.active.id);
    const target = e.over?.id ? String(e.over.id) : null;
    if (!target) return;
    const app = apps?.find((a) => a.id === appId);
    if (app && app.status !== target) move.mutate({ id: appId, status: target });
  };

  if (isLoading) return <Spinner label="loading tracker…" />;

  return (
    <div>
      <h1 className="mb-4 text-xl font-bold">Application tracker</h1>
      <DndContext onDragEnd={onDragEnd}>
        <div className="flex gap-3 overflow-x-auto pb-4">
          {COLUMNS.map((status) => (
            <Column key={status} status={status} apps={(apps ?? []).filter((a) => a.status === status)} />
          ))}
        </div>
      </DndContext>
    </div>
  );
}
