import { useState } from 'react';
import { Button, Field, Input, Select, Spinner, Surface, Textarea } from '../../../components/ui';
import { SectionTitle } from '../../../components/layout/PageHeader';
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
}

// T021: company/title/vacancy text plus the two controls that carry over
// from /tailor — grounding level and the 034 summary writer — reused via the
// same useSummaryModel hook TailorPage uses, so the menu and the current
// choice never disagree about which options exist.
export default function VacancyPane({ onGenerate, pending, disabled }: VacancyPaneProps) {
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
    </Surface>
  );
}
