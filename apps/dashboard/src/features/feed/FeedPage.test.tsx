import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi } from 'vitest';
import FeedPage from './FeedPage';
import { mockJobListResponse } from '../../test/factories';

vi.mock('./hooks', () => ({
  useJobs: vi.fn(),
  useFeedSources: vi.fn(),
  useFeedSubscriptions: vi.fn(),
  useShortlistJob: vi.fn(),
  useHideJob: vi.fn(),
  useClearJobs: vi.fn(),
}));

import { useJobs, useFeedSources, useFeedSubscriptions, useShortlistJob, useHideJob, useClearJobs } from './hooks';

const mockedUseJobs = vi.mocked(useJobs);

function setupCommonMocks() {
  vi.mocked(useFeedSources).mockReturnValue({ data: [] } as any);
  vi.mocked(useFeedSubscriptions).mockReturnValue({ data: [] } as any);
  vi.mocked(useShortlistJob).mockReturnValue({ mutate: vi.fn() } as any);
  vi.mocked(useHideJob).mockReturnValue({ mutate: vi.fn() } as any);
  vi.mocked(useClearJobs).mockReturnValue({ mutate: vi.fn(), isPending: false } as any);
}

function renderFeedPage() {
  return render(
    <MemoryRouter>
      <FeedPage />
    </MemoryRouter>,
  );
}

describe('FeedPage loading state', () => {
  it('renders a skeleton job list while loading, not a spinner', () => {
    setupCommonMocks();
    mockedUseJobs.mockReturnValue({ data: undefined, isLoading: true, error: null } as any);

    renderFeedPage();

    const region = screen.getByRole('status');
    expect(region).toHaveAttribute('aria-busy', 'true');
    expect(screen.getByText('loading jobs…')).toBeInTheDocument();
    expect(document.querySelector('.animate-spin')).not.toBeInTheDocument();
    expect(document.querySelectorAll('.animate-pulse').length).toBeGreaterThan(0);
  });

  it('renders real content and no skeleton once data resolves', () => {
    setupCommonMocks();
    mockedUseJobs.mockReturnValue({
      data: mockJobListResponse(),
      isLoading: false,
      error: null,
    } as any);

    renderFeedPage();

    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(document.querySelector('.animate-pulse')).not.toBeInTheDocument();
  });

  it('renders ErrorState and no skeleton when the request fails', () => {
    setupCommonMocks();
    mockedUseJobs.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('network down'),
    } as any);

    renderFeedPage();

    expect(screen.getByText('network down')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(document.querySelector('.animate-pulse')).not.toBeInTheDocument();
  });
});
