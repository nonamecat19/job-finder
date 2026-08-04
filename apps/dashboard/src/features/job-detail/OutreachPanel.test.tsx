import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import OutreachPanel from './OutreachPanel';
import { mockJobContact, mockOutreachDraft, mockOutreachToneOptions } from '../../test/factories';

vi.mock('./hooks', () => ({
  useJobContacts: vi.fn(),
  useOutreachTones: vi.fn(),
  useGenerateOutreachDraft: vi.fn(),
}));

vi.mock('../../lib/toastBus', () => ({
  emitToast: vi.fn(),
  toErrorMessage: (err: unknown) => (err instanceof Error ? err.message : String(err)),
}));

import { useJobContacts, useOutreachTones, useGenerateOutreachDraft } from './hooks';
import { emitToast } from '../../lib/toastBus';

const mockedUseJobContacts = vi.mocked(useJobContacts);
const mockedUseOutreachTones = vi.mocked(useOutreachTones);
const mockedUseGenerateOutreachDraft = vi.mocked(useGenerateOutreachDraft);

function setupContacts(data: unknown = []) {
  mockedUseJobContacts.mockReturnValue({ data, isLoading: false } as any);
}

function setupTones(data: unknown = mockOutreachToneOptions()) {
  mockedUseOutreachTones.mockReturnValue({ data, isLoading: false } as any);
}

