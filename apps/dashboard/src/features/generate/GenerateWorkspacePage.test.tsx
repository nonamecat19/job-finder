import { describe, it, expect, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import type { GenerationRunDto } from '@job-finder/shared';
import { renderWithProviders, screen } from '../../test/test-utils';
import GenerateWorkspacePage from './GenerateWorkspacePage';

vi.mock('./hooks', () => ({
  useGenerationRun: vi.fn(),
  useStartGenerationRun: vi.fn(),
  useToggleGenerationItem: vi.fn(),
  useReorderGenerationSection: vi.fn(),
  useExportGenerationRun: vi.fn(),
}));
vi.mock('../profile/hooks', () => ({
  useProfiles: vi.fn(),
}));
vi.mock('../tailor/hooks', () => ({
  useSummaryModel: vi.fn(),
}));

import {
  useExportGenerationRun,
  useGenerationRun,
  useReorderGenerationSection,
  useStartGenerationRun,
  useToggleGenerationItem,
} from './hooks';
import { useProfiles } from '../profile/hooks';
import { useSummaryModel } from '../tailor/hooks';

const mockedUseGenerationRun = vi.mocked(useGenerationRun);
const mockedUseStartGenerationRun = vi.mocked(useStartGenerationRun);
const mockedUseToggleGenerationItem = vi.mocked(useToggleGenerationItem);
const mockedUseReorderGenerationSection = vi.mocked(useReorderGenerationSection);
const mockedUseExportGenerationRun = vi.mocked(useExportGenerationRun);
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

// runWithSuggestion is a ready run carrying every section kind plus one
// AI-origin suggestion, unselected — the state a user is in the moment a run
// finishes, before they have touched anything.
function runWithSuggestion(): GenerationRunDto {
  const run = baseRun();
  run.sections[0].items.push({
    id: 'item-ai-1',
    origin: 'ai',
    kind: 'achievement',
    text: 'Led the migration to a service mesh',
    rank: 1,
    position: 1,
    selected: false,
    edited: false,
    unavailable: false,
  });
  run.sections.push(
    {
      id: 'sec-summary',
      kind: 'summary',
      position: 0,
      targetCount: 0,
      state: 'ready',
      fallbackUsed: false,
      items: [
        {
          id: 'item-summary',
          origin: 'ai',
          kind: 'summary',
          text: 'Senior engineer with a track record of shipping.',
          rank: 0,
          position: 0,
          selected: true,
          edited: false,
          unavailable: false,
        },
      ],
    },
    {
      id: 'sec-skills',
      kind: 'skills',
      position: 2,
      targetCount: 0,
      state: 'ready',
      fallbackUsed: false,
      items: [
        {
          id: 'item-skill',
          origin: 'profile',
          kind: 'skill_group',
          text: 'Languages: Go, TypeScript',
          sourceIndex: 0,
          rank: 0,
          position: 0,
          selected: true,
          edited: false,
          unavailable: false,
        },
      ],
    },
  );
  return run;
}

function setup() {
  mockedUseProfiles.mockReturnValue({ data: [{ id: 'profile-1' }] } as any);
  mockedUseSummaryModel.mockReturnValue({ data: undefined } as any);
  mockedUseStartGenerationRun.mockReturnValue({ mutate: vi.fn(), isPending: false, isError: false } as any);
  mockedUseToggleGenerationItem.mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
  mockedUseReorderGenerationSection.mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
  mockedUseExportGenerationRun.mockReturnValue({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    data: undefined,
  } as any);
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

  // T036 / SC-006: toggling an item previews the change through a PATCH
  // mutation, never through the start-a-run mutation — toggling must not
  // cost a model call.
  it('toggling an item mutates the item, not a new generation run', async () => {
    setup();
    const toggleMutate = vi.fn();
    mockedUseToggleGenerationItem.mockReturnValue({ mutate: toggleMutate, isPending: false } as any);
    const startMutate = vi.fn();
    mockedUseStartGenerationRun.mockReturnValue({ mutate: startMutate, isPending: false, isError: false } as any);
    window.history.pushState({}, '', '/generate?runId=run-1');
    mockedUseGenerationRun.mockReturnValue({ data: baseRun(), isLoading: false, error: null } as any);

    const user = userEvent.setup();
    renderWithProviders(<GenerateWorkspacePage />);

    const checkbox = screen.getByRole('checkbox', { name: /included/i });
    await user.click(checkbox);

    expect(toggleMutate).toHaveBeenCalledWith({ itemId: 'item-1', selected: false });
    expect(startMutate).not.toHaveBeenCalled();
  });

  // T057: a run with suggestions produces zero selected AI items until the
  // user acts (FR-013/SC-004) — the checkbox for an "ai" item starts
  // unchecked, and including it keeps the "AI · unverified" badge rather
  // than swapping it for the profile badge.
  it('renders AI suggestions unselected by default and keeps their badge once included', async () => {
    setup();
    const toggleMutate = vi.fn();
    mockedUseToggleGenerationItem.mockReturnValue({ mutate: toggleMutate, isPending: false } as any);
    window.history.pushState({}, '', '/generate?runId=run-1');
    const run = baseRun();
    run.sections[0].items.push({
      id: 'item-ai-1',
      origin: 'ai',
      kind: 'achievement',
      text: 'Led the migration to a service mesh',
      rank: 1,
      position: 1,
      selected: false,
      edited: false,
      unavailable: false,
    });
    mockedUseGenerationRun.mockReturnValue({ data: run, isLoading: false, error: null } as any);

    const user = userEvent.setup();
    renderWithProviders(<GenerateWorkspacePage />);

    const suggestionRow = screen.getByText('Led the migration to a service mesh').closest('li')!;
    expect(suggestionRow.querySelector('[data-badge="origin-ai-unverified"]')).not.toBeNull();

    const checkbox = suggestionRow.querySelector('input[type="checkbox"]') as HTMLInputElement;
    expect(checkbox.checked).toBe(false);

    await user.click(checkbox);

    expect(toggleMutate).toHaveBeenCalledWith({ itemId: 'item-ai-1', selected: true });
    // The badge is a property of origin, not of selection — it survives
    // inclusion (FR-014).
    expect(suggestionRow.querySelector('[data-badge="origin-ai-unverified"]')).not.toBeNull();
  });
  // T075 / SC-004: the user generates and exports without touching a single
  // suggestion. Nothing AI-written is included, so nothing AI-written ships.
  it('exports zero AI-origin items when the user acts on no suggestion', async () => {
    setup();
    const exportMutate = vi.fn();
    mockedUseExportGenerationRun.mockReturnValue({
      mutate: exportMutate,
      isPending: false,
      isError: false,
      data: undefined,
    } as any);
    window.history.pushState({}, '', '/generate?runId=run-1');
    const run = runWithSuggestion();
    mockedUseGenerationRun.mockReturnValue({ data: run, isLoading: false, error: null } as any);

    const user = userEvent.setup();
    renderWithProviders(<GenerateWorkspacePage />);

    // Untouched: the suggestion is present and not included.
    const suggestionRow = screen.getByText('Led the migration to a service mesh').closest('li')!;
    expect((suggestionRow.querySelector('input[type="checkbox"]') as HTMLInputElement).checked).toBe(false);

    await user.click(screen.getByRole('button', { name: /export pdf/i }));

    expect(exportMutate).toHaveBeenCalledTimes(1);
    // What the export ships is what is selected, and no selected item is
    // AI-origin — the same fact the pre-export unverified-content warning
    // reports, so its absence is the assertion.
    expect(screen.queryByText(/AI-written item/i)).not.toBeInTheDocument();
  });

  // T073: the warnings a user sees before exporting — an emptied section, and
  // AI-written content they chose to include.
  it('warns before export about empty sections and included AI content', () => {
    setup();
    window.history.pushState({}, '', '/generate?runId=run-1');
    const run = runWithSuggestion();
    run.sections[0].items = run.sections[0].items.map((i) =>
      i.origin === 'ai' ? { ...i, selected: true } : i,
    );
    run.sections[1].items = [];
    mockedUseGenerationRun.mockReturnValue({ data: run, isLoading: false, error: null } as any);

    renderWithProviders(<GenerateWorkspacePage />);

    expect(screen.getByText(/this resume has no summary/i)).toBeInTheDocument();
    expect(screen.getByText(/AI-written item/i)).toBeInTheDocument();
  });

  // T072: an over-budget export is reported with named candidates, and the
  // report offers no way to apply them (FR-019).
  it('reports an overflow with named candidates and no apply control', () => {
    setup();
    mockedUseExportGenerationRun.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: false,
      data: {
        status: 'blocked',
        report: {
          pagesRendered: 3,
          pagesTarget: 2,
          candidates: [{ itemId: 'item-1', sectionId: 'sec-1', label: 'Acme Inc. · bullet 8', rank: 7 }],
        },
      },
    } as any);
    window.history.pushState({}, '', '/generate?runId=run-1');
    mockedUseGenerationRun.mockReturnValue({ data: runWithSuggestion(), isLoading: false, error: null } as any);

    renderWithProviders(<GenerateWorkspacePage />);

    const report = screen.getByTestId('overflow-report');
    expect(report).toHaveTextContent('3 pages');
    expect(report).toHaveTextContent('Acme Inc. · bullet 8');
    expect(report.querySelector('button')).toBeNull();
  });
});
