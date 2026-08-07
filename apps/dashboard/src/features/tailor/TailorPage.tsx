import { AlertTriangle, FileDown } from 'lucide-react';
import { useState } from 'react';
import type { GeneratedDocumentDto } from '@job-finder/shared';
import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import { DashboardGrid, Tile } from '../../components/layout';
import { Button, EmptyState, Field, Input, Select, Spinner, Surface, Textarea } from '../../components/ui';
import { useToast } from '../../components/toast';
import { api } from '../../lib/api';
import {
  useAdHocDocuments,
  useGenerateCoverLetter,
  useSaveAdHocDocument,
  useTailorDocuments,
} from './hooks';

const GROUNDING_LEVELS = ['strict', 'moderate', 'aggressive'] as const;

export default function TailorPage() {
  const [vacancy, setVacancy] = useState('');
  const [company, setCompany] = useState('');
  const [title, setTitle] = useState('');
  const [groundingLevel, setGroundingLevel] = useState<(typeof GROUNDING_LEVELS)[number]>('moderate');
  const [result, setResult] = useState<{
    resume: GeneratedDocumentDto;
    coverLetter: GeneratedDocumentDto | null;
  } | null>(null);
  const [editingDoc, setEditingDoc] = useState<{ id: string; text: string } | null>(null);

  const { data: history } = useAdHocDocuments();
  const tailor = useTailorDocuments();
  const coverLetter = useGenerateCoverLetter();
  const saveLetter = useSaveAdHocDocument(() => setEditingDoc(null));
  const { success } = useToast();

  const submit = () => {
    if (!vacancy.trim()) return;
    tailor.mutate(
      { vacancy, company: company || undefined, title: title || undefined, groundingLevel },
      {
        onSuccess: (data) => {
          setResult({ resume: data.resume, coverLetter: data.coverLetter ?? null });
          success('Resume ready', 'Tailored resume generated.');
        },
      },
    );
  };

  const requestCoverLetter = () => {
    if (!result) return;
    coverLetter.mutate(result.resume.id, {
      onSuccess: (doc) => {
        setResult((prev) => (prev ? { ...prev, coverLetter: doc } : prev));
        success('Cover letter ready', 'Cover letter generated for this resume.');
      },
    });
  };

  return (
    <div>
      <PageHeader title="Tailor" description="Paste a vacancy to generate a tailored resume." />

      <DashboardGrid>
        <Tile span="full" title="Resume Tailor">

      <Surface>
        <SectionTitle>Vacancy</SectionTitle>
        <div className="grid gap-3 md:grid-cols-[1fr_1fr_auto] md:items-end">
          <Field label="Company">
            <Input value={company} onChange={(e) => setCompany(e.target.value)} placeholder="Acme Inc." />
          </Field>
          <Field label="Title">
            <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Senior Engineer" />
          </Field>
          <Field label="Grounding">
            <Select value={groundingLevel} onChange={(e) => setGroundingLevel(e.target.value as typeof groundingLevel)}>
              {GROUNDING_LEVELS.map((l) => (
                <option key={l} value={l}>
                  {l}
                </option>
              ))}
            </Select>
          </Field>
        </div>
        <Field label="Vacancy text" className="mt-3">
          <Textarea
            className="h-48"
            value={vacancy}
            onChange={(e) => setVacancy(e.target.value)}
            placeholder="Paste the job posting text here…"
          />
        </Field>
        <div className="mt-3 flex items-center gap-2">
          <Button disabled={!vacancy.trim() || tailor.isPending} onClick={submit}>
            Generate resume
          </Button>
          {tailor.isPending ? <Spinner label="tailoring resume… (local LLM, be patient)" /> : null}
        </div>
        {tailor.isError ? <p className="mt-2 text-sm text-danger">{(tailor.error as Error).message}</p> : null}
      </Surface>

      {result ? (
        <Surface>
          <SectionTitle>Result</SectionTitle>
          {result.resume.summarySubstituted ? (
            <SummarySubstitutionNotice model={result.resume.summaryModel} />
          ) : null}
          <DocumentRow doc={result.resume} />
          {result.coverLetter ? (
            <DocumentRow
              doc={result.coverLetter}
              editingDoc={editingDoc}
              onEdit={setEditingDoc}
              onCancelEdit={() => setEditingDoc(null)}
              onSave={(doc) => saveLetter.mutate(doc)}
            />
          ) : (
            <div className="mt-3 flex items-center gap-2">
              <Button variant="secondary" disabled={coverLetter.isPending} onClick={requestCoverLetter}>
                Generate cover letter
              </Button>
              {coverLetter.isPending ? <Spinner label="writing cover letter…" /> : null}
            </div>
          )}
          {coverLetter.isError ? (
            <p className="mt-2 text-sm text-danger">{(coverLetter.error as Error).message}</p>
          ) : null}
        </Surface>
      ) : null}

      <Surface>
        <SectionTitle>History</SectionTitle>
        {!history || history.length === 0 ? (
          <EmptyState>No ad-hoc generations yet.</EmptyState>
        ) : (
          <ul className="space-y-2">
            {groupHistory(history).map(([key, docs]) => (
              <li key={key} className="rounded-md border border-border bg-surface-secondary/60 p-3 text-sm">
                <div className="mb-2 font-semibold">{key}</div>
                <div className="space-y-2">
                  {docs.map((doc) => (
                    <DocumentRow
                      key={doc.id}
                      doc={doc}
                      editingDoc={editingDoc}
                      onEdit={setEditingDoc}
                      onCancelEdit={() => setEditingDoc(null)}
                      onSave={(d) => saveLetter.mutate(d)}
                    />
                  ))}
                </div>
              </li>
            ))}
          </ul>
        )}
      </Surface>

        </Tile>
      </DashboardGrid>
    </div>
  );
}