function setupGenerate(overrides: Record<string, unknown> = {}) {
  const mutate = vi.fn();
  mockedUseGenerateOutreachDraft.mockReturnValue({
    mutate,
    isPending: false,
    isError: false,
    error: null,
    ...overrides,
  } as any);
  return { mutate };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('OutreachPanel', () => {
  it('renders the draft-only disclaimer and no send action anywhere', () => {
    setupContacts([]);
    setupTones();
    setupGenerate();

    render(<OutreachPanel jobId="job-1" />);
    expect(screen.getByText(/Draft-only/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /send/i })).not.toBeInTheDocument();
  });

  it('shows a neutral-greeting option when no contact is resolved', () => {
    setupContacts([]);
    setupTones();
    setupGenerate();

    render(<OutreachPanel jobId="job-1" />);
    expect(screen.getByText(/No resolved contact — neutral greeting/)).toBeInTheDocument();
  });

  it('lists resolved contacts in the contact selector', () => {
    setupContacts([mockJobContact({ id: 'c1', name: 'Jane Doe', title: 'Recruiter' })]);
    setupTones();
    setupGenerate();

    render(<OutreachPanel jobId="job-1" />);
    expect(screen.getByText('Jane Doe — Recruiter')).toBeInTheDocument();
  });

  it('clicking Generate draft calls the mutation', async () => {
    const user = userEvent.setup();
    setupContacts([mockJobContact({ id: 'c1' })]);
    setupTones();
    const { mutate } = setupGenerate();

    render(<OutreachPanel jobId="job-1" />);
    await user.click(screen.getByRole('button', { name: /Generate draft/ }));
    expect(mutate).toHaveBeenCalledOnce();
  });

  it('refuses to generate without an explicit contact choice when several contacts exist', async () => {
    const user = userEvent.setup();
    setupContacts([mockJobContact({ id: 'c1', name: 'Jane Doe' }), mockJobContact({ id: 'c2', name: 'Bob Recruiter' })]);
    setupTones();
    const { mutate } = setupGenerate();

    render(<OutreachPanel jobId="job-1" />);
    await user.click(screen.getByRole('button', { name: /Generate draft/ }));
    expect(mutate).not.toHaveBeenCalled();
    expect(emitToast).toHaveBeenCalledWith(expect.objectContaining({ title: expect.stringContaining('Choose a contact') }));
  });

  it('renders the generated draft text, contact, and tone once produced', async () => {
    const user = userEvent.setup();
    setupContacts([mockJobContact({ id: 'c1' })]);
    setupTones();
    const draft = mockOutreachDraft();
    const { mutate } = setupGenerate();
    mutate.mockImplementation((_vars: unknown, opts: { onSuccess: (d: typeof draft) => void }) => opts.onSuccess(draft));

    render(<OutreachPanel jobId="job-1" />);
    await user.click(screen.getByRole('button', { name: /Generate draft/ }));

    expect(screen.getByTestId('outreach-draft')).toBeInTheDocument();
    expect(screen.getByTestId('outreach-draft-text')).toHaveValue(draft.text);
    expect(screen.getByText(`addressed to: ${draft.contactName}`)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Regenerate/ })).toBeInTheDocument();
  });

  it('shows the honest "no specific claims" message when the draft has zero grounding traces', async () => {
    const user = userEvent.setup();
    setupContacts([mockJobContact({ id: 'c1' })]);
    setupTones();
    const draft = mockOutreachDraft({ groundingTraces: [] });
    const { mutate } = setupGenerate();
    mutate.mockImplementation((_vars: unknown, opts: { onSuccess: (d: typeof draft) => void }) => opts.onSuccess(draft));

    render(<OutreachPanel jobId="job-1" />);
    await user.click(screen.getByRole('button', { name: /Generate draft/ }));

    expect(screen.getByText(/No specific claims/)).toBeInTheDocument();
    expect(screen.queryByTestId('outreach-traces-toggle')).not.toBeInTheDocument();
  });

  it('expands grounding traces to show which signal backs each claim', async () => {
    const user = userEvent.setup();
    setupContacts([mockJobContact({ id: 'c1' })]);
    setupTones();
    const draft = mockOutreachDraft();
    const { mutate } = setupGenerate();
    mutate.mockImplementation((_vars: unknown, opts: { onSuccess: (d: typeof draft) => void }) => opts.onSuccess(draft));

    render(<OutreachPanel jobId="job-1" />);
    await user.click(screen.getByRole('button', { name: /Generate draft/ }));
    await user.click(screen.getByTestId('outreach-traces-toggle'));

    const traces = screen.getByTestId('outreach-traces');
    expect(traces).toHaveTextContent('Go and React');
    expect(traces).toHaveTextContent('tech_stack');
    expect(traces).toHaveTextContent('Go, React, Postgres');
  });

  it('editing the draft text updates what copy will send to the clipboard', async () => {
    const user = userEvent.setup();
    const writeText = vi.spyOn(navigator.clipboard, 'writeText');
    setupContacts([mockJobContact({ id: 'c1' })]);
    setupTones();
    const draft = mockOutreachDraft();
    const { mutate } = setupGenerate();
    mutate.mockImplementation((_vars: unknown, opts: { onSuccess: (d: typeof draft) => void }) => opts.onSuccess(draft));

    render(<OutreachPanel jobId="job-1" />);
    await user.click(screen.getByRole('button', { name: /Generate draft/ }));

    const textarea = screen.getByTestId('outreach-draft-text');
    await user.clear(textarea);
    await user.type(textarea, 'edited message');
    await user.click(screen.getByRole('button', { name: /^copy$/i }));

    expect(writeText).toHaveBeenCalledWith('edited message');
  });

  it('shows a spinner on Generate draft while pending', () => {
    setupContacts([]);
    setupTones();
    setupGenerate({ isPending: true });

    render(<OutreachPanel jobId="job-1" />);
    const btn = screen.getByRole('button', { name: /Generate draft/ });
    expect(btn).toBeDisabled();
  });

  it('renders an error message when generation fails', () => {
    setupContacts([]);
    setupTones();
    setupGenerate({ isError: true, error: new Error('LLM unavailable') });

    render(<OutreachPanel jobId="job-1" />);
    expect(screen.getByText('LLM unavailable')).toBeInTheDocument();
  });

  it('renders empty state when contacts loaded but none resolved', () => {
    setupContacts([]);
    setupTones();
    setupGenerate();

    render(<OutreachPanel jobId="job-1" />);
    expect(screen.getByText(/No resolved contact/)).toBeInTheDocument();
  });

  it('renders loading spinner while generating draft', () => {
    setupContacts([]);
    setupTones();
    setupGenerate({ isPending: true });

    render(<OutreachPanel jobId="job-1" />);
    expect(screen.getByText(/drafting outreach message/)).toBeInTheDocument();
  });
});
