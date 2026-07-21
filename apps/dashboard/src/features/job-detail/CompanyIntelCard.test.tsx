import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import CompanyIntelCard from './CompanyIntelCard';
import { mockCompanyIntel } from '../../test/factories';

vi.mock('./hooks', () => ({
  useCompanyIntel: vi.fn(),
  useRefreshCompanyIntel: vi.fn(),
}));

import { useCompanyIntel, useRefreshCompanyIntel } from './hooks';

const mockedUseCompanyIntel = vi.mocked(useCompanyIntel);
const mockedUseRefreshCompanyIntel = vi.mocked(useRefreshCompanyIntel);

function setupRefreshMock(isPending = false) {
  const mutate = vi.fn();
  mockedUseRefreshCompanyIntel.mockReturnValue({ mutate, isPending } as any);
  return { mutate };
}

function setupQueryMock(overrides: Record<string, unknown>) {
  mockedUseCompanyIntel.mockReturnValue({
    data: undefined,
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
    ...overrides,
  } as any);
}

describe('CompanyIntelCard', () => {
  it('renders loading state', () => {
    setupQueryMock({ isLoading: true });
    setupRefreshMock();

    render(<CompanyIntelCard jobId="job-1" />);
    expect(screen.getByText('loading company intel…')).toBeInTheDocument();
  });

  it('renders error state with retry button', async () => {
    const user = userEvent.setup();
    const refetch = vi.fn();
    setupQueryMock({ isError: true, refetch });
    setupRefreshMock();

    render(<CompanyIntelCard jobId="job-1" />);
    expect(screen.getByText('Failed to load company intel')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'retry' }));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it('renders empty state with refresh button', async () => {
    const user = userEvent.setup();
    const { mutate } = setupRefreshMock();
    setupQueryMock({ data: null });

    render(<CompanyIntelCard jobId="job-1" />);
    expect(screen.getByText('No company intel yet. Click Refresh to fetch.')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(mutate).toHaveBeenCalledOnce();
  });

  it('renders populated state with collapsed summary', () => {
    const intel = mockCompanyIntel();
    setupRefreshMock();
    setupQueryMock({ data: intel });

    render(<CompanyIntelCard jobId="job-1" />);
    expect(screen.getByText('Company Intel')).toBeInTheDocument();
    expect(screen.getByText('Acme Corp — funding Series B — Glassdoor 3.8')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument();
  });

  it('expands to show all signal rows on click', async () => {
    const user = userEvent.setup();
    const intel = mockCompanyIntel();
    setupRefreshMock();
    setupQueryMock({ data: intel });

    render(<CompanyIntelCard jobId="job-1" />);

    await user.click(screen.getByText('Company Intel'));

    expect(screen.getByText('Funding')).toBeInTheDocument();
    expect(screen.getByText('Series B')).toBeInTheDocument();
    expect(screen.getByText('Layoffs')).toBeInTheDocument();
    expect(screen.getByText('Glassdoor rating')).toBeInTheDocument();
    expect(screen.getByText('3.8')).toBeInTheDocument();
    expect(screen.getByText('Headcount')).toBeInTheDocument();
    expect(screen.getByText('200-500')).toBeInTheDocument();
    expect(screen.getByText('Tech stack')).toBeInTheDocument();
    expect(screen.getByText('React, TypeScript, Go')).toBeInTheDocument();
  });

  it('shows "No data yet" for null signal values', async () => {
    const user = userEvent.setup();
    const intel = mockCompanyIntel({ funding: null, glassdoorRating: null, headcount: null, techStack: null, layoffs: null });
    setupRefreshMock();
    setupQueryMock({ data: intel });

    render(<CompanyIntelCard jobId="job-1" />);
    await user.click(screen.getByText('Company Intel'));

    const noDataElements = screen.getAllByText('No data yet');
    expect(noDataElements.length).toBe(5);
  });

  it('shows stale tag for data older than 30 days', async () => {
    const user = userEvent.setup();
    const staleDate = new Date(Date.now() - 31 * 24 * 60 * 60 * 1000).toISOString();
    const intel = mockCompanyIntel({ fetchedAt: staleDate });
    setupRefreshMock();
    setupQueryMock({ data: intel });

    render(<CompanyIntelCard jobId="job-1" />);
    await user.click(screen.getByText('Company Intel'));

    const staleTags = screen.getAllByText('possibly stale');
    expect(staleTags.length).toBeGreaterThanOrEqual(1);
  });

  it('refresh button calls mutation', async () => {
    const user = userEvent.setup();
    const intel = mockCompanyIntel();
    const { mutate } = setupRefreshMock();
    setupQueryMock({ data: intel });

    render(<CompanyIntelCard jobId="job-1" />);

    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(mutate).toHaveBeenCalledOnce();
  });

  it('shows spinner on refresh button while mutation is pending', () => {
    const intel = mockCompanyIntel();
    setupQueryMock({ data: intel });
    setupRefreshMock(true);

    render(<CompanyIntelCard jobId="job-1" />);
    const btn = screen.getByRole('button', { name: 'Refresh' });
    expect(btn).toBeDisabled();
    expect(btn.querySelector('.animate-spin')).toBeInTheDocument();
  });
});
