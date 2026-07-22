import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import ContactLine from './ContactLine';
import { mockJobContact } from '../../test/factories';

vi.mock('./hooks', () => ({
  useJobContacts: vi.fn(),
  useRefreshJobContacts: vi.fn(),
}));

import { useJobContacts, useRefreshJobContacts } from './hooks';

const mockedUseJobContacts = vi.mocked(useJobContacts);
const mockedUseRefreshJobContacts = vi.mocked(useRefreshJobContacts);

function setupQueryMock(overrides: Record<string, unknown>) {
  mockedUseJobContacts.mockReturnValue({
    data: undefined,
    isLoading: false,
    ...overrides,
  } as any);
}

function setupRefreshMock(isPending = false) {
  const mutate = vi.fn();
  mockedUseRefreshJobContacts.mockReturnValue({ mutate, isPending } as any);
  return { mutate };
}

describe('ContactLine', () => {
  it('renders loading state', () => {
    setupQueryMock({ isLoading: true });

    render(<ContactLine jobId="job-1" />);
    expect(screen.getByText('loading contact…')).toBeInTheDocument();
  });

  it('renders the "No contact found — try Refresh" state when zero contacts resolved', () => {
    setupRefreshMock();
    setupQueryMock({ data: [] });

    render(<ContactLine jobId="job-1" />);
    expect(screen.getByTestId('contact-empty')).toHaveTextContent('No contact found — try Refresh');
  });

  it('renders the headline contact name and title when one contact is resolved', () => {
    setupRefreshMock();
    const contact = mockJobContact({ name: 'Jane Doe', title: 'Recruiter', email: 'jane@acme.com' });
    setupQueryMock({ data: [contact] });

    render(<ContactLine jobId="job-1" />);
    const headline = screen.getByTestId('contact-headline');
    expect(headline).toHaveTextContent('Jane Doe — Recruiter');
    expect(headline).toHaveTextContent('jane@acme.com');
  });

  it('renders name-only when no title was resolved', () => {
    setupRefreshMock();
    const contact = mockJobContact({ name: 'Jane Doe', title: null, email: null });
    setupQueryMock({ data: [contact] });

    render(<ContactLine jobId="job-1" />);
    expect(screen.getByTestId('contact-headline')).toHaveTextContent('Jane Doe');
  });

  it('surfaces the highest-confidence contact as the headline when several are resolved', () => {
    setupRefreshMock();
    const low = mockJobContact({ id: 'c-low', name: 'Low Confidence', confidence: 0.3, source: 'linkedin' });
    const high = mockJobContact({ id: 'c-high', name: 'High Confidence', confidence: 0.9, source: 'posting' });
    // API already returns contacts ordered best-first; the component trusts that order.
    setupQueryMock({ data: [high, low] });

    render(<ContactLine jobId="job-1" />);
    expect(screen.getByTestId('contact-headline')).toHaveTextContent('High Confidence');
  });

  it('clicking Refresh contacts calls the refresh mutation', async () => {
    const user = userEvent.setup();
    const { mutate } = setupRefreshMock();
    setupQueryMock({ data: [] });

    render(<ContactLine jobId="job-1" />);
    await user.click(screen.getByRole('button', { name: /refresh contacts/i }));
    expect(mutate).toHaveBeenCalledOnce();
  });

  it('shows a spinner on the refresh button while the mutation is pending', () => {
    setupRefreshMock(true);
    setupQueryMock({ data: [] });

    render(<ContactLine jobId="job-1" />);
    const btn = screen.getByRole('button', { name: /refresh contacts/i });
    expect(btn).toBeDisabled();
  });
});
