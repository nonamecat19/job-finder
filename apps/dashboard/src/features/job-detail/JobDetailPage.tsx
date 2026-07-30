import { ArrowLeft, ExternalLink, FileDown } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import type { DocumentType, GeneratedDocumentDto, JobDto } from '@job-finder/shared';
import { PageHeader } from '../../components/layout/PageHeader';
import { DashboardGrid, Tile } from '../../components/layout';
import { Button, Chip, LoadingRegion, ScoreBadge, Spinner, SkeletonBlock, SkeletonLine, Textarea } from '../../components/ui';
import { api } from '../../lib/api';
import {
  useGenerateDocument,
  useJobDetail,
  useJobDocuments,
  useMarkJobApplied,
  useSaveDocument,
} from './hooks';
import CoachPanel from './CoachPanel';
import CompanyIntelCard from './CompanyIntelCard';
import ContactLine from './ContactLine';
import DOMPurify from 'dompurify';
import GhostSignalPanel from './GhostSignalPanel';
import KeywordDiffPanel from './KeywordDiffPanel';
import OutreachPanel from './OutreachPanel';
import PostAgeSignal from './PostAgeSignal';
import PrepPackPanel from './PrepPackPanel';
import ReferralPathsCard from './ReferralPathsCard';

type DetailedJob = JobDto & { documents: GeneratedDocumentDto[] };

export default function JobDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [generating, setGenerating] = useState<DocumentType | null>(null);
  const [countAtGenerate, setCountAtGenerate] = useState(0);
  const [editingDoc, setEditingDoc] = useState<{ id: string; text: string } | null>(null);
  const handledMutationRef = useRef<DocumentType | null>(null);

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
    if (generate.isSuccess && generate.variables && handledMutationRef.current !== generate.variables) {
      handledMutationRef.current = generate.variables;
      setGenerating(generate.variables);
      return;
    }
    if (generating && documents && docCountOfType(generating) > countAtGenerate) {
      // Clears the "generating" flag once the query cache actually reflects
      // the new document — genuinely reacting to an external system (the
      // documents query) settling, not state derivable from props/state
      // during render. Reviewed as safe (spec 023-workflow-quality-gates
      // FR-012 lint adoption).
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setGenerating(null);
    }
  }, [generate.isSuccess, generate.variables, generating, documents, countAtGenerate]);

  if (isLoading || !job) {
    return (
      <LoadingRegion label="loading job…" className="space-y-5">
        <SkeletonLine width="w-1/2" className="h-6" />
        <SkeletonLine width="w-1/3" />
        <SkeletonBlock className="h-24 w-full" />
        <SkeletonBlock className="h-24 w-full" />
        <SkeletonBlock className="h-40 w-full" />
      </LoadingRegion>
    );
  }

  return (
    <div>
      <Link to="/" className="mb-5 inline-flex items-center gap-1 text-sm font-medium text-muted hover:text-foreground">
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

      <DashboardGrid>
        {job.matchResult ? (
          <Tile span="full" title="Fit">
            <FitSummary job={job} />
          </Tile>
        ) : null}

        <Tile span="wide" title="Ghost-job signal">
          <GhostSignalPanel jobId={job.id} ghostSignal={job.ghostSignal} />
        </Tile>
        <Tile span="standard" title="Response rate">
          <PostAgeSignal />
        </Tile>

        <Tile span="standard" title="Contact">
          <ContactLine jobId={job.id} />
        </Tile>
        <Tile span="standard" title="Company Intel">
          <CompanyIntelCard jobId={job.id} />
        </Tile>
        <Tile span="standard" title="Referral paths">
          <ReferralPathsCard jobId={id} />
        </Tile>

        <Tile span="wide" title="ATS keyword match">
          <KeywordDiffPanel jobId={id} />
        </Tile>
        <Tile span="standard" title="Coaching">
          <div className="flex flex-col gap-5">
            <CoachPanel jobId={id} />
            <OutreachPanel jobId={id} />
          </div>
        </Tile>

        <Tile span="full" title="Interview prep pack">
          <PrepPackPanel jobId={id} />
        </Tile>

        <Tile span="full" title="Documents">
          <DocumentsPanel
            documents={documents ?? []}
            generating={generating}
            editingDoc={editingDoc}
            onGenerate={(type) => generate.mutate(type)}
            onEdit={setEditingDoc}
            onCancelEdit={() => setEditingDoc(null)}
            onSave={(doc) => saveLetter.mutate(doc)}
          />
        </Tile>

        <Tile span="feature" title="Job description">
          {job.descriptionHtml ? (
            <div
              className="prose prose-sm max-w-none text-muted [&_a]:text-accent [&_a:hover]:underline [&_h1]:text-base [&_h1]:font-semibold [&_h2]:text-sm [&_h2]:font-semibold [&_h3]:text-sm [&_h3]:font-semibold [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5 [&_li]:my-1 [&_p]:my-2 [&_strong]:font-semibold"
              dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(job.descriptionHtml) }}
            />
          ) : (
            <p className="whitespace-pre-wrap text-sm leading-6 text-muted">{job.description}</p>
          )}
        </Tile>
        {resumeDoc ? (
          <Tile span="standard" title="Resume">
            <ResumePreview doc={resumeDoc} />
          </Tile>
        ) : null}
      </DashboardGrid>
    </div>
  );
}

function ResumePreview({ doc }: { doc: GeneratedDocumentDto }) {
  return (
    <>
      <div className="mb-3 flex items-center justify-between gap-2">
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
    </>
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
    <>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <ScoreBadge score={mr.score} />
        <span className="text-xs font-normal text-faint">
          similarity {Number(mr.similarity).toFixed(2)} · {mr.model}
        </span>
      </div>
      {mr.summary ? <p className="mb-2 text-sm leading-6 text-muted">{mr.summary}</p> : null}
      <div className="flex flex-wrap gap-1">
        {(mr.matchedSkills ?? []).map((s: string, i: number) => (
          <Chip key={`matched-${s}-${i}`} tone="green">
            {s}
          </Chip>
        ))}
        {(mr.missingSkills ?? []).map((s: string, i: number) => (
          <Chip key={`missing-${s}-${i}`} tone="red">
            {s}
          </Chip>
        ))}
      </div>
      {(mr.redFlags ?? []).length > 0 ? (
        <ul className="mt-3 list-inside list-disc text-sm text-danger">
          {(mr.redFlags as string[]).map((f, i) => (
            <li key={`${f}-${i}`}>{f}</li>
          ))}
        </ul>
      ) : null}
    </>
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
    <>
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
          <li key={doc.id} className="rounded-md border border-border bg-surface-secondary/60 p-3 text-sm">
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
            {editingDoc && editingDoc.id === doc.id ? (
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
    </>
  );
}
