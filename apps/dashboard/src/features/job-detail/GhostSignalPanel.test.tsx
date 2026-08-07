import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import type { JobSignalDto } from '@job-finder/shared';
import GhostSignalPanel from './GhostSignalPanel';

vi.mock('./hooks', () => ({
  useRefreshGhostScore: vi.fn(),
}));

import { useRefreshGhostScore } from './hooks';

const mockedUseRefreshGhostScore = vi.mocked(useRefreshGhostScore);

function setupRefreshMock(isPending = false) {
  const mutate = vi.fn();
  mockedUseRefreshGhostScore.mockReturnValue({ mutate, isPending } as any);
  return { mutate };
}

function ghostSignal(overrides: Partial<JobSignalDto> = {}): JobSignalDto {
  return {
    id: 'sig-1',
    jobId: 'job-1',
    kind: 'ghost',
    score: 82,
    model: 'qwen2.5:14b',
    createdAt: '2026-07-20T10:00:00Z',
    signals: {
      repostCount: 3,
      daysOpen: 71,
      crossBoardCount: 2,
      alwaysHiringCount: 9,
      confidence: 0.72,
      explanation: 'Reposted 3 times and open 71 days.',
      notes: { repost: 'measured', daysOpen: 'measured', crossBoard: 'measured', alwaysHiring: 'measured' },
    },
    ...overrides,
  };
}

describe('GhostSignalPanel', () => {
  it('renders "not scored yet" when there is no ghost signal', () => {
    setupRefreshMock();
    render(<GhostSignalPanel jobId="job-1" ghostSignal={null} />);
    expect(screen.getByText('Not scored yet.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /score now/ })).toBeInTheDocument();
  });

  it('renders score, confidence, model, and explanation for a scored job', () => {
    setupRefreshMock();
    render(<GhostSignalPanel jobId="job-1" ghostSignal={ghostSignal()} />);

    expect(screen.getByText('82')).toBeInTheDocument();
    expect(screen.getByText(/confidence 0.72/)).toBeInTheDocument();
    expect(screen.getByText(/qwen2.5:14b/)).toBeInTheDocument();
    expect(screen.getByText('Reposted 3 times and open 71 days.')).toBeInTheDocument();
  });

  it('renders all four signal values', () => {
    setupRefreshMock();
    render(<GhostSignalPanel jobId="job-1" ghostSignal={ghostSignal()} />);

    expect(screen.getByText('Repost count')).toBeInTheDocument();
    expect(screen.getByText('Days open')).toBeInTheDocument();
    expect(screen.getByText('Cross-board duplicates')).toBeInTheDocument();
    expect(screen.getByText('Always-hiring count')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();
    expect(screen.getByText('71')).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.getByText('9')).toBeInTheDocument();
  });

  it('renders unknown signals with their reason, never as 0', () => {
    setupRefreshMock();
    const sig = ghostSignal({
      signals: {
        repostCount: 1,
        daysOpen: null,
        crossBoardCount: null,
        alwaysHiringCount: null,
        confidence: 0.3,
        explanation: 'Signals mostly unknown.',
        notes: {
          repost: 'measured',
          daysOpen: 'unknown: no postedAt',
          crossBoard: 'unknown: description empty or a teaser',
          alwaysHiring: 'unknown: company name unparseable',
        },
      },
    });
    render(<GhostSignalPanel jobId="job-1" ghostSignal={sig} />);

    const unknowns = screen.getAllByText(/^unknown/);
    expect(unknowns.length).toBe(3);
    expect(screen.queryByText('0')).not.toBeInTheDocument();
  });

  it('shows a low-confidence indicator when confidence is below the threshold', () => {
    setupRefreshMock();
    const sig = ghostSignal({ signals: { ...ghostSignal().signals, confidence: 0.3 } });
    render(<GhostSignalPanel jobId="job-1" ghostSignal={sig} />);
    expect(screen.getByText('low confidence')).toBeInTheDocument();
  });

  it('does not show a low-confidence indicator when confidence is high', () => {
    setupRefreshMock();
    render(<GhostSignalPanel jobId="job-1" ghostSignal={ghostSignal()} />);
    expect(screen.queryByText('low confidence')).not.toBeInTheDocument();
  });

  it('refresh button calls the mutation', async () => {
    const user = userEvent.setup();
    const { mutate } = setupRefreshMock();
    render(<GhostSignalPanel jobId="job-1" ghostSignal={ghostSignal()} />);

    await user.click(screen.getByRole('button', { name: /refresh/ }));
    expect(mutate).toHaveBeenCalledOnce();
  });

  it('disables the refresh button while a refresh is in flight', () => {
    setupRefreshMock(true);
    render(<GhostSignalPanel jobId="job-1" ghostSignal={ghostSignal()} />);
    const btn = screen.getByRole('button', { name: /scoring…/ });
    expect(btn).toBeDisabled();
  });
});
