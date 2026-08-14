import { FileDown } from 'lucide-react';
import { useState } from 'react';
import type { GenerationExportDto } from '@job-finder/shared';
import { Button, Field, Input, Select, Spinner, Surface, Textarea } from '../../../components/ui';
import { SectionTitle } from '../../../components/layout/PageHeader';
import { api } from '../../../lib/api';
import { useSummaryModel } from '../../tailor/hooks';

const GROUNDING_LEVELS = ['strict', 'moderate', 'aggressive'] as const;

export interface VacancyPaneInput {
  company?: string;
  title?: string;
  vacancy: { company: string; title: string; text: string };
  groundingLevel: string;
  summaryOptionId?: string;
}

interface VacancyPaneProps {
  onGenerate: (input: VacancyPaneInput) => void;
  pending: boolean;
  disabled?: boolean;
  // The export half (T072/T073): absent until a run exists, so the pane is
  // unchanged on the empty workspace.
  onExport?: () => void;
  exportPending?: boolean;
  exportDisabled?: boolean;
  exportState?: GenerationExportDto;
  exportError?: string;
  warnings?: string[];
  // T076/T078: rerun — absent until a run exists, same as onExport. Calling
  // onRerun() with no argument reruns the whole run; passing section ids
  // reruns exactly those (a `failed` section's per-section retry).
  onRerun?: (sections?: string[]) => void;
  rerunPending?: boolean;
  failedSections?: { id: string; label: string }[];
}

// FR-021's warning, shown before every rerun regardless of scope — the
// server preserves matched selections either way, but the ranking/ordering
// itself is genuinely replaced and the user should know that before they ask
// for it.
const RERUN_WARNING = "Re-running replaces the AI's ordering for this section. Your checkbox choices, positions and edits carry over where the same content still exists. Continue?";

