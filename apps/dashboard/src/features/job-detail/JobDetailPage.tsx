import { ArrowLeft, ExternalLink, FileDown } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import type { DocumentType, GeneratedDocumentDto, JobDto } from '@job-finder/shared';
import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import { Button, Chip, ScoreBadge, Spinner, Surface, Textarea } from '../../components/ui';
import { api } from '../../api';
import {
  useGenerateDocument,
  useJobDetail,
  useJobDocuments,
  useMarkJobApplied,
  useSaveDocument,
} from './hooks';
import DOMPurify from 'dompurify';

type DetailedJob = JobDto & { documents: GeneratedDocumentDto[] };

export default function JobDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [generating, setGenerating] = useState<DocumentType | null>(null);
  const [countAtGenerate, setCountAtGenerate] = useState(0);
  const [editingDoc, setEditingDoc] = useState<{ id: string; text: string } | null>(null);

  const { data: job, isLoading } = useJobDetail(id);
  const { data: documents } = useJobDocuments(id, !!generating);
  const docCountOfType = (type: DocumentType) => (documents ?? []).filter((d) => d.type === type).length;

  const generate = useGenerateDocument(id, (type) => {
    setCountAtGenerate(docCountOfType(type));
  });
  const resumeDoc = useMemo(() => {
    const resumes = (documents ?? []).filter((d) => d.type === 'resume' && d.pdfPath);
    return resumes.length ? resumes.reduce((a, b) => (b.version > a.version ? b : a)) : null;
  }, [documents]);
  const markApplied = useMarkJobApplied(id);
  const saveLetter = useSaveDocument(id, () => setEditingDoc(null));

  useEffect(() => {
    if (generate.isSuccess && generate.variables && generating !== generate.variables) {
      setGenerating(generate.variables);
      return;
    }
    if (generating && documents && docCountOfType(generating) > countAtGenerate) {
      setGenerating(null);
    }
  }, [generate.isSuccess, generate.variables, generating, documents, countAtGenerate]);

  if (isLoading || !job) return <Spinner label="loading job…" />;

  return (
    <div className="space-y-5">
      <Link to="/" className="inline-flex items-center gap-1 text-sm font-medium text-muted hover:text-fg">
        <ArrowLeft className="h-4 w-4" aria-hidden="true" />
        back to feed
      </Link>

      <PageHeader
        title={job.title}
        description={<JobMeta job={job} />}
        actions={
          <>
            <a href={job.url} target="_blank" rel="noreferrer">
              <Button variant="secondary">
                open posting <ExternalLink className="h-4 w-4" aria-hidden="true" />
              </Button>
            </a>
            <Button onClick={() => markApplied.mutate()} disabled={markApplied.isPending}>
              mark applied
            </Button>
          </>
        }
      />

      {job.matchResult ? <FitSummary job={job} /> : null}

      <DocumentsPanel
        documents={documents ?? []}
        generating={generating}
        editingDoc={editingDoc}
        onGenerate={(type) => generate.mutate(type)}
        onEdit={setEditingDoc}
        onCancelEdit={() => setEditingDoc(null)}
        onSave={(doc) => saveLetter.mutate(doc)}
      />

      <div className={resumeDoc ? 'grid gap-5 lg:grid-cols-2 lg:items-start' : ''}>
        <Surface>
          <SectionTitle>Job description</SectionTitle>
          {job.descriptionHtml ? (
            <div
              className="prose prose-sm max-w-none text-muted [&_a]:text-accent [&_a:hover]:underline [&_h1]:text-base [&_h1]:font-semibold [&_h2]:text-sm [&_h2]:font-semibold [&_h3]:text-sm [&_h3]:font-semibold [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5 [&_li]:my-1 [&_p]:my-2 [&_strong]:font-semibold"
              dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(job.descriptionHtml) }}
            />
          ) : (
            <p className="whitespace-pre-wrap text-sm leading-6 text-muted">{job.description}</p>
          )}
        </Surface>
        {resumeDoc ? <ResumePreview doc={resumeDoc} /> : null}
      </div>
    </div>
  );
}

function ResumePreview({ doc }: { doc: GeneratedDocumentDto }) {
  return (
    <Surface>
      <div className="mb-3 flex items-center justify-between gap-2">
        <SectionTitle className="mb-0">Resume</SectionTitle>
        <a href={api.documents.pdfUrl(doc.id)} target="_blank" rel="noreferrer">
          <Button variant="secondary">
            open PDF <FileDown className="h-4 w-4" aria-hidden="true" />
          </Button>
        </a>
      </div>
      <iframe
        title="Resume preview"
        src={api.documents.pdfUrl(doc.id)}
        className="h-[75vh] w-full rounded-md border border-border bg-white"
      />
    </Surface>
  );
}

