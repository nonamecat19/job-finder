import type { GenerationRunDto, GenerationSectionDto, ResumeShapeConfigDto } from '@job-finder/shared';
import { EmptyState, ErrorState, Field, LoadingRegion, Select, SkeletonLine, Switch } from '../../../components/ui';
import { useResumeShape } from '../../settings/hooks';
import { useSummaryModel } from '../hooks';

export const GROUNDING_LEVELS = ['strict', 'moderate', 'aggressive'] as const;

const SECTION_KIND_LABELS: Record<string, string> = {
  summary: 'Summary',
  skills: 'Skills',
  projects: 'Projects',
  certifications: 'Certifications',
  education: 'Education',
};

const GLOBAL_ENABLED_KEY: Record<string, keyof ResumeShapeConfigDto> = {
  summary: 'summaryEnabled',
  experience: 'experienceEnabled',
  skills: 'skillsEnabled',
  projects: 'projectsEnabled',
  certifications: 'certificationsEnabled',
  education: 'educationEnabled',
};

function SectionRow({
  label,
  section,
  globalEnabled,
  onToggleEnabled,
}: {
  label: string;
  section: GenerationSectionDto;
  globalEnabled: boolean | undefined;
  onToggleEnabled: (sectionId: string, enabled: boolean) => void;
}) {
  const overridden = globalEnabled !== undefined && section.enabled !== globalEnabled;

  return (
    <div className="flex items-center justify-between gap-3 rounded-xl border border-border px-3.5 py-3">
      <div className="flex items-center gap-2">
        <span className="text-sm text-foreground">{label}</span>
        {overridden ? (
          <span
            className="rounded-full bg-warning-soft px-2 py-0.5 text-xs text-warning"
            title={`Global default is ${globalEnabled ? 'on' : 'off'} for this section — this run overrides it.`}
          >
            overridden
          </span>
        ) : null}
      </div>
      <Switch
        checked={section.enabled}
        label={`Include ${label.toLowerCase()} in this resume`}
        onChange={(enabled) => onToggleEnabled(section.id, enabled)}
      />
    </div>
  );
}

export default function GenerateSettingsTab({
  run,
  onToggleEnabled,
  groundingLevel,
  onGroundingLevelChange,
  summaryOptionId,
  onSummaryOptionIdChange,
}: {
  run: GenerationRunDto | undefined;
  onToggleEnabled: (sectionId: string, enabled: boolean) => void;
  groundingLevel: (typeof GROUNDING_LEVELS)[number];
  onGroundingLevelChange: (level: (typeof GROUNDING_LEVELS)[number]) => void;
  summaryOptionId: string | undefined;
  onSummaryOptionIdChange: (optionId: string) => void;
}) {
  const shape = useResumeShape();
  const { data: summaryModel } = useSummaryModel();

  const singletonSections = (run?.sections ?? [])
    .filter((s) => s.kind !== 'experience')
    .map((s) => ({ section: s, label: SECTION_KIND_LABELS[s.kind] ?? s.kind }));
  const experienceSections = (run?.sections ?? [])
    .filter((s) => s.kind === 'experience')
    .sort((a, b) => a.position - b.position)
    .map((s) => ({ section: s, label: s.entryKey ?? s.entryLabel ?? 'Experience entry' }));

  const preRunFields = (
    <div className="space-y-2">
      <Field label="Grounding">
        <Select
          aria-label="Grounding level"
          value={groundingLevel}
          onChange={(e) => onGroundingLevelChange(e.target.value as (typeof GROUNDING_LEVELS)[number])}
        >
          {GROUNDING_LEVELS.map((l) => (
            <option key={l} value={l}>
              {l}
            </option>
          ))}
        </Select>
      </Field>
      {summaryModel ? (
        <Field label="Summary writer">
          <Select
            aria-label="Summary writer"
            value={summaryOptionId ?? summaryModel.optionId}
            onChange={(e) => onSummaryOptionIdChange(e.target.value)}
          >
            {summaryModel.options.map((o) => (
              <option key={o.id} value={o.id}>
                {o.label} — {o.cost}
              </option>
            ))}
          </Select>
        </Field>
      ) : null}
    </div>
  );

  if (!run) {
    return (
      <div className="space-y-4" data-testid="generate-settings-tab">
        {preRunFields}
      </div>
    );
  }

  if (shape.isPending) {
    return (
      <LoadingRegion label="Loading resume shape settings…" className="space-y-2">
        <SkeletonLine width="w-1/3" className="h-5" />
        <SkeletonLine width="w-full" className="h-10" />
        <SkeletonLine width="w-full" className="h-10" />
      </LoadingRegion>
    );
  }
  if (shape.error) {
    return <ErrorState error={shape.error} />;
  }

  const config = shape.data;

  return (
    <div className="space-y-4" data-testid="generate-settings-tab">
      {preRunFields}

      <p className="text-sm text-muted">
        Overrides the account&apos;s resume shape sections for this run only. Your global defaults, set on the{' '}
        <a href="/settings" className="text-accent hover:underline">
          Settings
        </a>{' '}
        page, are unaffected.
      </p>

      <div className="space-y-2">
        {singletonSections.map(({ section, label }) => (
          <SectionRow
            key={section.id}
            label={label}
            section={section}
            globalEnabled={config?.[GLOBAL_ENABLED_KEY[section.kind]] as boolean | undefined}
            onToggleEnabled={onToggleEnabled}
          />
        ))}
        {experienceSections.map(({ section, label }) => (
          <SectionRow
            key={section.id}
            label={label}
            section={section}
            globalEnabled={config?.experienceEnabled}
            onToggleEnabled={onToggleEnabled}
          />
        ))}
      </div>

      {run.sections.length === 0 ? <EmptyState>This run has no sections yet.</EmptyState> : null}
    </div>
  );
}