// T021: company/title/vacancy text plus the two controls that carry over
// from /tailor — grounding level and the 034 summary writer — reused via the
// same useSummaryModel hook TailorPage uses, so the menu and the current
// choice never disagree about which options exist.
export default function VacancyPane({
  onGenerate,
  pending,
  disabled,
  onExport,
  exportPending,
  exportDisabled,
  exportState,
  exportError,
  warnings,
  onRerun,
  rerunPending,
  failedSections,
}: VacancyPaneProps) {
  const [company, setCompany] = useState('');
  const [title, setTitle] = useState('');
  const [vacancy, setVacancy] = useState('');
  const [groundingLevel, setGroundingLevel] = useState<(typeof GROUNDING_LEVELS)[number]>('moderate');
  const [summaryOptionId, setSummaryOptionId] = useState<string | undefined>(undefined);

  const { data: summaryModel } = useSummaryModel();
  const chosenSummaryOptionId = summaryOptionId ?? summaryModel?.optionId ?? '';
  const chosenSummaryOption = summaryModel?.options.find((o) => o.id === chosenSummaryOptionId);

  const submit = () => {
    if (!vacancy.trim()) return;
    onGenerate({
      company,
      title,
      vacancy: { company, title, text: vacancy },
      groundingLevel,
      summaryOptionId,
    });
  };

  return (
    <Surface>
      <SectionTitle>Vacancy</SectionTitle>
      <div className="grid gap-3 md:grid-cols-2">
        <Field label="Company">
          <Input value={company} onChange={(e) => setCompany(e.target.value)} placeholder="Acme Inc." />
        </Field>
        <Field label="Title">
          <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Senior Engineer" />
        </Field>
      </div>

      <Field label="Grounding (governs the summary only)" className="mt-3">
        <Select
          aria-label="Grounding level"
          value={groundingLevel}
          onChange={(e) => setGroundingLevel(e.target.value as typeof groundingLevel)}
        >
          {GROUNDING_LEVELS.map((l) => (
            <option key={l} value={l}>
              {l}
            </option>
          ))}
        </Select>
        <p className="mt-1 text-xs text-muted">
          Profile-sourced bullets are always byte-identical to your master resume. This control only
          affects how the written summary is worded.
        </p>
      </Field>

      {summaryModel ? (
        <Field label="Summary writer" className="mt-3">
          <Select
            aria-label="Summary writer"
            value={chosenSummaryOptionId}
            onChange={(e) => setSummaryOptionId(e.target.value)}
          >
            {summaryModel.options.map((o) => (
              <option key={o.id} value={o.id}>
                {o.label} — {o.cost}
              </option>
            ))}
          </Select>
          {chosenSummaryOption ? (
            <p className="mt-1 text-xs text-muted">{chosenSummaryOption.description}</p>
          ) : null}
        </Field>
      ) : null}

      <Field label="Vacancy text" className="mt-3">
        <Textarea
          className="h-48"
          value={vacancy}
          onChange={(e) => setVacancy(e.target.value)}
          placeholder="Paste the job posting text here…"
        />
      </Field>

      <div className="mt-3 flex items-center gap-2">
        <Button disabled={disabled || !vacancy.trim() || pending} onClick={submit}>
          Generate
        </Button>
        {pending ? <Spinner label="starting generation…" /> : null}
      </div>

      {onRerun ? (
        <div className="mt-4 border-t border-subtle pt-3">
          <SectionTitle>Re-run</SectionTitle>
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              disabled={rerunPending}
              onClick={() => {
                if (window.confirm(RERUN_WARNING)) onRerun();
              }}
            >
              Re-run whole resume
            </Button>
            {rerunPending ? <Spinner label="re-running…" /> : null}
          </div>
          {failedSections && failedSections.length > 0 ? (
            <div className="mt-2 space-y-1" data-testid="failed-sections">
              <p className="text-xs text-danger">These sections failed to generate:</p>
              {failedSections.map((s) => (
                <div key={s.id} className="flex items-center justify-between gap-2 text-xs">
                  <span className="text-muted">{s.label}</span>
                  <Button
                    variant="secondary"
                    disabled={rerunPending}
                    onClick={() => {
                      if (window.confirm(RERUN_WARNING)) onRerun([s.id]);
                    }}
                  >
                    Retry
                  </Button>
                </div>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

      {onExport ? (
        <div className="mt-4 border-t border-subtle pt-3">
          <SectionTitle>Export</SectionTitle>
          {warnings?.map((warning) => (
            <p key={warning} className="mb-2 rounded-md border border-warning/30 bg-warning-soft p-2 text-xs text-warning">
              {warning}
            </p>
          ))}
          <div className="flex items-center gap-2">
            <Button disabled={exportDisabled || exportPending} onClick={onExport}>
              Export PDF
            </Button>
            {exportPending ? <Spinner label="rendering your resume…" /> : null}
          </div>
          {exportError ? <p className="mt-2 text-sm text-danger">{exportError}</p> : null}
          {exportState?.status === 'exported' ? (
            <div className="mt-2">
              <p className="text-xs text-muted">
                Exported — it contains exactly the items you included.
              </p>
              {exportState.documentId ? (
                <a
                  className="mt-2 inline-block"
                  href={api.documents.pdfUrl(exportState.documentId)}
                  download
                >
                  <Button variant="secondary">
                    Download PDF <FileDown className="h-4 w-4" aria-hidden="true" />
                  </Button>
                </a>
              ) : (
                <p className="mt-1 text-xs text-muted">Find it in your documents.</p>
              )}
            </div>
          ) : null}
          {exportState?.status === 'blocked' && exportState.report ? (
            <OverflowReport report={exportState.report} />
          ) : null}
        </div>
      ) : null}
    </Surface>
  );
}

// OverflowReport is FR-019: the export is over the page budget, so it is
// reported with named candidates the user acts on. There is deliberately no
// "apply" control — nothing here deselects anything, because resolving the
// overflow silently is exactly what the rule forbids.
function OverflowReport({ report }: { report: NonNullable<GenerationExportDto['report']> }) {
  return (
    <div className="mt-2 rounded-md border border-warning/30 bg-warning-soft p-2" data-testid="overflow-report">
      <p className="text-xs text-warning">
        This selection renders as {report.pagesRendered} page{report.pagesRendered === 1 ? '' : 's'}, over your target
        of {report.pagesTarget}. Nothing was dropped or reworded.
      </p>
      {report.candidates.length > 0 ? (
        <>
          <p className="mt-2 text-xs text-muted">Lowest-ranked included items, worst first:</p>
          <ul className="mt-1 list-disc space-y-0.5 pl-4 text-xs text-muted">
            {report.candidates.map((c) => (
              <li key={c.itemId}>{c.label}</li>
            ))}
          </ul>
          <p className="mt-2 text-xs text-muted">Uncheck what you can spare on the left, then export again.</p>
        </>
      ) : null}
    </div>
  );
}
