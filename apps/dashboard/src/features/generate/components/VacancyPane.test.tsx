import { describe, it, expect, vi } from 'vitest';
import type { GenerationExportDto } from '@job-finder/shared';
import { renderWithProviders, screen } from '../../../test/test-utils';
import VacancyPane from './VacancyPane';

vi.mock('../../tailor/hooks', () => ({
  useSummaryModel: vi.fn(() => ({ options: [], current: undefined, isLoading: false })),
}));

function renderPane(exportState?: GenerationExportDto) {
  return renderWithProviders(
    <VacancyPane onGenerate={vi.fn()} pending={false} onExport={vi.fn()} exportState={exportState} />,
  );
}

describe('VacancyPane export', () => {
  // A finished export is a file the user asked for: the pane has to hand it
  // over, not just say it exists somewhere.
  it('offers the rendered PDF as a download once the export finished', () => {
    renderPane({ status: 'exported', documentId: 'doc-1' });

    const link = screen.getByRole('link', { name: /download pdf/i });
    expect(link).toHaveAttribute('href', '/api/documents/doc-1/pdf');
    expect(link).toHaveAttribute('download');
  });

  // An export that came back without a document id has nothing to link to;
  // the pane says where the file went rather than rendering a dead link.
  it('falls back to the documents pointer when no document id came back', () => {
    renderPane({ status: 'exported' });

    expect(screen.queryByRole('link', { name: /download pdf/i })).not.toBeInTheDocument();
    expect(screen.getByText(/find it in your documents/i)).toBeInTheDocument();
  });

  it('shows no download control before an export finishes', () => {
    renderPane({ status: 'rendering' });

    expect(screen.queryByRole('link', { name: /download pdf/i })).not.toBeInTheDocument();
  });
});
