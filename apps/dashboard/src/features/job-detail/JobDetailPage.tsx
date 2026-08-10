import { ArrowLeft, ExternalLink, FileDown, X, FileText, Sparkles } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import type { DocumentType, GeneratedDocumentDto, JobDto } from '@job-finder/shared';
import { PageHeader } from '../../components/layout/PageHeader';
import { DashboardGrid, Tile } from '../../components/layout';
import { Button, Chip, LoadingRegion, ScoreBadge, Spinner, SkeletonBlock, SkeletonLine, Textarea } from '../../components/ui';
import { api } from '../../lib/api';
import { queryKeys } from '../../lib/queryKeys';
import { useStartGenerationRun } from '../generate/hooks';
import { useProfiles } from '../profile/hooks';
import {
  useGenerateDocument,
  useJobDetail,
  useJobDocuments,
  useJobDocumentStatuses,
  useMarkJobApplied,
  useMarkJobNotFit,
  useReenrichJob,
  useSaveDocument,
  useUndoJobNotFit,
  useUnmarkJobApplied,
} from './hooks';
import CoachPanel from './CoachPanel';
import CompanyIntelCard from './CompanyIntelCard';
import ContactLine from './ContactLine';
import DOMPurify from 'dompurify';
import EnglishLevelBadge from './EnglishLevelBadge';
import ExperienceBadge from './ExperienceBadge';
import GhostSignalPanel from './GhostSignalPanel';
import KeywordDiffPanel from './KeywordDiffPanel';
import OutreachPanel from './OutreachPanel';
import PostAgeSignal from './PostAgeSignal';
import PrepPackPanel from './PrepPackPanel';
import ReferralPathsCard from './ReferralPathsCard';
import SalaryEstimateCard from './SalaryEstimateCard';

type DetailedJob = JobDto & { documents: GeneratedDocumentDto[] };