function JobMeta({ job }: { job: DetailedJob }) {
  return (
    <>
      {job.company}
      {job.location ? ` · ${job.location}` : ''}
      {job.remote ? ' · remote' : ''}
      {job.salaryRaw ? ` · ${job.salaryRaw}` : ''}
      <span className="mt-2 flex gap-1">
        <Chip>{job.sourceKey}</Chip>
        <Chip>{job.status}</Chip>
      </span>
    </>
  );
}

function FitSummary({ job }: { job: DetailedJob }) {
  const mr = job.matchResult!;

  return (
    <Surface>
      <h2 className="mb-2 flex flex-wrap items-center gap-2 font-semibold">
        Fit <ScoreBadge score={mr.score} />
        <span className="text-xs font-normal text-faint">
          similarity {Number(mr.similarity).toFixed(2)} · {mr.model}
        </span>
      </h2>
      {mr.summary ? <p className="mb-2 text-sm leading-6 text-muted">{mr.summary}</p> : null}
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
      {(mr.redFlags ?? []).length > 0 ? (
        <ul className="mt-3 list-inside list-disc text-sm text-danger">
          {(mr.redFlags as string[]).map((f) => (
            <li key={f}>{f}</li>
          ))}
        </ul>
      ) : null}
    </Surface>
  );
}

function DocumentsPanel({
  documents,
  generating,
  editingDoc,
  onGenerate,
  onEdit,
  onCancelEdit,
  onSave,
}: {
  documents: GeneratedDocumentDto[];
  generating: DocumentType | null;
  editingDoc: { id: string; text: string } | null;
  onGenerate: (type: DocumentType) => void;
  onEdit: (doc: { id: string; text: string }) => void;
  onCancelEdit: () => void;
  onSave: (doc: { id: string; text: string }) => void;
}) {
  return (
    <Surface>
      <SectionTitle>Documents</SectionTitle>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Button disabled={!!generating} onClick={() => onGenerate('resume')}>
          Generate resume
        </Button>
        <Button disabled={!!generating} onClick={() => onGenerate('cover_letter')}>
          Generate cover letter
        </Button>
        {generating ? (
          <Spinner label={`generating ${generating.replace('_', ' ')}… (local LLM, be patient)`} />
        ) : null}
      </div>
      <ul className="space-y-2">
        {documents.map((doc) => (
          <li key={doc.id} className="rounded-md border border-border bg-elevated/60 p-3 text-sm">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <span>
                <b>{doc.type === 'resume' ? 'Resume' : 'Cover letter'}</b> v{doc.version}
                <span className="ml-2 text-xs text-faint">
                  {new Date(doc.createdAt).toLocaleString()} · {doc.model}
                </span>
              </span>
              <span className="flex gap-2">
                {doc.type === 'cover_letter' ? (
                  <Button
                    variant="ghost"
                    onClick={() => onEdit({ id: doc.id, text: (doc.content as { text?: string })?.text ?? '' })}
                  >
                    edit
                  </Button>
                ) : null}
                {doc.pdfPath ? (
                  <a href={api.documents.pdfUrl(doc.id)}>
                    <Button variant="secondary">
                      PDF <FileDown className="h-4 w-4" aria-hidden="true" />
                    </Button>
                  </a>
                ) : null}
              </span>
            </div>
            {doc.type === 'cover_letter' && editingDoc?.id !== doc.id ? (
              <p className="mt-2 whitespace-pre-wrap text-muted">{(doc.content as { text?: string })?.text}</p>
            ) : null}
            {editingDoc?.id === doc.id ? (
              <div className="mt-2">
                <Textarea
                  className="h-40"
                  value={editingDoc.text}
                  onChange={(e) => onEdit({ ...editingDoc, text: e.target.value })}
                />
                <div className="mt-2 flex gap-2">
                  <Button onClick={() => onSave(editingDoc)}>save & re-render PDF</Button>
                  <Button variant="ghost" onClick={onCancelEdit}>
                    cancel
                  </Button>
                </div>
              </div>
            ) : null}
          </li>
        ))}
      </ul>
    </Surface>
  );
}
