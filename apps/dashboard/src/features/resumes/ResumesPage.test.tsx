import { describe, it, expect, vi, beforeEach } from 'vitest';
import userEvent from '@testing-library/user-event';
import type { GenerationRunDto } from '@job-finder/shared';
import { renderWithProviders, screen } from '../../test/test-utils';
import ResumesPage from './ResumesPage';

vi.mock('./hooks', () => ({
  useGenerationRuns: vi.fn(),
  useDeleteGenerationRun: vi.fn(),
}));
vi.mock('../generate/hooks', () => ({
  useSummaryModel: vi.fn(),
}));
vi.mock('../../lib/api', () => ({
  api: { documents: { pdfUrl: (id: string) => `/api/documents/${id}/pdf` } },
}));

import { useDeleteGenerationRun, useGenerationRuns } from './hooks';
import { useSummaryModel } from '../generate/hooks';

const mockedUseGenerationRuns = vi.mocked(useGenerationRuns);
const mockedUseDelete = vi.mocked(useDeleteGenerationRun);
const mockedUseSummaryModel = vi.mocked(useSummaryModel);

const SHAPE = {
  summaryLines: 3,
  skillsEnabled: true,
  skillsMaxGroups: 4,
  experienceBulletsMin: 3,
  experienceBulletsMax: 5,
  targetPages: 2,
  projectsEnabled: false,
  projectsMin: 0,
  projectsMax: 0,
  projectBulletsMax: 0,
  certificationsEnabled: false,
  certificationsMin: 0,
  certificationsMax: 0,
};

function run(overrides: Partial<GenerationRunDto> = {}): GenerationRunDto {
  return {
    id: 'run-1',
    state: 'ready',
    vacancy: { company: 'Acme', title: 'Senior Engineer' },
    jobId: 'job-1',
    groundingLevel: 'moderate',
    summaryOptionId: 'fast',
    summarySubstituted: false,
    masterChanged: false,
    shapeConfig: SHAPE,
    export: { status: 'exported', documentId: 'doc-1' },
    sections: [
      {
        id: 'sec-1',
        kind: 'summary',
        position: 0,
        targetCount: 1,
        state: 'ready',
        fallbackUsed: false,
        items: [
          {
            id: 'item-1',
            origin: 'ai',
            kind: 'summary',
            text: 'Built pipelines.',
            rank: 0,
            position: 0,
            selected: true,
            edited: false,
            unavailable: false,
          },
        ],
      },
    ],
    createdAt: '2026-08-01T10:00:00Z',
    updatedAt: '2026-08-01T10:00:00Z',
    ...overrides,
  } as GenerationRunDto;
}

beforeEach(() => {
  mockedUseDelete.mockReturnValue({ mutate: vi.fn(), isPending: false } as never);
  mockedUseSummaryModel.mockReturnValue({
    data: { optionId: 'fast', options: [{ id: 'fast', label: 'Fast writer', cost: 'low' }] },
  } as never);
});

describe('ResumesPage', () => {
  it('lists recent runs and shows the newest run config', () => {
    mockedUseGenerationRuns.mockReturnValue({ data: [run()], isLoading: false, error: null, refetch: vi.fn() } as never);

    renderWithProviders(<ResumesPage />);

    expect(screen.getAllByText('Senior Engineer — Acme').length).toBeGreaterThan(0);
    expect(screen.getByText('Fast writer')).toBeInTheDocument();
    expect(screen.getByText('moderate')).toBeInTheDocument();
    // targetPages from the run's own shape config
    expect(screen.getByText('Target pages')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /open pdf/i })).toHaveAttribute(
      'href',
      '/api/documents/doc-1/pdf',
    );
  });

  it('selects a different run when its row is clicked', async () => {
    const runs = [
      run(),
      run({
        id: 'run-2',
        vacancy: { company: 'Globex', title: 'Staff Engineer' },
        groundingLevel: 'strict',
        export: { status: 'blocked' },
      }),
    ];
    mockedUseGenerationRuns.mockReturnValue({ data: runs, isLoading: false, error: null, refetch: vi.fn() } as never);

    renderWithProviders(<ResumesPage />);
    await userEvent.click(screen.getByRole('button', { name: /Staff Engineer/ }));

    expect(screen.getByText('strict')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: /open pdf/i })).not.toBeInTheDocument();
  });

  it('shows an empty state when nothing has been generated', () => {
    mockedUseGenerationRuns.mockReturnValue({ data: [], isLoading: false, error: null, refetch: vi.fn() } as never);

    renderWithProviders(<ResumesPage />);

    expect(screen.getByText('No resumes generated yet.')).toBeInTheDocument();
  });
});
