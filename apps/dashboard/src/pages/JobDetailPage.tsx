import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import type { DocumentType, GeneratedDocumentDto } from '@job-finder/shared';
import { api } from '../api';
import { Button, Chip, ScoreBadge, Spinner } from '../components/ui';

export default function JobDetailPage() {
  const { id } = useParams<{ id: string }>();
  const qc = useQueryClient();
  const [generating, setGenerating] = useState<DocumentType | null>(null);
  const [editingDoc, setEditingDoc] = useState<{ id: string; text: string } | null>(null);

  const { data: job, isLoading } = useQuery({
    queryKey: ['job', id],
    queryFn: () => api.jobs.get(id!),
    enabled: !!id,
  });

  // poll documents while generation is queued (local LLM = tens of seconds)
  const { data: documents } = useQuery({
    queryKey: ['documents', id],
    queryFn: () => api.jobs.documents(id!),
    enabled: !!id,
    refetchInterval: generating ? 3000 : false,
  });

  const docCountOfType = (type: DocumentType) => (documents ?? []).filter((d) => d.type === type).length;
  const [countAtGenerate, setCountAtGenerate] = useState(0);
  if (generating && documents && docCountOfType(generating) > countAtGenerate) {
    setGenerating(null); // new version arrived
  }

  const generate = useMutation({
    mutationFn: (type: DocumentType) => {
      setCountAtGenerate(docCountOfType(type));
      return api.jobs.generate(id!, type);
    },
    onSuccess: (_res, type) => setGenerating(type),
  });

  const markApplied = useMutation({
    mutationFn: async () => {
      const j = await api.jobs.get(id!);
      if (j.application) return api.applications.update(j.application.id, { status: 'applied' });
      await api.jobs.shortlist(id!);
      const j2 = await api.jobs.get(id!);
      return api.applications.update(j2.application!.id, { status: 'applied' });
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['job', id] }),
  });

  const saveLetter = useMutation({
    mutationFn: (p: { id: string; text: string }) => api.documents.update(p.id, p.text),
    onSuccess: () => {
      setEditingDoc(null);
      qc.invalidateQueries({ queryKey: ['documents', id] });
    },
  });

  if (isLoading || !job) return <Spinner label="loading job…" />;
  const mr = job.matchResult;

  return (
    <div className="space-y-6">
      <div>
        <Link to="/" className="text-sm text-slate-500 hover:underline">
          ← back to feed
        </Link>
        <div className="mt-2 flex items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-bold">{job.title}</h1>
            <p className="text-slate-600">
              {job.company}
              {job.location ? ` · ${job.location}` : ''}
              {job.remote ? ' · remote' : ''}
              {job.salaryRaw ? ` · ${job.salaryRaw}` : ''}
            </p>
            <div className="mt-1 flex gap-1">
              <Chip>{job.sourceKey}</Chip>
              <Chip>{job.status}</Chip>
            </div>
          </div>
          <div className="flex shrink-0 gap-2">
            <a href={job.url} target="_blank" rel="noreferrer">
              <Button variant="secondary">open posting ↗</Button>
            </a>
            <Button onClick={() => markApplied.mutate()}>mark applied</Button>
          </div>
        </div>
      </div>

      {mr && (
        <section className="rounded-lg border border-slate-200 bg-white p-4">
          <h2 className="mb-2 flex items-center gap-2 font-semibold">
            Fit <ScoreBadge score={mr.score} />
            <span className="text-xs font-normal text-slate-400">
              similarity {Number(mr.similarity).toFixed(2)} · {mr.model}
            </span>
          </h2>
          {mr.summary && <p className="mb-2 text-sm text-slate-700">{mr.summary}</p>}
          <div className="flex flex-wrap gap-1">
            {(mr.matchedSkills ?? []).map((s: string) => (
              <Chip key={s} tone="green">
                {s}
              </Chip>
            ))}
            {(mr.missingSkills ?? []).map((s: string) => (
              <Chip key={s} tone="red">
                {s}
              </Chip>
            ))}
          </div>
          {(mr.redFlags ?? []).length > 0 && (
            <ul className="mt-2 list-inside list-disc text-sm text-rose-700">
              {(mr.redFlags as string[]).map((f) => (
                <li key={f}>{f}</li>
              ))}
            </ul>
          )}
        </section>
      )}

      <section className="rounded-lg border border-slate-200 bg-white p-4">
        <h2 className="mb-2 font-semibold">Documents</h2>
        <div className="mb-3 flex gap-2">
          <Button disabled={!!generating} onClick={() => generate.mutate('resume')}>
            Generate resume
          </Button>
          <Button disabled={!!generating} onClick={() => generate.mutate('cover_letter')}>
            Generate cover letter
          </Button>
          {generating && <Spinner label={`generating ${generating.replace('_', ' ')}… (local LLM, be patient)`} />}
        </div>
        <ul className="space-y-2">
          {(documents ?? []).map((doc: GeneratedDocumentDto) => (
            <li key={doc.id} className="rounded border border-slate-200 p-2 text-sm">
              <div className="flex items-center justify-between">
                <span>
                  <b>{doc.type === 'resume' ? 'Resume' : 'Cover letter'}</b> v{doc.version}
                  <span className="ml-2 text-xs text-slate-400">
                    {new Date(doc.createdAt).toLocaleString()} · {doc.model}
                  </span>
                </span>
                <span className="flex gap-2">
                  {doc.type === 'cover_letter' && (
                    <Button
                      variant="ghost"
                      onClick={() =>
                        setEditingDoc({ id: doc.id, text: (doc.content as { text?: string })?.text ?? '' })
                      }
                    >
                      edit
                    </Button>
                  )}
                  {doc.pdfPath && (
                    <a href={api.documents.pdfUrl(doc.id)}>
                      <Button variant="secondary">PDF ↓</Button>
                    </a>
                  )}
                </span>
              </div>
              {doc.type === 'cover_letter' && editingDoc?.id !== doc.id && (
                <p className="mt-1 whitespace-pre-wrap text-slate-600">
                  {(doc.content as { text?: string })?.text}
                </p>
              )}
              {editingDoc?.id === doc.id && (
                <div className="mt-2">
                  <textarea
                    className="h-40 w-full rounded border border-slate-300 p-2 text-sm"
                    value={editingDoc.text}
                    onChange={(e) => setEditingDoc({ ...editingDoc, text: e.target.value })}
                  />
                  <div className="mt-1 flex gap-2">
                    <Button onClick={() => saveLetter.mutate(editingDoc)}>save & re-render PDF</Button>
                    <Button variant="ghost" onClick={() => setEditingDoc(null)}>
                      cancel
                    </Button>
                  </div>
                </div>
              )}
            </li>
          ))}
        </ul>
      </section>

      <section className="rounded-lg border border-slate-200 bg-white p-4">
        <h2 className="mb-2 font-semibold">Job description</h2>
        <p className="whitespace-pre-wrap text-sm text-slate-700">{job.description}</p>
      </section>
    </div>
  );
}
