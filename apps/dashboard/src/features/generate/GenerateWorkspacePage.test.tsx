import { describe, it, expect, vi } from 'vitest';
import type { GenerationRunDto } from '@job-finder/shared';
import { renderWithProviders, screen } from '../../test/test-utils';
import GenerateWorkspacePage from './GenerateWorkspacePage';

vi.mock('./hooks', () => ({
  useGenerationRun: vi.fn(),
  useStartGenerationRun: vi.fn(),
}));
vi.mock('../profile/hooks', () => ({
  useProfiles: vi.fn(),
}));
vi.mock('../tailor/hooks', () => ({
  useSummaryModel: vi.fn(),
}));

import { useGenerationRun, useStartGenerationRun } from './hooks';
import { useProfiles } from '../profile/hooks';
import { useSummaryModel } from '../tailor/hooks';

const mockedUseGenerationRun = vi.mocked(useGenerationRun);
const mockedUseStartGenerationRun = vi.mocked(useStartGenerationRun);
const mockedUseProfiles = vi.mocked(useProfiles);
const mockedUseSummaryModel = vi.mocked(useSummaryModel);

function baseRun(overrides: Partial<GenerationRunDto> = {}): GenerationRunDto {
  return {
    id: 'run-1',
    state: 'ready',
    vacancy: { company: 'Acme', title: 'Senior Engineer' },
    groundingLevel: 'moderate',
    summarySubstituted: false,
    masterChanged: false,
    shapeConfig: {
      summaryLines: 4,
      skillsEnabled: true,
      skillsMaxGroups: 0,
      experienceBulletsMin: 8,
      experienceBulletsMax: 10,
      targetPages: 2,
      projectsEnabled: true,
      projectsMin: 0,
      projectsMax: 0,
      projectBulletsMax: 0,
      certificationsEnabled: true,
      certificationsMin: 0,
      certificationsMax: 0,
      fontSize: 10,
    },
    export: { status: '' },
    sections: [
      {
        id: 'sec-1',
        kind: 'experience',
        entryKey: 'Acme Inc.',
        entryLabel: 'Senior Engineer · 2021–2024',
        position: 1,
        targetCount: 8,
        state: 'ready',
        fallbackUsed: false,
        items: [
          {
            id: 'item-1',
            origin: 'profile',
            kind: 'achievement',
            text: 'Shipped the thing',
            sourceIndex: 0,
            rank: 0,
            position: 0,
            selected: true,
            edited: false,
            unavailable: false,
          },
        ],
      },
    ],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function setup() {
  mockedUseProfiles.mockReturnValue({ data: [{ id: 'profile-1' }] } as any);
  mockedUseSummaryModel.mockReturnValue({ data: undefined } as any);
  mockedUseStartGenerationRun.mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false } as any);
}

describe('GenerateWorkspacePage', () => {
  it('renders the two-pane layout', () => {
    setup();
    mockedUseGenerationRun.mockReturnValue({ data: undefined, isLoading: false, error: null } as any);

    renderWithProviders(<GenerateWorkspacePage />);

    // Left pane: the generated-resume surface (empty state with no run yet).
    expect(screen.getByRole('heading', { name: /generated resume/i })).toBeInTheDocument();
    // Right pane: the vacancy controls.
    expect(screen.getByText(/^vacancy$/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /generate/i })).toBeInTheDocument();
  });

  it('shows progress rather than an empty workspace for a running run', () => {
    setup();
    window.history.pushState({}, '', '/generate?runId=run-1');
    mockedUseGenerationRun.mockReturnValue({
      data: baseRun({ state: 'running', sections: [] }),
      isLoading: false,
      error: null,
    } as any);

    renderWithProviders(<GenerateWorkspacePage />);

    expect(screen.getByRole('status')).toBeInTheDocument();
    expect(screen.queryByText(/fill in a vacancy/i)).not.toBeInTheDocument();
  });

  it('renders a ready run\'s sections', () => {
    setup();
    window.history.pushState({}, '', '/generate?runId=run-1');
    mockedUseGenerationRun.mockReturnValue({ data: baseRun(), isLoading: false, error: null } as any);

    renderWithProviders(<GenerateWorkspacePage />);

    expect(screen.getByText('Shipped the thing')).toBeInTheDocument();
  });
});