function SummarySubstitutionNotice({ model }: { model?: string }) {
  return (
    <div
      className="mb-3 flex items-start gap-2 rounded-lg border border-warning/30 bg-warning-soft p-3 text-sm text-warning"
      role="status"
    >
      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
      <div>
        <span className="font-semibold">Summary written by a fallback model</span>
        <p className="mt-1">
          The configured summary model was unavailable, so the professional summary was written by
          {model ? <> <b>{model}</b></> : ' a fallback'} instead. Review it before you send this resume.
        </p>
      </div>
    </div>
  );
}

function groupHistory(docs: GeneratedDocumentDto[]): [string, GeneratedDocumentDto[]][] {
  const groups = new Map<string, GeneratedDocumentDto[]>();
  for (const doc of docs) {
    const key = `${doc.company ?? 'vacancy'} — ${doc.title ?? 'resume'} · ${new Date(doc.createdAt).toLocaleString()}`;
    groups.set(key, [...(groups.get(key) ?? []), doc]);
  }
  return [...groups.entries()];
}

function DocumentRow({
  doc,
  editingDoc,
  onEdit,
  onCancelEdit,
  onSave,
}: {
  doc: GeneratedDocumentDto;
  editingDoc?: { id: string; text: string } | null;
  onEdit?: (doc: { id: string; text: string }) => void;
  onCancelEdit?: () => void;
  onSave?: (doc: { id: string; text: string }) => void;
}) {
  const isEditing = editingDoc?.id === doc.id;

  return (
    <div className="rounded-md border border-border bg-surface-secondary/60 p-3 text-sm">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <span>
          <b>{doc.type === 'resume' ? 'Resume' : 'Cover letter'}</b> v{doc.version}
          <span className="ml-2 text-xs text-faint">{doc.model}</span>
        </span>
        <span className="flex gap-2">
          {doc.type === 'cover_letter' && onEdit ? (
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
      {doc.type === 'cover_letter' && !isEditing ? (
        <p className="mt-2 whitespace-pre-wrap text-muted">{(doc.content as { text?: string })?.text}</p>
      ) : null}
      {isEditing && editingDoc && onEdit && onSave && onCancelEdit ? (
        <div className="mt-2">
          <Textarea className="h-40" value={editingDoc.text} onChange={(e) => onEdit({ ...editingDoc, text: e.target.value })} />
          <div className="mt-2 flex gap-2">
            <Button onClick={() => onSave(editingDoc)}>save &amp; re-render PDF</Button>
            <Button variant="ghost" onClick={onCancelEdit}>
              cancel
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
