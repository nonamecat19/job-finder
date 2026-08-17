import type { GenerationExportDto } from '@job-finder/shared';

// OverflowReport is FR-019: the export is over the page budget, so it is
// reported with named candidates the user acts on. There is deliberately no
// "apply" control — nothing here deselects anything, because resolving the
// overflow silently is exactly what the rule forbids. Shared by VacancyPane
// (the export control itself) and ResumePreviewPane (the live preview),
// rather than duplicated between them.
export default function OverflowReport({ report }: { report: NonNullable<GenerationExportDto['report']> }) {
  return (
    <div className="mt-2 rounded-lg bg-warning-soft p-2.5" data-testid="overflow-report">
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
