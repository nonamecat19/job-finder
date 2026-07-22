import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import CoachPanel from './CoachPanel';
import { mockFitGapAssessment } from '../../test/factories';

vi.mock('./hooks', () => ({
  useCoachAssessment: vi.fn(),
  useAssessCoach: vi.fn(),
}));

vi.mock('../../lib/toastBus', () => ({
  emitToast: vi.fn(),
  toErrorMessage: (err: unknown) => (err instanceof Error ? err.message : String(err)),
}));

import { useCoachAssessment, useAssessCoach } from './hooks';

const mockedUseCoachAssessment = vi.mocked(useCoachAssessment);
const mockedUseAssessCoach = vi.mocked(useAssessCoach);

function setupAssessMock(overrides: Record<string, unknown> = {}) {
  const mutate = vi.fn();
  mockedUseAssessCoach.mockReturnValue({
    mutate,
    isPending: false,
    isError: false,
    error: null,
    data: undefined,
    ...overrides,
  } as any);
  return { mutate };
}

function setupQueryMock(overrides: Record<string, unknown> = {}) {
  mockedUseCoachAssessment.mockReturnValue({
    data: undefined,
    isLoading: false,
    isError: false,
    ...overrides,
  } as any);
}

describe('CoachPanel', () => {
  it('renders loading state', () => {
    setupQueryMock({ isLoading: true });
    setupAssessMock();

    render(<CoachPanel jobId="job-1" />);
    expect(screen.getByText('loading fit-gap assessment…')).toBeInTheDocument();
  });

  it('renders empty state with an Assess button before anything is assessed', () => {
    setupQueryMock({ isError: true });
    setupAssessMock();

    render(<CoachPanel jobId="job-1" />);
    expect(screen.getByText(/Not assessed yet/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Assess/ })).toBeInTheDocument();
  });

  it('clicking Assess calls the mutation', async () => {
    const user = userEvent.setup();
    setupQueryMock({ isError: true });
    const { mutate } = setupAssessMock();

    render(<CoachPanel jobId="job-1" />);
    await user.click(screen.getByRole('button', { name: /Assess/ }));
    expect(mutate).toHaveBeenCalledOnce();
  });

  it('shows a spinner on the Assess button while pending', () => {
    setupQueryMock({ isError: true });
    setupAssessMock({ isPending: true });

    render(<CoachPanel jobId="job-1" />);
    const btn = screen.getByRole('button', { name: /Assess/ });
    expect(btn).toBeDisabled();
    expect(btn.querySelector('.animate-spin')).toBeInTheDocument();
  });

  it('renders an error state when the assess mutation fails', () => {
    setupQueryMock({ isError: true });
    setupAssessMock({ isError: true, error: new Error('LLM unavailable') });

    render(<CoachPanel jobId="job-1" />);
    expect(screen.getByText('LLM unavailable')).toBeInTheDocument();
  });

  it('renders a cached assessment from the GET query', () => {
    const assessment = mockFitGapAssessment();
    setupQueryMock({ data: assessment });
    setupAssessMock();

    render(<CoachPanel jobId="job-1" />);
    expect(screen.getByText(/You fail/)).toBeInTheDocument();
    expect(screen.getByText('Kubernetes')).toBeInTheDocument();
    expect(screen.getByText('Ran container orchestration adjacent to Kubernetes')).toBeInTheDocument();
    expect(screen.getByText(/from: DevOps Engineer, Acme/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Re-assess/ })).toBeInTheDocument();
  });

  it('renders a freshly assessed result from the mutation', () => {
    const assessment = mockFitGapAssessment({ failedMustHaves: 2, totalMustHaves: 3 });
    setupQueryMock({ data: undefined });
    setupAssessMock({ data: assessment });

    render(<CoachPanel jobId="job-1" />);
    expect(screen.getByText('You fail 2 of 3 must-haves')).toBeInTheDocument();
  });

  it('shows the honest empty-evidence message when a gap has none', () => {
    const assessment = mockFitGapAssessment({
      gaps: [{ term: 'Rust', polarity: 'required', noAdjacentEvidence: true, adjacentEvidence: [] }],
    });
    setupQueryMock({ data: assessment });
    setupAssessMock();

    render(<CoachPanel jobId="job-1" />);
    expect(screen.getByText('Rust')).toBeInTheDocument();
    expect(screen.getByText(/No adjacent experience in your profile/)).toBeInTheDocument();
  });

  it('shows a strong-fit message when there are no gaps', () => {
    const assessment = mockFitGapAssessment({ failedMustHaves: 0, gaps: [] });
    setupQueryMock({ data: assessment });
    setupAssessMock();

    render(<CoachPanel jobId="job-1" />);
    expect(screen.getByText('No missing must-haves — strong fit.')).toBeInTheDocument();
  });
});