export default function JobDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [generating, setGenerating] = useState<DocumentType | null>(null);
  const [countAtGenerate, setCountAtGenerate] = useState(0);
  const [editingDoc, setEditingDoc] = useState<{ id: string; text: string } | null>(null);
  const handledMutationRef = useRef<DocumentType | null>(null);

  const qc = useQueryClient();
  const { data: job, isLoading } = useJobDetail(id);
  const { data: profiles } = useProfiles();
  const profileId = profiles?.[0]?.id;
  const tailorForJob = useStartGenerationRun();
  const handleTailorForJob = () => {
    if (!profileId || !id) return;
    tailorForJob.mutate(
      { profileId, jobId: id },
      { onSuccess: (data) => navigate(`/generate?runId=${data.runId}`) },
    );
  };
  const { data: documents } = useJobDocuments(id);
  const { data: statuses } = useJobDocumentStatuses(id, !!generating);
  const statusCountOfType = (type: DocumentType) => (statuses ?? []).filter((d) => d.type === type).length;

  const generate = useGenerateDocument(id, (type) => {
    setCountAtGenerate(statusCountOfType(type));
  });

  const generatingStatusCount = generating ? statusCountOfType(generating) : 0;
  useEffect(() => {
    if (generating && statuses && generatingStatusCount > countAtGenerate) {
      qc.invalidateQueries({ queryKey: queryKeys.jobs.documents(id) });
    }
  }, [statuses, generating, generatingStatusCount, countAtGenerate, qc, id]);
  const resumeDoc = useMemo(() => {
    const resumes = (documents ?? []).filter((d) => d.type === 'resume' && d.pdfPath);
    return resumes.length ? resumes.reduce((a, b) => (b.version > a.version ? b : a)) : null;
  }, [documents]);
  const markApplied = useMarkJobApplied(id);
  const unmarkApplied = useUnmarkJobApplied(id);
  const markNotFit = useMarkJobNotFit(id);
  const undoNotFit = useUndoJobNotFit(id);
  const saveLetter = useSaveDocument(id, () => setEditingDoc(null));
  const reenrich = useReenrichJob(id);
  const [resumeOpen, setResumeOpen] = useState(false);

  useEffect(() => {
    if (generate.isSuccess && generate.variables && handledMutationRef.current !== generate.variables) {
      handledMutationRef.current = generate.variables;
      setGenerating(generate.variables);
      return;
    }
    if (generating && statuses && generatingStatusCount > countAtGenerate) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setGenerating(null);
    }
  }, [generate.isSuccess, generate.variables, generating, statuses, generatingStatusCount, countAtGenerate]);

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
            <Button
              variant="secondary"
              onClick={handleTailorForJob}
              disabled={!profileId || tailorForJob.isPending}
            >
              <Sparkles className="h-4 w-4" aria-hidden="true" />
              {tailorForJob.isPending ? 'starting…' : 'tailor for this job'}
            </Button>
            {job.status === 'hidden' ? (
              <Button variant="secondary" onClick={() => undoNotFit.mutate()} disabled={undoNotFit.isPending}>
                undo not fit
              </Button>
            ) : job.application?.status === 'applied' ? (
              <Button
                variant="secondary"
                onClick={() => unmarkApplied.mutate()}
                disabled={unmarkApplied.isPending}
              >
                unapply
              </Button>
            ) : (
              <>
                <Button variant="secondary" onClick={() => markNotFit.mutate()} disabled={markNotFit.isPending}>
                  not fit
                </Button>
                <Button onClick={() => markApplied.mutate()} disabled={markApplied.isPending}>
                  mark applied
                </Button>
              </>
            )}
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
        <SalaryEstimateCard
          estimateRaw={job.salaryEstimateRaw}
          estimateMin={job.salaryEstimateMin}
          estimateMax={job.salaryEstimateMax}
          estimateCurrency={job.salaryEstimateCurrency}
          isEstimated={job.salaryIsEstimated}
        />
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
            <button
              onClick={() => setResumeOpen(true)}
              className="flex w-full items-center gap-3 rounded-lg p-2 text-left transition-colors hover:bg-surface-secondary"
            >
              <FileText className="h-8 w-8 text-accent" />
              <div className="flex-1">
                <p className="text-sm font-medium">v{resumeDoc.version}</p>
                <p className="text-xs text-muted">Click to preview</p>
              </div>
            </button>
          </Tile>
        ) : null}

        {resumeOpen && resumeDoc && (
          <div className="fixed inset-0 z-50 flex justify-end">
            <div className="absolute inset-0 bg-black/60" onClick={() => setResumeOpen(false)} />
            <div className="relative flex h-full w-full max-w-4xl flex-col border-l border-border bg-surface shadow-2xl">
              <div className="flex items-center justify-between border-b border-border/60 px-5 py-3.5">
                <div className="flex items-center gap-3">
                  <FileText className="h-5 w-5 text-accent" />
                  <h2 className="text-sm font-semibold tracking-wide text-foreground/90">
                    Resume v{resumeDoc.version}
                  </h2>
                </div>
                <div className="flex items-center gap-2">
                  <a href={api.documents.pdfUrl(resumeDoc.id)} target="_blank" rel="noreferrer">
                    <Button variant="secondary">
                      <FileDown className="h-4 w-4" aria-hidden="true" />
                      open PDF
                    </Button>
                  </a>
                  <button
                    onClick={() => setResumeOpen(false)}
                    className="rounded-md p-1.5 text-muted transition-colors hover:bg-surface-secondary hover:text-foreground"
                    aria-label="Close"
                  >
                    <X className="h-5 w-5" />
                  </button>
                </div>
              </div>
              <iframe
                title="Resume preview"
                src={`${api.documents.pdfUrl(resumeDoc.id)}#toolbar=0&navpanes=0&scrollbar=0`}
                className="flex-1 w-full border-0 bg-white"
              />
            </div>
          </div>
        )}

        <Tile span="wide" title="Danger Zone" tone="default" className="border-red-500/40">
          <div className="flex flex-col gap-3">
            <p className="text-sm text-muted">Re-scrape this vacancy to refresh its metadata from the source.</p>
            <Button onClick={() => reenrich.mutate()} disabled={reenrich.isPending} variant="danger">
              {reenrich.isPending ? 'Enqueuing…' : 'Rescrape vacancy'}
            </Button>
          </div>
        </Tile>
      </DashboardGrid>
    </div>
  );
}

function statusLabel(s: string): string {
  const labels: Record<string, string> = {
    new: 'New',
    found: 'New',
    shortlisted: 'Shortlisted',
    hidden: 'Not fit',
    docs_generated: 'Docs ready',
    applied: 'Applied',
  };
  return labels[s] ?? s;
}

function JobMeta({ job }: { job: DetailedJob }) {
  return (
    <>
      {job.company}
      {job.location ? ` · ${job.location}` : ''}
      {job.remote ? ' · remote' : ''}
      {job.salaryRaw ? ` · ${job.salaryRaw}` : ''}
      <span className="mt-2 flex gap-1 flex-wrap">
        <Chip>{job.sourceKey}</Chip>
        <Chip>{statusLabel(job.status)}</Chip>
        <ExperienceBadge level={job.experienceLevel} minYears={job.experienceMinYears} />
        <EnglishLevelBadge level={job.englishLevel} />
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
