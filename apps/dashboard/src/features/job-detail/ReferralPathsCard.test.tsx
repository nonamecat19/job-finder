import { render, screen } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import ReferralPathsCard from './ReferralPathsCard';
import { mockReferralPath, mockReferralContact } from '../../test/factories';

vi.mock('./hooks', () => ({
  useReferralPaths: vi.fn(),
}));

import { useReferralPaths } from './hooks';

const mockedUseReferralPaths = vi.mocked(useReferralPaths);

function setupQueryMock(overrides: Record<string, unknown>) {
  mockedUseReferralPaths.mockReturnValue({
    data: undefined,
    isLoading: false,
    isError: false,
    ...overrides,
  } as any);
}

describe('ReferralPathsCard', () => {
  it('renders loading state', () => {
    setupQueryMock({ isLoading: true });

    render(<ReferralPathsCard jobId="job-1" />);
    expect(screen.getByText('looking for warm paths…')).toBeInTheDocument();
  });

  it('renders nothing on error', () => {
    setupQueryMock({ isError: true });

    const { container } = render(<ReferralPathsCard jobId="job-1" />);
    expect(container).toBeEmptyDOMElement();
  });

  it('renders nothing when there are no paths', () => {
    setupQueryMock({ data: [] });

    const { container } = render(<ReferralPathsCard jobId="job-1" />);
    expect(container).toBeEmptyDOMElement();
  });

  it('renders each warm path with its contacts, chained by arrows', () => {
    const paths = [
      mockReferralPath({
        path: [
          mockReferralContact({ id: 'me', name: 'You', role: null }),
          mockReferralContact({ id: 'c1', name: 'Jane Doe', role: 'Engineering Manager' }),
        ],
        score: 0.72,
        length: 2,
      }),
    ];
    setupQueryMock({ data: paths });

    render(<ReferralPathsCard jobId="job-1" />);

    expect(screen.getByText('1 warm path found')).toBeInTheDocument();
    expect(screen.getByText('You')).toBeInTheDocument();
    expect(screen.getByText('Jane Doe')).toBeInTheDocument();
    expect(screen.getByText('Engineering Manager', { exact: false })).toBeInTheDocument();
    expect(screen.getByText('strong · 0.72')).toBeInTheDocument();
    expect(screen.getByText('1 hop')).toBeInTheDocument();
  });

  it('labels weak paths distinctly from strong ones', () => {
    const paths = [mockReferralPath({ score: 0.15, length: 3 })];
    setupQueryMock({ data: paths });

    render(<ReferralPathsCard jobId="job-1" />);
    expect(screen.getByText('weak · 0.15')).toBeInTheDocument();
    expect(screen.getByText('2 hops')).toBeInTheDocument();
  });

  it('pluralizes path count for multiple paths', () => {
    const paths = [mockReferralPath(), mockReferralPath()];
    setupQueryMock({ data: paths });

    render(<ReferralPathsCard jobId="job-1" />);
    expect(screen.getByText('2 warm paths found')).toBeInTheDocument();
  });
});
