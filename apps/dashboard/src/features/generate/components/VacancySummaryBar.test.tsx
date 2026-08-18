import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderWithProviders, screen, waitFor } from '../../../test/test-utils';
import { api } from '../../../lib/api';
import VacancySummaryBar from './VacancySummaryBar';

vi.mock('../../../lib/api', () => ({
  api: { jobs: { get: vi.fn() } },
}));

describe('VacancySummaryBar', () => {
  beforeEach(() => vi.clearAllMocks());

  it('pulls the full posting from the job', async () => {
    vi.mocked(api.jobs.get).mockResolvedValue({
      id: 'job-1',
      company: 'Acme Inc.',
      title: 'Senior Platform Engineer',
      location: 'Berlin',
      remote: true,
      salaryRaw: '€78,000–92,000',
      description: 'Own our payment and event pipelines.',
      documents: [],
      application: null,
    } as unknown as Awaited<ReturnType<typeof api.jobs.get>>);

    renderWithProviders(<VacancySummaryBar jobId="job-1" />);

    await waitFor(() => expect(screen.getByText('Senior Platform Engineer')).toBeInTheDocument());
    expect(screen.getByText(/Berlin/)).toBeInTheDocument();
    expect(screen.getByText(/remote/)).toBeInTheDocument();
    expect(screen.getByText(/Own our payment and event pipelines\./)).toBeInTheDocument();
    expect(screen.getByText(/came from the job you picked in Feed/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /edit it there/i })).toHaveAttribute('href', '/jobs/job-1');
  });
});
